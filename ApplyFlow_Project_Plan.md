# ApplyFlow — AI Resume & Cover Letter Studio

> 2026-09-22 实施状态：OpenAPI 0.5.0 定义 50 个操作，其中 34 个已有 handler。已实现认证、工作区、私有 PDF/DOCX、手动简历确认、文本 JD、独立 Mock Worker、双文档版本保存/应用、任务取消、单份重试与固定版本 PDF/DOCX 导出。当前 Worker 使用 PostgreSQL outbox 直接派发；Redis/Asynq、真实 AI、自动简历解析、截图/链接、SSE 尚未实现；英文玻璃前端已接通现有 Mock 流程。迁移仍为 00001–00006。

> Implementation status: the first foundation slice (OpenAPI, executable SQL migrations and tests) is now available. See [README](README.md). Auth/workspace HTTP handlers are implemented; UI and the remaining handlers are not.

> Engineering implementation: see [ARCHITECTURE.md](ARCHITECTURE.md) for the authoritative package layout, file responsibilities, transaction boundaries, API refinements, and production acceptance criteria. The trees below are introductory sketches; use the architecture documents when implementing.

## 1. Project Overview

**ApplyFlow** turns a job description (screenshot, text, or public link) and a user’s base resume into a tailored resume and cover letter. Users review facts, edit drafts, export documents, and optionally track the application. The interface defaults to English.

The approved visual direction is V3: light peach/rose/sage diffusion behind neutral frosted glass, dark readable text, and a minimal creation workflow. See [approved interface design](docs/design/DESIGN.md) and [Application Studio architecture](docs/architecture/application-studio.md). These supersede the earlier tracker-first scope; existing migrations and foundation contracts remain valid.

The project is intentionally designed as more than a CRUD dashboard. Its backend focuses on realistic engineering concerns such as:

- asynchronous task processing
- Redis caching and queues
- worker pools
- bounded concurrency
- Server-Sent Events (SSE)
- rate limiting
- database load reduction
- authentication and authorisation
- Nginx reverse proxy
- security controls
- load testing and performance monitoring

### Core idea

1. Paste a JD, paste a public job link, or upload/drop/paste a screenshot.
2. Upload a base resume or reuse a confirmed resume revision.
3. Confirm extracted job details and any uncertain resume facts.
4. Generate a tailored resume and cover letter using the selected AI configuration.
5. Review changes, edit directly or request AI revisions, and apply selected candidates.
6. Export a saved version as PDF or DOCX.
7. Optionally save the workspace and selected documents to an application record.

AI may reorganise and clarify evidence-backed experience; it must not invent qualifications or metrics. Missing information prompts the user. Interview preparation and analytics remain supporting, later features.

# 2. Main User Flow

```text
Create → JD (image / text / link) + base resume
       → Review extracted job details and facts
       → Generate two independent document drafts
       → Resume / Cover letter tabs: edit, compare, refine
       → Export selected saved revision
       → Optional: Save to my applications
```

Drafts exist in a workspace before a tracking application is created. One failed document must not discard the other. Refresh restores accepted tasks and saved revisions; in-flight AI results never silently overwrite human edits.

---

# 3. Target Users

Initial version:

- personal use
- graduate developers
- junior developers
- software engineering job seekers

Initial deployment may serve one person, but every private resource is owned by a user from day one. Multiple registered accounts must remain isolated even in the MVP.

## Delivery Scope

The first complete product includes all three JD input modes, PDF/DOCX base-resume import and confirmation, personal AI configuration, reliable asynchronous generation of both documents, manual/AI editing, version review, PDF/DOCX export, and optional application tracking. Start implementation with text JD and a mock provider as an internal slice, not as the complete release.

A full profile is optional for onboarding; a confirmed base resume supplies initial facts. Skill self-assessment is optional and is never inferred from keywords. Defer interview notes, advanced analytics, account-wide export/deletion and caching until the core document workflow is reliable.

