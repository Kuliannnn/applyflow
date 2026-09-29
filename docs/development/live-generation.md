# Personal AI generation — 0.7.0

The text JD workflow supports two explicit modes: `mock` (sample drafts, no external AI) and `personal` (OpenAI with the user's saved key). There is no automatic fallback between them. Screenshots, links, automatic resume parsing and AI revision instructions remain separate future work.

## Run it

1. Apply migration 00008 with `make migrate-up`. Restart both API and worker; both require schema 8.
2. Set **the same persistent** `CREDENTIAL_MASTER_KEY` and `CREDENTIAL_KEY_VERSION` in both processes. These are application encryption credentials, not the user's provider key. Keep the API's existing signing keys and database configuration. The API cannot inspect another worker process's environment; an unconfigured/mismatched worker fails the task with a safe error.
3. Open Account → AI settings. Save the provider key/model, test and explicitly enable it. A test itself may incur a small provider charge.
4. In Create, select **My AI connection**, paste the JD and select a confirmed resume. Review company, role, JD and the facts before **Generate with AI**. Sample mode remains the default and always requires explicit switching to personal mode.
5. Review the independent Resume / Cover letter results, edit and save. Export PDF or Word using the existing fixed-revision flow. A failed document does not discard the other; Retry makes a new paid attempt.

`GET /api/ai/providers` advertises API credential capability. The API's flag does not prove a worker is running or correctly configured. A personal generation request additionally carries `ai_revision`; the server checks the current enabled/tested revision and binds the immutable run to it. The model/provider/revision are returned on the generation response. Neither the request nor queued tasks contain a plaintext key. Replacing a model/key affects new runs; accepted runs keep their bindings. Disabling the configuration or deleting credentials prevents unsent calls. Deleting cannot retract a call already dispatched.

## Provider boundary and document quality

The worker uses the fixed OpenAI Responses endpoint and [Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs), with a separate strict schema for each document. The request contains confirmed JD text, role/company and all selected confirmed fact IDs/categories/text. It excludes original files, evidence excerpts, login identity and unrelated profile fields. Free-form facts or JD text can themselves contain personal information: the user should review what they submit. Contact details are not fetched from the separate profile or invented by the model.

Prompt `personal-v1` instructs the model to treat inputs as untrusted data, rewrite only supported facts and not invent qualifications, dates, skills, metrics or experience. Each substantive item/paragraph must cite supplied fact IDs; the server validates document shape, field limits and fact ownership before committing. Title/salutation/closing are constructed locally. These checks do **not** prove semantic truth: the user must review every claim. Source-ID validation is not a hallucination detector.

Supported models remain the registered `gpt-4.1-mini` and `gpt-4.1`. No arbitrary URL, redirects, environment proxy, model tools or automatic provider retries. Input JSON is bounded to 128 KiB; response to 256 KiB; output to 4,000 tokens per document; one call has a maximum of 60 seconds. `store=false` is requested; it does not override every provider retention policy. Truncation, refusal, malformed structure or unsupported evidence fails that document instead of storing a partial draft or mock substitute. Raw provider errors, bodies and prompts are not logged.

## Reservations, usage and unknown outcomes

- Two documents atomically reserve two request slots when accepting a run. A single-document retry reserves one. The owner lock serializes checks/reservations across replicas and workspaces; a version/idempotency conflict rolls everything back.
- `daily_request_limit` now bounds reservations plus dispatched attempts over a **rolling 24-hour window**. Tests remain on their independent 3/minute and 20/day budget. Failed/cancelled unsent tasks release their reservations through the budget query; dispatched calls count even after cancellation/failure.
- A worker rechecks revocation, enablement, lease and the current limit immediately before dispatch. Old reservations crossing the 24-hour window are rechecked. Lowering the limit can stop previously queued work. A reservation is a request-count bound, not a dollar or token cap; other apps using the same provider key are outside it.
- The usage row is marked `started` and committed **before** external I/O. The same task cannot dispatch twice. After a crash, a recovered task encounters that marker and fails `provider_outcome_unknown`; it does not send again. This conservatively counts an attempt even if the process crashed between the marker and the network send.
- Provider-reported input/output tokens are stored when available, including incomplete outputs. Missing usage remains null, never fabricated as zero. A result can have known usage even when its document commit fails. A failure to durably record usage leaves an uncertain attempt.
- `GET /api/me/ai-usage` shows only the owner's rolling totals: reserved/attempted calls, known input/output tokens and the count with unknown usage. AI settings displays these values. It is not an exact invoice or a cost estimate.
- API idempotency prevents duplicate acceptance. The frontend replays the same non-secret generation request after an uncertain response, and refuses to resend a pending personal request under a newly selected sample mode. Reloading restores saved workspaces; do not interpret a network failure as proof no provider call happened.

Task leases are 90 seconds and a worker iteration is bounded to 75 seconds, covering the 60-second provider timeout and database work. This release intentionally uses bounded calls under a fixed lease rather than introducing Redis or lease renewal. Existing cancellation/fencing and candidate revision rules remain: stale completion cannot overwrite a newer task or a human edit. Durable PostgreSQL outbox dispatch remains in use.

A failed document requires explicit Retry. That creates a new task and may incur another charge. Retry uses the run's original AI revision; if the current configuration has changed, start a new generation from Job details. A timeout is reported as uncertain, never silently retried. Cancellation is local and does not guarantee the provider stopped computing or charging.

## Verification and limits

`make check`, `make test-migrations-local` and `make check-frontend` cover contracts, provider adapter response parsing/limits, owner-bound credentials, reservations, revocation, crash recovery, partial success, explicit retry and fixed-revision PDF/DOCX export. Provider tests use synthetic HTTP responses; PostgreSQL tests use a disposable database. They do not require a real key or incur model charges. A real paid call and model-writing quality evaluation still require the user's configured provider account; no real key is used by the automated suite.

Large-document performance, broader models, semantic quality evaluations, provider billing reconciliation and long-running/streaming execution are not claimed by this increment. The provider timeout is deliberately bounded and can fail slow requests safely.
