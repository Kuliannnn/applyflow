# Personal AI settings — 0.8.0

This increment implements the backend for **save → test → explicitly enable → replace/delete**. The settings form and personal generation are now connected. Users explicitly select My AI connection on Create; enabling alone does not silently change the default sample mode. Daily request reservations and known token usage are enforced/recorded over a rolling 24-hour window; see [live generation](live-generation.md).

## Configure the API

1. Apply migrations through `00009_aiwanwu_provider.sql` using `make migrate-up`. API and worker now require schema version 9.
2. Configure a separate, persistent `CREDENTIAL_MASTER_KEY` (base64 encoding of exactly 32 random bytes) and `CREDENTIAL_KEY_VERSION=1` in the API process environment. Generate the key once with `openssl rand -base64 32`, then keep it in your local secret environment or deployment secret manager. Do not commit it or paste it into logs/chat. It must differ from `AUTH_SIGNING_KEY` and `CSRF_KEY`.
3. Restart the API. The worker needs the same credential key/version for personal generation.

Both credential variables may be unset: the mock workflow still works, the provider catalogue reports `available=false`, configuration reads/deletes work, and save/test/enable return 503 `ai_credentials_unavailable`. If either variable is provided, both must be valid; malformed configuration fails startup. `.env.example` is documentation, not automatically loaded.

The master key is the application's encryption key; it is **not** a user's provider API key. Keep the same key/version across API restarts and replicas. Only one master key/version is supported in this increment: key rotation and re-encryption tooling are deferred. Replacing or losing the master key makes stored credentials unreadable; users would have to delete and re-enter them. Back up the master key securely and separately from the database.

## API sequence

All routes require the existing authenticated session. All writes require the existing same-origin `Origin`, CSRF cookie and `X-CSRF-Token`. The server derives ownership from the session, rejects unknown/duplicate fields and never accepts an arbitrary Base URL.

| Route | Behavior |
|---|---|
| `GET /api/ai/providers` | OpenAI/aiwanwu catalogue with fixed endpoint URLs, supported models, availability and test cost notice; `generation_available` reflects API credential availability and `platform_available=false` |
| `GET /api/me/ai-config` | Safe configuration, version and status; an empty account starts at version 0 |
| `PUT /api/me/ai-config` | `expected_version`, `provider_id`, `model_id`, `daily_request_limit`, optional write-only `api_key`; first save requires the key |
| `POST /api/me/ai-config/test` | `expected_version`, `revision`, UUID `Idempotency-Key`; bounded fixed-prompt test |
| `PATCH /api/me/ai-config` | `expected_version` and at least one of `enabled`, `daily_request_limit`, `mode` (`personal` only) |
| `DELETE /api/me/ai-config` | `If-Match: "ai-N"`; revokes all saved credentials for the account and returns the new safe snapshot |

Read current version before mutations. A usual first-save sequence is version 0 → saved 1 → testing 2 → tested 3 → explicitly enabled 4. GET emits an ETag with the version. Version mismatch returns 409 `config_version_conflict`; GET again before deciding how to resolve it. Changing only the daily limit preserves a successful test. Replacing the key or model creates a new immutable revision and resets to disabled/untested. Omitting `api_key` retains the saved key; null, blank and masked keys are rejected. No key prefix/suffix is returned.

The initial catalogue includes `gpt-4.1-mini` and `gpt-4.1`. This is a deliberately bounded supported list, not a claim that these are the newest models or available to every provider account. Changes require updating the server validator, database allowlist and OpenAPI together.

## Connection tests

A user-initiated test makes one request to the selected provider’s fixed Responses endpoint with the fixed input `Reply with OK.`, `store=false`, `stream=false`, and `max_output_tokens=32`. It sends no resume, JD or profile. **A real test may incur a small provider charge.** It has an 8-second maximum, a 64 KiB response bound, bounded network timeouts, no redirects, no environment proxy and no automatic application retry. The accepted call runs independently of a disconnected browser. Transport and provider response bodies are never returned or logged.