[Section 34](#34-development-roadmap-and-acceptance-criteria) defines delivery order. The foundation OpenAPI and migrations are already present; the studio services and frontend are still target designs.

---

# 4. Page Structure

## 4.1 Login Page

Route:

```text
/login
```

Functions:

- email login
- password login
- register account
- basic validation
- display authentication errors

Backend concepts:

- JWT authentication
- bcrypt password hashing
- login rate limiting
- authentication middleware

---

## 4.2 Create — default home

Route: `/create`. `/` redirects here after login. Top navigation: **Create / My applications**; the account menu contains Profile, My resumes, AI settings. No permanent sidebar, metric-card grid or dashboard is required for the core release.

One glass composer accepts a job description, URL or screenshot, then displays the selected base resume and the primary **Create my drafts** action. A compact confirmation state resolves extracted company/title and uncertain facts before **Generate drafts**. AI settings are reachable without losing saved input.

The two-document workspace is `/studio/:workspaceId?document=resume` (or `cover_letter`). Document text occupies the main area, with a narrow refinement area on desktop and collapsible controls on mobile. The [design specification](docs/design/DESIGN.md) covers English copy, responsive behaviour and states.

---

# 5. Applications Page

Route:

```text
/applications
```

Functions:

- create application
- search applications
- filter by status
- filter by company
- filter by date
- sort by latest update
- open job details
- delete/archive application

---

# 6. Job Intake and Document Workspace

Routes: `/create`, `/create/:workspaceId`, `/studio/:workspaceId`.

- Source input is a tagged choice: text, public HTTPS URL, or ordered screenshot files.
- Extract company/title/JD first; ask only for missing or uncertain required fields.
- Reuse or upload a confirmed base resume. Profile completion is optional.
- Show the selected AI provider and data categories sent for this operation.
- Store immutable source revisions; generate Resume and Cover letter independently.
- Review evidence, edit text, ask for changes, apply a candidate, export the chosen revision.
- **Save to my applications** creates or returns one linked tracker record. Exporting does not mark it Applied.

Location, salary, source, date applied and notes belong in progressive job-detail editing; they do not block initial drafting. Unreadable links fall back to text/screenshots; uncertain OCR is confirmed by the user. No automatic third-party submission or email sending.

---

# 7. Job Detail Page

Route:

```text
/applications/:id
```

This supporting page tracks the application and links back to its document workspace and the exact submitted document revisions.

## 7.1 Overview

```text
Company
Role
Location
Application status
Job URL
Date applied
Last updated
```

## 7.2 Job Description

Functions:

- view JD
- edit JD
- copy text
- analyse job description
- re-run analysis

## 7.3 Skill Analysis

This optional later feature is not a prerequisite for document generation. Matching requires both extracted JD requirements and a user-maintained skill profile. For skill matching, users explicitly record their skills and self-assessed proficiency. An absent skill means **not assessed**, not proof that the user lacks it. Users may explicitly mark a skill as new to them.

Keep technical proficiency separate from confidence explaining a topic in an interview. A CV mentioning a technology does not establish proficiency, and a missing CV keyword does not establish a missing skill.

Display the JD evidence for extracted requirements and label matches as based on self-assessment, not an objective hiring score. Normalise aliases such as `Go` and `Golang` to one canonical skill.

Example:

```text
Required Skills

Strong Match
✓ Go
✓ REST APIs
✓ PostgreSQL
✓ Docker

Need Revision
△ Redis
△ Kubernetes
△ Nginx

Not Assessed
? Kafka

Marked as New by User
✗ Terraform
```

## 7.4 Interview Preparation

```text
Recommended Topics

High Priority
- Go concurrency
- Redis caching
- SQL indexing

Medium Priority
- Docker
- Kubernetes basics
```

Each topic can have a confidence state:

```text
Don't Know
Learning
Can Explain
Confident
```

## 7.5 Resume / Cover Letter

Link the application to its workspace and the selected immutable resume/letter revisions. The studio supports generation, direct editing, AI candidate revisions, comparison with the base resume, and PDF/DOCX export in the core release. The original resume remains unchanged. Updating a workspace does not silently change which versions are attached to a tracked application.

Detailed schemas and lifecycle: [Application Studio](docs/architecture/application-studio.md).

## 7.6 Interview Notes

```text
Round: Technical Interview

Questions:
- What is Redis?
- How would you reduce database traffic?
- Explain Go concurrency.
- What does Nginx do?
- What is middleware?

Notes:
Need to review rate limiting and connection pools.
```

These questions feed into global interview-topic analytics.

---

# 8. Background Task Progress

Display real backend stages:

```text
Queued → Extracting / Generating / Exporting (task-specific) → Validating → Saving → Completed
```

If analysis uses a single model request, show an indeterminate indicator during that request. Do not invent extraction sub-stages or exact percentages such as 72%. Separate stages are valid only when the backend actually performs separate operations. Any future percentage must be labelled as a stage-based estimate, not elapsed-work precision.

Flow:

```text
Worker → Persist current task state in PostgreSQL
       → Redis Pub/Sub notification → Go SSE handler → Nginx → React
```

PostgreSQL is the source of truth. Pub/Sub is only a live notification mechanism; disconnected subscribers lose messages.

On connection, subscribe before fetching the current state, then send a snapshot. Events include a monotonically increasing state version; clients ignore older versions. Reconcile current state periodically while connected so a missed final notification cannot leave the UI running forever. On reconnect or page refresh, fetch the current snapshot again. Full event replay is outside the first-release scope.

Close the client stream on terminal states. Support heartbeat messages, handler cleanup on disconnect, and a polling fallback when streaming is unavailable.

---

# 9. Interview Prep Page

Route:

```text
/interview-prep
```

Possible topics:

```text
Go
Redis
PostgreSQL
HTTP
Docker
Kubernetes
Nginx
Concurrency
System Design
Security
```

Each topic can contain:

- definition
- key concepts
- common interview questions
- personal notes
- interview confidence level (separate from technical proficiency)
- jobs requiring this skill
- interview frequency

---

# 10. Analytics Page

Route:

```text
/analytics
```

Statistics:

### Application statistics

```text
Total applications
Applications this week
Applications this month
Response rate
Interview rate
Offer rate
```

### Technology statistics

```text
Most requested skills:
Go
React
AWS
Docker
SQL
```

### Interview statistics

```text
Most frequently asked topics:
Redis
Concurrency
Database design
HTTP
Kubernetes
```

---

# 11. Settings Page

Route:

```text
/settings
```

Functions (profile and AI access support the core document release; broader account features are staged):

- optional personal profile at `/profile`: basic information, preferences, education, work/projects, and self-assessed skills
- base resume library at `/resumes`: upload, confirm extracted facts, select default and retain revisions
- account settings
- change password
- preferred job types
- preferred technologies
- personal AI settings at `/settings/ai`: provider/model, encrypted user API Key, save/test/enable/revoke, usage limits
- optional explicit platform-key mode; no automatic fallback from a personal Key

Detailed pages, fields, APIs and credential lifecycle: [Profile and AI settings design](docs/architecture/profile-and-ai-settings.md).
- delete account
- export data

---

# 12. Frontend Technology Stack

- React
- TypeScript
- Vite
- React Router
- Fetch + TanStack Query
- EventSource for SSE

Routes:

```text
/login, /register
/create, /create/:workspaceId
/studio/:workspaceId?document=resume|cover_letter
/applications, /applications/:id
/profile, /resumes, /settings/ai
```

Interview preparation and analytics routes are added only in their delivery phase. All interface copy defaults to English. See the approved design rather than earlier prototype navigation.

---

# 13. Backend Technology Stack

## Go

Primary backend language.

Good for:

- REST APIs
- goroutines
- channels
- worker pools
- high-concurrency services
- simple deployment

## Gin

Used for:

- routing
- request validation
- middleware
- JSON responses
- SSE endpoints

---

# 14. Backend Architecture

Use the feature-oriented modular monolith in [ARCHITECTURE.md](ARCHITECTURE.md) and the detailed [studio module/file responsibilities](docs/architecture/application-studio.md). API and Worker are separate processes within one Go module.

```text
HTTP middleware → feature handler → use case → transactional PostgreSQL adapter
                                              → outbox → bounded Worker
                                                       → parser/provider/export adapter
```

Core modules: auth, profile, aisettings, attachment, intake, resume, studio, document, task and application. No generic handler/service/repository mega-packages; no request-scoped goroutine used as a durable job.

---

# 15. Database Design

Use PostgreSQL with migrations, foreign keys, ownership checks, and timestamps stored consistently in UTC.

## Foundation and supporting tables

The core document pipeline additionally requires files, job_sources/job_revisions, resumes/resume_revisions, workspaces, generation_runs, documents/document_revisions, job_tasks/task_outbox, document_exports and application_documents. Their relationships and concurrency constraints are authoritative in [Studio data design](docs/architecture/application-studio.md#3-数据设计).

The following older analysis tables describe the later skill-analysis domain. Do not implement a second task runtime: status, retries, leases and outbox for all new async work live in job_tasks/task_outbox. analyses will store domain inputs/results linked to a task, rather than duplicate execution state.

Additional profile, preferences, education/experience, AI settings/revisions/credentials and test-idempotency tables are specified in [Profile and AI settings](docs/architecture/profile-and-ai-settings.md).

| Table | Main fields and purpose |
|---|---|
| `users` | `id`, unique normalised `email`, `password_hash`, timestamps |
| `applications` | `id`, `user_id`, company, role, location, URL, JD, `jd_version`, status, source, salary text, `date_applied`, `first_response_at`, notes, archive timestamp, timestamps |
| `application_status_history` | application, previous/new status, `occurred_at`, `recorded_at`; preserves funnel history |
| `skills` | canonical skill name and identifier |
| `skill_aliases` | unique normalised alias mapped to a canonical skill |
| `user_skills` | user, skill, self-assessed proficiency, notes, timestamps; unique user/skill pair |
| `analyses` (later) | application, task_id, immutable JD/profile snapshot versions, validated skill result; execution state belongs to job_tasks |
| `analysis_skills` | analysis, canonical skill, requirement category, JD evidence; supports cross-job queries |
| `task_outbox` | task, delivery generation, dispatch claim and retry state; durable enqueue intent shared by all task kinds |
| `idempotency_requests` | owner, operation, key hash, request hash, resource/response references, expiry; unique owner/operation/key |
| `interview_rounds` | application, round name, interview date, notes, timestamps |
| `interview_questions` | interview round, question, answer notes, timestamps |
| `question_topics` | question and canonical topic; unique pair for reliable counting |
| `study_topics` | canonical topic name, optional linked skill |
| `user_study_topics` | user, topic, interview confidence, notes, timestamps; unique user/topic pair |

Use an explicit `not_assessed` state for skills without a recorded assessment. Persist the profile snapshot used in a comparison so later profile edits do not silently change the meaning of an old result.

## Private files and document versions (core release)

Files are required in the core release for JD images, base resumes and exports. Immutable binary files are stored privately; extracted facts and editable document revisions are stored in PostgreSQL. Owner checks cover upload, parse, preview, download and all indirect references. Export is a separate task pinned to a saved document revision, format and template version. See [Studio architecture](docs/architecture/application-studio.md).

## Shared task state model

```text
queued → running → completed
                 → retry_wait → queued
                 → failed
queued / retry_wait → cancelled
running → cancelled (after the worker observes cancellation)
```

Terminal states cannot be overwritten by delayed workers. Use conditional updates with a fencing token/state version to reject stale attempts. `cancel_requested` is a flag, not a promise that an in-flight provider call has stopped.

A partial unique constraint permits at most one active analysis per application. Re-running a completed analysis creates a new record. A changed JD increments `jd_version`; old results remain historical and cannot become the current result for the new version.

## Analytics definitions

- The later analytics status breakdown counts current, non-archived applications.
- Funnel metrics use a cohort of applications whose `date_applied` falls in the selected period; all numerator and denominator counts use the same cohort.
- Response rate: applications with a manually recorded `first_response_at` / applications in the cohort. The field represents an employer response beyond an automatic receipt.
- Interview rate: applications with at least one recorded interview / applications in the cohort.
- Offer rate: applications that reached Offer in status history / applications in the cohort.
- Count each application once per funnel metric; display N/A when the denominator is zero. Cohort outcomes may evolve over time.
- Required skill frequency counts distinct applications using their latest completed analysis matching the current JD version. Show how many applications have an eligible analysis.
- Interview topic frequency counts recorded questions tagged with each canonical topic. A question tagged with several topics contributes once to each.

Status changes and their history rows must commit in one transaction. Do not infer historical conversions solely from current status.

---

# 16. Redis Responsibilities

## Queue and notifications: core analysis release

```text
API transaction: task(s) + outbox record(s) + idempotency record
    ↓
Outbox dispatcher publishes task ID to Redis queue
    ↓
Worker claims task, processes it, persists state, acknowledges delivery
```

The dispatcher retries pending outbox entries. A crash between enqueue and marking dispatched may cause duplicate delivery; workers must handle this safely. A recovery scan finds overdue nonterminal tasks and redispatches eligible work if Redis loses queued data.

Use one established Redis-backed queue implementation with acknowledgement, retry, and recovery support. Confirm its delivery and persistence behaviour before adopting it. Do not use Pub/Sub as the task queue or rely on destructive pop without a recovery mechanism.

Pub/Sub publishes state-change notifications after database commits. Missed notifications are repaired through snapshot reconciliation described in Sections 8 and 20.

## Rate limiting

Apply account/IP login throttling in the first release. Use Redis-backed shared counters before running multiple API instances. Analysis submission limits, queue admission limits, and provider request/token limits serve different purposes and must be configured separately.

## Cache: measured optimisation phase

First measure PostgreSQL with pagination and appropriate indexes. Add Redis caching only to endpoints with a demonstrated benefit.

Example key: `user:1001:application:123:v1`. Verify resource ownership before returning data, including cache hits. Invalidate on relevant writes; consider concurrent stale repopulation and use a documented consistency policy. A TTL alone does not guarantee freshness.

Cache failure may fall back to PostgreSQL with load protection. Queue failure must not silently lose accepted tasks. Keeping these workloads in one Redis instance is acceptable for development, but memory and eviction policies must protect queue data; split cache and queue storage if required before a broader deployment.

---

# 17. Worker System and Analysis Contract

## Analysis approach

The core release parses job sources and resumes, then uses bounded provider adapters to tailor a resume and write a cover letter. These are separate typed operations with separate schemas and tasks, sharing one reliable runtime. Resume facts are confirmed before generation; neither JD keywords nor model output establish personal proficiency.

The following skill extraction schema belongs to optional later interview preparation. Canonical skill matching remains deterministic and does not gate document drafting. The core generation schemas are specified in [Application Studio](docs/architecture/application-studio.md).

Define a versioned result schema containing:

- required and preferred skills with supporting JD excerpts
- canonical skill mappings or unresolved names for review
- suggested interview topics with a short rationale
- explicit unknowns rather than invented personal abilities

Validate the provider response against the schema before saving. Treat the JD as untrusted content, not instructions; do not give the analysis model tools or permission to perform actions. Suggested topics are preparation aids, not predictions of actual interview questions.

Bound input length, output size, request duration, retries, and per-user usage. Record latency and available token/cost metadata without logging JD/CV contents or secrets. Use a small curated JD fixture set to evaluate extraction and evidence quality, and a mock adapter for repeatable automated tests and load tests.

## Worker reliability

- Configure a bounded number of concurrent handlers in the chosen queue system; avoid stacking an unnecessary second in-memory queue/pool on top.
- Claim an eligible task atomically, increment its attempt/fencing token, and maintain a renewable lease.
- Retry transient failures with capped exponential backoff and jitter; respect provider retry guidance. Treat invalid input and permanent configuration errors as failures rather than retrying indefinitely.
- Cap total attempts and execution time. Expose a safe error and allow a deliberate new run after terminal failure.
- Recover expired leases with a scheduled scan. A worker whose lease/token is stale cannot commit a result.
- Check cancellation before the external call, between stages, and in the final conditional commit. Persist cancellation requests so they work across processes.
- On graceful shutdown, stop claiming work and finish or release current work within a deadline.

A crash after the provider completes but before the database commit can still cause another paid request. Local idempotency prevents duplicate application results; it does not promise exactly-once external billing. Use provider idempotency where available and enforce cost/retry limits.

Browser disconnection must not cancel the background analysis. A running cancellation is best effort for the external request; cancelled work cannot subsequently replace a saved terminal state.

---

# 18. Why Bounded Concurrency Matters

Bad:

```text
1000 jobs
↓
1000 goroutines
↓
1000 AI/API requests
↓
database / external service overloaded
```

Better:

```text
1000 jobs
↓
Queue
↓
5–20 workers
↓
controlled processing
```

This bounds active work, but does not by itself bound queue growth or enforce requests per minute. Add a maximum backlog/admission policy and a separate provider rate limiter. Consider all worker replicas when calculating total concurrency. Reject excess submissions with an actionable response instead of allowing an unlimited backlog.

---

# 19. API Design

`api/openapi.yaml` is the executable foundation and Studio contract (0.5.0), with an implemented text-JD and mock dual-document workflow. Fixed-revision PDF/DOCX exports are implemented; automatic resume extraction and real AI remain pending; the English glass frontend is connected to the mock workflow. Studio endpoints for files, sources, resumes, workspaces, documents, tasks, export and tracking are specified in the Studio contract in [Application Studio](docs/architecture/application-studio.md#7-http-协议草案). The analysis/interview endpoints below are later feature sketches; task progress/cancellation will use the shared `/api/tasks/{id}` family.

## Authentication

```text
POST /api/auth/register
POST /api/auth/login
POST /api/auth/logout
```

## Applications

```text
GET    /api/applications
POST   /api/applications
GET    /api/applications/:id
PATCH  /api/applications/:id
POST   /api/applications/:id/archive
```

## Analysis

```text
POST /api/applications/:id/analyse
GET  /api/analyses/:id
GET  /api/tasks/:id/events
POST /api/tasks/:id/cancel
```

## Skill Profile

```text
GET    /api/me/skills
PUT    /api/me/skills/:skill_id
DELETE /api/me/skills/:skill_id
```

## Interview

```text
POST /api/applications/:id/interviews
GET  /api/applications/:id/interviews
POST /api/interviews/:id/questions
```

Analysis submission returns `202 Accepted` with the analysis ID and status URL only after durable acceptance. Idempotent replay returns the existing task. Cancellation returns the actual current state if the task has already completed; otherwise it records cancellation intent. Define typed error responses and validate ownership through the parent application for every analysis/interview endpoint.

Core private-file and document-export endpoints are specified in the Studio contract draft. Topic management and account-wide export/deletion remain later scope.

## Analytics

```text
GET /api/analytics/overview
GET /api/analytics/skills
GET /api/analytics/interview-topics
```

---

# 20. SSE and Authentication

Endpoint:

```text
GET /api/tasks/{id}/events
```

Example snapshot or update:

```text
event: task_state
data: {"task_id":"uuid","state_version":7,"status":"running","stage":"generating"}

```

Use the snapshot/version reconciliation contract in Section 8. This is a current-state stream, not a replayable event log. SSE handlers verify task ownership, send heartbeats, flush updates, release subscriptions on disconnect, and periodically reconcile persistent state. The frontend closes terminal streams and can fall back to bounded polling.

Serve the frontend and API from the same origin. For the MVP, store a short-lived authentication JWT in an HttpOnly cookie, with Secure enabled in HTTPS environments and an appropriate SameSite policy. This allows native EventSource to authenticate without a custom Authorization header. Implement CSRF protection/origin validation for state-changing requests; do not put long-lived tokens in URLs.

Logout clears the cookie, but a stateless JWT remains valid until expiry if copied. The MVP may require login again when it expires; immediate session revocation requires additional server-side state. Close SSE connections at authentication expiry and require reauthentication.

Disable proxy buffering and caching for SSE routes, configure heartbeat-compatible proxy timeouts, and test progress through Nginx rather than only against the Go server.

References: [Redis Pub/Sub delivery semantics](https://redis.io/docs/latest/develop/pubsub/) and [EventSource constructor](https://developer.mozilla.org/en-US/docs/Web/API/EventSource/EventSource).

---

# 21. Nginx

Nginx sits in front of the application.

```text
Internet
   ↓
Nginx
   ↓
 ┌───────────────┐
 ↓               ↓
React          Go API
```

Responsibilities:

- reverse proxy
- serve static frontend files
- HTTPS / TLS termination
- rate limiting
- request size limits
- traffic control
- future load balancing

Routes:

```text
/       → React frontend
/api/*  → Go backend
```

---

# 22. Security

Key goals:

## Authentication

- short-lived JWT in an HttpOnly cookie, following Section 20
- bcrypt password hashing
- token expiration and documented logout behaviour
- CSRF/origin checks for state-changing cookie-authenticated requests
- login throttling and safe authentication errors

These controls, ownership checks, and input validation belong in MVP 1, not a later hardening phase.

## Authorisation

Every application belongs to a user.

Backend must verify:

```text
application.user_id == authenticated_user_id
```

Apply the same ownership rule through parent relationships to analyses, SSE, interview records, attachments, and cache hits. Include cross-account access tests from the first release.

## Input Validation

Validate:

- email
- URLs
- string lengths
- status values
- file type
- file size

## SQL Injection Prevention

Use parameterised SQL.

```sql
SELECT *
FROM users
WHERE email = $1;
```

## Rate Limiting

Possible limits:

```text
Login:
5 attempts / minute / IP

Analysis:
10 requests / minute / user

General API:
100 requests / minute / user
```

Return:

```text
HTTP 429 Too Many Requests
```

## File Security

For CV uploads:

- restrict file types
- restrict size
- randomise stored filename
- never trust original filename
- do not expose local filesystem paths

## Secrets

Do not hardcode:

```text
JWT secret
database password
AI API key
```

Use environment variables.

---

# 23. Database Traffic Optimisation

## 23.1 Indexing

Example:

```sql
CREATE INDEX idx_applications_user_id
ON applications(user_id);
```

Composite index:

```sql
CREATE INDEX idx_applications_user_status
ON applications(user_id, status);
```

Inspect with:

```sql
EXPLAIN ANALYZE
```

## 23.2 Redis Cache

Optional after measuring a database-only baseline. Include invalidation correctness, cache hit rate, and write-heavy behaviour in the evaluation.

```text
1000 GET requests
        ↓
      Redis
        ↓
 only cache misses
        ↓
   PostgreSQL
```

## 23.3 Pagination

```text
GET /api/applications?page=1&limit=20
```

## 23.4 Connection Pool

```go
db.SetMaxOpenConns(50)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)
```

The values above are illustrative. Budget connections across all API and worker replicas against PostgreSQL capacity, then tune using measurements.

Important idea:

```text
10,000 requests
does not mean
10,000 database connections
```

---

# 24. Cache Problems to Explore

## Cache Penetration

Requests repeatedly ask for data that does not exist.

Possible solution:

- cache negative results briefly

## Cache Breakdown

A hot key expires and many requests hit the database simultaneously.

Possible solutions:

- singleflight
- locking
- stale cache

## Cache Avalanche

Many keys expire at the same time.

Possible solution:

- randomised TTL

---

# 25. Go Concurrency Topics

Practise:

- goroutines
- channels
- context
- worker pools
- mutex where appropriate
- cancellation
- timeouts
- bounded concurrency

Use timeouts for:

- database
- Redis
- external APIs
- analysis tasks

---

# 26. Idempotency

Idempotency is required when asynchronous analysis and retries are introduced.

```text
POST /api/applications/:id/analyse
Idempotency-Key: abc123
```

Scope keys by authenticated user and operation, and store a hash of the request including the application and JD/profile/AI-configuration versions. The same key and payload return the same analysis; the same key with a different payload returns a conflict. Record the key, analysis, and outbox entry atomically.

Use a documented retention period longer than the expected client retry window. Independently prevent multiple active analyses for the same application, since repeated clicks may use different keys.

Queue delivery may occur more than once. Atomic claims, fencing tokens, terminal-state checks, and transactional result writes prevent duplicate or stale database effects. These guarantees do not imply exactly-once provider execution.

---

# 27. Logging

Structured logging starts in MVP 1. Never log passwords, tokens, full JD/CV contents, or raw provider responses by default.

Each request should log:

```text
request_id
user_id
method
path
status_code
latency
```

Example:

```text
request_id=abc123
method=GET
path=/api/applications
status=200
latency=42ms
```

---

# 28. Monitoring

Add queue/task metrics with asynchronous analysis; add full dashboards in the performance phase. Useful metrics:

```text
requests per second
error rate
p50 latency
p95 latency
p99 latency
Redis hit rate
database connections
queue length
worker utilisation
oldest queued task age
retry / failure counts
provider latency / rate-limit responses
provider usage / estimated cost
```

Optional tools:

- Prometheus
- Grafana

---

# 29. Load Testing

Use:

```text
k6
```

or:

```text
hey
wrk
```

Example scenario:

```text
100 concurrent users
10,000 requests
```

Measure:

```text
throughput
average latency
p95 latency
p99 latency
error rate
CPU
memory
database connections
```

---

# 30. Performance Experiments

Run real before/after tests with a fixed dataset, documented hardware, query mix, warm-up, duration, and concurrency. Include cold and warm cache cases and report error rates alongside latency.

Use a mock analysis provider for repeatable load tests. Real provider smoke tests are separate and have a small explicit cost budget. Admission limits and worker throughput need separate scenarios; a fast job-submission endpoint does not imply fast analysis completion.

Do not assume Redis or higher concurrency will improve the result. Record no improvement and regressions as valid outcomes.

## Experiment A

```text
PostgreSQL only
```

## Experiment B

```text
PostgreSQL + index
```

## Experiment C

```text
PostgreSQL + Redis cache
```

## Experiment D

```text
Rate limiting enabled
```

## Experiment E

```text
5 workers vs 20 workers
```

Record results and explain trade-offs.

---

# 31. Docker

Production-style Compose services:

```text
backend
worker (includes outbox dispatch and recovery loops)
postgres
redis
nginx (serves built frontend assets and proxies /api)
```

Use a frontend development server only in the development profile. Configure health checks, database/Redis persistence, migrations, and bounded graceful shutdown. Document database backup/restore and private-file storage before storing real personal data.

Run everything with:

```bash
docker compose up
```

---

# 32. Full Architecture

```text
Browser → Nginx → React static assets
               → Go API → PostgreSQL
                            ├─ application / skill / interview data
                            └─ analysis + outbox + idempotency transaction

Outbox dispatcher → Redis task queue → Go workers → Model provider
                         ↑                 ↓
Recovery scan ← PostgreSQL ← persisted state and validated results
                                           ↓
                                  Redis Pub/Sub notification
                                           ↓
Browser ← Nginx ← Go SSE handler ←───────────┘
                      ↑
              PostgreSQL state reconciliation
```

API and worker are separate processes built from the same backend repository. Dispatcher and recovery loops may run in the worker process with atomic claims so additional replicas remain safe. No separate microservices are required for these responsibilities.

Redis-backed rate limiting supports shared quotas. Endpoint caching is an optional measured extension. PostgreSQL remains authoritative for accepted work and task state; Redis notifications never replace persistent state.

---

# 33. Suggested Tech Stack

## Frontend

```text
React
TypeScript
Vite
React Router
Axios / Fetch
```

## Backend

```text
Go
Gin
```

## Database

```text
PostgreSQL
```

## Redis

```text
Caching
Queue
Pub/Sub
Rate limiting
```

## Infrastructure

```text
Docker
Docker Compose
Nginx
```

## Authentication

```text
JWT
bcrypt
```

## Real-Time

```text
Server-Sent Events
```

## Testing

```text
Go testing
Postman
k6
```

## CI/CD

Future:

```text
GitHub Actions
```

---

# 34. Development Roadmap and Acceptance Criteria

## Foundation — already started

OpenAPI 0.5.0, six migrations, migration runner and contract/database verification are present. Auth/workspace/private storage/manual resume/text-JD/document handlers and a PostgreSQL-backed mock Worker are implemented; fixed-revision PDF/DOCX exports are implemented; the English glass UI is connected, while automatic resume extraction and real AI remain pending. Keep existing migrations immutable.

## Core slice 1 — Workspace, facts and draft lifecycle

Extend the next contract/schema batch for sources, resume facts, workspaces, document revisions and shared tasks. Implement auth/CSRF, ownership, private uploads, basic profile, confirmed base resumes and the text-JD path. Use a mock provider to exercise two document drafts, manual editing, candidate review and one export path before adding live model complexity.

Acceptance: two-account isolation, snapshot/version consistency, refresh recovery, retained user edits, and a readable exported document. This internal slice is not the complete promised input support.

## Core slice 2 — Complete AI document workflow

Add personal AI save/test/enable/revoke, immutable configuration revisions, bounded provider calls, screenshot extraction, safe public-link reading, PDF/DOCX import and export. Reuse one task/outbox/lease/fence/recovery runtime. Adopt the approved English frosted-glass UI, independent document retries and optional Save to my applications.

Acceptance:

- Screenshot, text and link inputs reach reviewable drafts; unreadable inputs have a useful fallback.
- Resume and cover letter use confirmed facts; unsupported claims prompt for input.
- Concurrent same-key requests create one run; stale workers cannot overwrite user revisions.
- One failed document leaves the other usable; retry does not regenerate successful work.
- Worker crashes, queue loss, missed SSE events, cancellation and credential revocation recover correctly.
- PDF and DOCX exports match the selected saved revision and remain private.
- Export does not mark a job Applied; repeated tracking creates only one application.
- English desktop/mobile UI, reduced transparency, contrast and keyboard use are verified.

This is the first complete product release. Demonstrate the main flow and failure recovery behind Nginx/Compose before expanding scope.

## Release 3 — Interview and search depth

Add skill matching, interview rounds/questions, study topics and carefully defined analytics. Extend profile editors as needed. Account export/deletion must include all studio snapshots, files and active tasks before claiming support.

## Release 4 — Measured performance

Use reproducible load tests, query plans and pool/concurrency measurements. Add caching only when justified. Report workload, environment, error rate and p95/p99 alongside throughput; quality and ownership correctness remain mandatory.

## Planning estimate

Earlier tracker-based 2–3 / 4–6 / 8–12 week ranges are withdrawn for this scope. Parsing, multimodal intake, version-aware generation and reliable PDF/DOCX export need a new task-level estimate. Do not reuse the old dates as commitments.

---

# 35. Repository Structure

The maintained target tree is [ARCHITECTURE.md](ARCHITECTURE.md#4-目标目录与文件职责). The detailed studio files are in [Application Studio](docs/architecture/application-studio.md#2-模块与文件职责) and frontend feature files in [Delivery and frontend](docs/architecture/delivery-and-frontend.md#1-前端目标目录). Keep those sources authoritative instead of maintaining a second contradictory tree.

---

# 36. Interview Stories This Project Can Create

### Database traffic

Question:

> How would you reduce database load?

Real project answer:

- indexing
- Redis caching
- pagination
- connection pooling
- async processing
- queue-based traffic smoothing

### Concurrency

Question:

> Have you worked with Go concurrency?

Real project answer:

- goroutines
- channels
- worker pool
- bounded concurrency
- context cancellation

### Middleware

Question:

> What middleware have you implemented?

Real project answer:

- authentication
- logging
- rate limiting
- panic recovery
- request IDs

### Security

Question:

> How did you secure your backend?

Real project answer:

- password hashing
- JWT
- authorisation checks
- rate limiting
- validation
- parameterised SQL
- secret management
- HTTPS through Nginx

### Redis

Question:

> Why did you use Redis?

Real project answer:

- caching
- background job queue
- Pub/Sub
- distributed rate limiting

### Nginx

Question:

> Why use Nginx?

Real project answer:

- reverse proxy
- frontend static files
- TLS termination
- traffic control
- rate limiting
- future load balancing

---

# 37. Resume Description

The following are templates to use only after the corresponding features have been implemented and verified. Add performance claims only when supported by reproducible measurements.

## Short Version

**ApplyFlow — Job Application & Interview Preparation Platform**

Built a full-stack job tracking and interview preparation platform using React, Go, PostgreSQL and Redis. Designed asynchronous job-description analysis with Redis-backed workers and SSE progress updates, and implemented JWT authentication, caching, rate limiting and Nginx reverse proxying.

## Backend-Focused Version

**ApplyFlow — Backend-Focused Job Application Platform**

Designed and implemented a Go backend for job application tracking and asynchronous job-description analysis. Used PostgreSQL for persistent storage and Redis for caching, job queues, Pub/Sub and rate limiting. Built a bounded worker pool using goroutines and channels, streamed task progress through SSE, and deployed services behind Nginx with JWT authentication and traffic controls.

---

# 38. Main Learning Goals

By completing ApplyFlow, you should get practical experience with:

```text
REST APIs
Go backend development
Gin
PostgreSQL
SQL indexing
Redis
Caching
Queue systems
Pub/Sub
Goroutines
Channels
Worker pools
Concurrency control
SSE
JWT
Authentication
Authorisation
Middleware
Rate limiting
Nginx
Docker
Load testing
Database optimisation
Security
System design
```

---

# 39. Final Product Vision

On the surface, ApplyFlow is:

> A focused AI workspace that turns a job description and real experience into a tailored resume and cover letter, ready to review and export.

Underneath, it is a backend engineering playground:

```text
traffic arrives
↓
Nginx controls entry
↓
middleware authenticates and limits requests
↓
Go handles business logic
↓
Redis protects and coordinates the system
↓
PostgreSQL stores reliable data
↓
workers process expensive tasks asynchronously
↓
SSE updates the browser in real time
↓
load tests reveal bottlenecks
↓
performance improvements can be measured
```

The goal is not to add as many technologies as possible.

The goal is to understand **why each technology exists, what problem it solves, and what trade-offs it introduces.**