The provider implementation follows the official [text generation guide](https://developers.openai.com/api/docs/guides/text) and [conversation state guidance](https://developers.openai.com/api/docs/guides/conversation-state). `store=false` controls response storage; it is not a general promise about every provider data-retention policy.

The database allows one running test per account, at most 3 accepted tests per rolling minute and 20 per rolling 24 hours. These test limits are separate from `daily_request_limit` and apply across API replicas. Failed/uncertain accepted tests count too. Same idempotency key and body never initiate another provider call: completed replay returns its original safe snapshot; in-flight replay returns 202 and the current safe snapshot. Reusing a key with a different body returns 409. Replay is checked before version preconditions. A replayed snapshot can be historical: GET current configuration before any subsequent mutation.

A persisted 12-second deadline covers the bounded provider call and final database write. After a crash or failed final write, the next configuration access converts the expired test to `inconclusive`/`test_interrupted`; it never automatically calls the provider again. Retrying with a new key is an explicit new test and may incur another charge. Test receipts currently remain indefinitely (at least the designed 24 hours); retention cleanup is deferred.

Authentication/model/rate failures from the provider are a 200 configuration response with `test_status=failed` and a safe error code, not a local authentication failure. Timeouts, malformed responses or infrastructure uncertainty produce `inconclusive`. Local infrastructure failures use 503. A successful test does not enable the configuration. A late result cannot activate or mark a replaced/deleted revision ready. Deleting a key cannot retract a request already accepted/in flight with the provider.

## Storage and maintenance

- AES-256-GCM, fresh random nonce, authenticated owner + credential ID + key version. Only ciphertext/nonce/version enter `ai_credentials`; ordinary reads and test receipts contain no secret material.
- `user_ai_settings` owns the monotonic visible version and current revision; `user_ai_config_revisions` holds immutable model/credential bindings plus mutable test metadata. `ai_test_requests` stores hashed idempotency keys, hashes of non-secret test requests, deadlines and safe snapshots.
- All settings transactions lock the user first; external provider I/O happens after the acceptance transaction commits. Current-revision/run checks fence result writes. Delete erases live database ciphertext/nonce for all the owner's credentials, permanently revokes their revisions and preserves counters. It does not revoke the external provider key or erase backups/WAL.
- Limit-only edits retain the revision; replacing credentials retains historical encrypted rows until account configuration deletion. Automated historical credential/receipt cleanup is a later maintenance task.
- Readiness uses `migrations.LatestVersion`, shared by the API and worker. Released migrations 00001–00007 remain unchanged.

Verification uses unit tests with fake HTTP responses and a disposable PostgreSQL integration suite. No real user key or paid provider call is needed to run `make check` and `make test-migrations-local`. Tests cover encryption/AAD tampering, fixed outbound destination, response bounds, auth/CSRF, ownership, revision CAS, saved-key reuse, explicit enable, concurrent replay, replacement/deletion fences, crash expiration and the test budget.

## Frontend settings

Open **Account → AI settings** (`/settings/ai`). The English glass form supports save, replacing the key/model, connection tests, explicit enable/disable and confirmed removal. Key input is local component state only and clears after a successful save; the saved key is never fetched back. Leaving with unsaved changes warns before navigation. Usage preferences are collapsed and explicitly describe the rolling daily request limit.

A lost test response offers **Check same test**, replaying its in-memory UUID and original non-secret request. It then fetches current settings because a completed replay can be historical. A running test polls safe metadata; poll failure pauses until an explicit refresh. No background test or automatic mutation retry is performed. If the tab is reloaded, GET restores the persisted test status; a crashed test becomes inconclusive after its server deadline. Changes in another tab require **Review latest settings** before resubmission and retain local edits. Missing server encryption configuration disables credential entry with a clear explanation.

Run `make check-frontend`. With Vite on a disposable port, `E2E_URL=http://127.0.0.1:5194 npm --prefix frontend run test:ai-browser` tests the UI with synthetic API responses (no real provider calls). `CHROMIUM_PATH` can select an installed test browser. This complements the real PostgreSQL/API tests; it is not a live provider verification.

## Registered providers (0.8.0)

The shared `aisettings.Providers()` registry drives validation, catalogue, connection tests and live dispatch. OpenAI maps to `https://api.openai.com/v1/responses` with `gpt-4.1-mini` / `gpt-4.1`; aiwanwu maps to `https://2api.aiwanwu.cc/v1/responses` with `gpt-6-sol`. Bearer authentication is used for both. The relay endpoint follows the provider documentation supplied by the user: recommended host `2api.aiwanwu.cc` and `/v1/responses`. The supplied page documents image-tool use; text-model access and structured-output compatibility still need account verification. No redirects, environment proxy, alternate-path fallback or arbitrary client URL/header is permitted. Host-level organization/project credentials are not forwarded.

Changing provider requires explicitly supplying a new key, even if one is saved; the form also clears any unsaved key on provider changes. An immutable revision binds provider, model and credential, and both the durable test claim and worker credential carry the provider ID. Historical tasks keep their bound destination. Migration 00009 replaces the official-only constraints with valid provider/model pairs; downgrade refuses while relay revisions exist, including revoked history.

The UI discloses the third-party destination before testing/generating. Compatibility tests use fake HTTP transports, not a real relay account. The service must support non-streaming Responses and strict JSON-schema output. Codex's reasoning effort/context window/features are not copied into document generation; the existing bounded token/time budgets remain in force. A successful small connection test does not prove support for structured generation.
