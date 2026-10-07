# ApplyFlow — Job Discovery Integration Plan

> 这份文档说明如何在当前 ApplyFlow 上新增 **Job Discovery / Job Radar**，同时保留现有 Application Studio 主链路。
>
> 当前项目已实现认证、workspace、私有 PDF/DOCX、手动简历确认、文本 JD、独立 Worker、双文档版本、任务取消/重试与固定版本 PDF/DOCX 导出；当前 Worker 直接消费 PostgreSQL durable outbox。Redis/Asynq、SSE、截图/链接解析、自动简历解析和真实 AI 仍未实现。

---

# 1. 目标

新增一条用户流程：

```text
Discover suitable jobs
        ↓
Review / save a job
        ↓
Send selected job into existing ApplyFlow Studio
        ↓
Generate tailored Resume + Cover Letter
        ↓
Review / edit / export
        ↓
Optionally track application
```

产品主线可以变成：

> **发现职位 → 定制 Resume / Cover Letter → 审阅 → 导出 → 可选申请跟踪**

现有 Application Studio 继续作为核心文档生成流程，不推翻重做。

---

# 2. 新功能定位

新增主入口：

```text
Discover Jobs
```

推荐 route：

```text
/discover
```

或：

```text
/jobs/discover
```

页面示例：

```text
Discover Jobs

Backend Engineer
Company A
Sydney

Strong signals
✓ Go
✓ PostgreSQL
✓ REST APIs

Requires review
? AWS
? Kubernetes

Location
✓ Sydney

[Save] [Create Drafts] [Open Job] [Ignore]
```

---

# 3. 不建议一开始做“87% Match”

除非这个分数有非常明确、可测试的计算公式，否则不要展示：

```text
Match: 87%
```

更推荐展示可解释的 evidence-based signals：

```text
Strong signals
✓ Go
✓ PostgreSQL
✓ REST APIs

Requires review
? AWS
? Kubernetes

Not assessed
? Kafka
```

原因：

- CV 里出现关键词，不等于用户真的熟练。
- CV 里没出现关键词，不等于用户不会。
- 用户技能应优先来自用户确认 / self-assessment。
- 不确定就显示 unknown / not assessed。

如果以后真的需要数字评分，要把计算规则写清楚。

---

# 4. 主用户流程

```text
User opens Discover Jobs
        ↓
System imports / fetches job listings
        ↓
Normalise job data
        ↓
Deduplicate obvious duplicates
        ↓
Evaluate against explicit preferences
        ↓
User reviews a job
        ↓
Save / Ignore / Open original / Create Drafts
        ↓
Selected job becomes an existing JD source
        ↓
Existing Intake / Workspace flow
        ↓
Resume + Cover Letter generation
        ↓
Review / edit / export
        ↓
Optional application tracking
```

关键点：

```text
Job Discovery ≠ Application Studio
```

二者相连，但应该是两个独立业务关注点。

---

# 5. 现有模块继续复用

尽量复用现有：

```text
intake
resume
studio
document
generation
files
task
application
```

新功能只负责把一个 discovered job 转换成现有 Studio 能消费的 JD source / workspace 输入。

不要重复实现：

- generation logic
- document revision logic
- export logic
- task runtime
- application tracking
- worker lifecycle

推荐集成点：

```text
Discovered Job
      ↓
Create / reuse JD source
      ↓
Create workspace
      ↓
Existing Studio generation flow
```

---

# 6. 新业务模块

推荐新增：

```text
backend/internal/discovery/
├── model.go
├── ports.go
├── service.go
├── validation.go
├── errors.go
└── service_test.go
```

如果想写得更明确，也可以叫：

```text
jobdiscovery/
```

但 `discovery` 更简洁。

---

# 7. discovery 模块负责什么

负责：

- discovered job 生命周期
- source metadata
- company / title / location 规范化
- duplicate handling
- save / ignore 状态
- preference filtering
- discovered job → Studio JD source 转换
- 基础的可解释匹配信号
- ownership / validation

不要直接依赖：

```text
Gin
pgx
sqlc
Redis
Asynq
具体 AI SDK
具体爬虫库
```

继续遵守现有依赖方向：

```text
transport
   ↓
business
   ↑
adapters
```

---

# 8. 新 Ports

例如在：

```text
internal/discovery/ports.go
```

概念上可以有：

```go
type JobStore interface {
    SaveDiscoveredJob(...)
    ListDiscoveredJobs(...)
    GetDiscoveredJob(...)
    MarkSaved(...)
    MarkIgnored(...)
}

type JobSource interface {
    FetchJobs(...)
}

type StudioStarter interface {
    CreateWorkspaceFromJob(...)
}
```

具体签名按你现在项目风格写。

规则：

> 接口定义在使用它的业务模块，具体实现放 adapters。

---

# 9. Job Source Adapters

推荐新增：

```text
backend/internal/adapters/jobsource/
├── manual.go
├── email.go
├── rss.go
└── api.go
```

以后再按需要加：

```text
company_careers.go
approved_provider.go
```

结构：

```text
discovery service
        ↓
JobSource interface
        ↑
manual adapter
email adapter
RSS adapter
approved API adapter
```

这样换来源不需要改业务逻辑。

---

# 10. 第一版职位来源建议

第一版优先支持：

```text
手动 URL 导入
手动 JD 文本导入
Email job alerts 导入
RSS / feed
允许的公司 careers 页面
批准的 API
```

不要让整个产品依赖某个第三方网站的 scraper 才能工作。

即使没有任何外部 Job Board integration，ApplyFlow 仍然应该能完整使用。

---

# 11. 数据库新增

不要把 discovery 阶段的数据全部塞进 `applications`。

推荐新表：

## 11.1 discovered_jobs

用途：

在职位还没有变成正式 application 之前保存它。

建议字段：

```text
id
owner_id
source_type
source_external_id
source_url
company_name
job_title
location_text
description_text
published_at
discovered_at
content_hash
created_at
updated_at
```

可选：

```text
salary_text
employment_type
remote_type
raw_source_metadata
```

约束：

- 所有 private row 都要 owner scope。
- URL 本身不能作为唯一 identity。
- 有 external ID 时优先使用 source + external ID 去重。
- 保留原始 source provenance。

---

## 11.2 discovered_job_states

用于记录用户对职位的决策：

```text
new
saved
ignored
draft_created
expired
```

如果第一版简单，也可以先直接放在 `discovered_jobs.status`。

---

## 11.3 job_preferences

用于用户显式设置搜索偏好：

```text
user_id
keywords
locations
remote_preference
employment_types
excluded_keywords
minimum_salary_text
updated_at
```

不要自动推断敏感偏好。

---

## 11.4 job_match_evidence（后续）

如果以后需要结构化匹配：

```text
id
discovered_job_id
user_id
signal_type
canonical_name
status
evidence
created_at
```

status 示例：

```text
matched
not_assessed
requires_review
user_marked_new
```

---

# 12. 与现有表的关系

建议流程：

```text
discovered_jobs
      ↓ user clicks Create Drafts
job_sources / job_revisions
      ↓
workspaces
      ↓
generation_runs
      ↓
job_tasks / task_outbox
      ↓
documents / document_revisions
      ↓
exports
```

只有用户明确选择 track / save as application 时，再创建或关联：

```text
applications
application_documents
```

导出 CV 不等于已经申请。

---

# 13. API 新增

建议新增：

```text
GET    /api/discovery/jobs
POST   /api/discovery/import
GET    /api/discovery/jobs/{id}
POST   /api/discovery/jobs/{id}/save
POST   /api/discovery/jobs/{id}/ignore
POST   /api/discovery/jobs/{id}/create-workspace
GET    /api/discovery/preferences
PUT    /api/discovery/preferences
```

后续可加：

```text
POST /api/discovery/sync
GET  /api/discovery/sources
```

继续遵守项目现有规则：

> 先改 `api/openapi.yaml`，再改实现、migration 和测试。

---

# 14. 前端改动

顶部导航建议：

```text
Create
Discover Jobs
My Applications
```

现有 Studio 导航不要破坏。

推荐 route：

```text
/discover
```

---

# 15. Discover Jobs 页面

目标不是做数据大屏，而是方便快速决策：

```text
scan
review
save / ignore
create drafts
```

示例：

```text
Discover Jobs

Filters:
[Backend] [Full Stack] [Sydney] [Remote]

---------------------------------------------------
Backend Engineer
Company A
Sydney

Strong signals
✓ Go
✓ PostgreSQL
✓ REST APIs

Requires review
? AWS
? Kubernetes

[Save] [Create Drafts] [Open Job] [Ignore]
---------------------------------------------------
```

---

# 16. Job Detail 页面 / Drawer

显示：

```text
Company
Title
Location
Original URL
Published date
Source
Full JD
```

再显示：

```text
Relevant signals
User preferences
Unknown / missing information
```

操作：

```text
Save
Ignore
Create Drafts
Open Original Posting
```

---

# 17. 接入现有 Studio

用户点击：

```text
Create Drafts
```

推荐流程：

```text
POST /api/discovery/jobs/{id}/create-workspace
        ↓
validate owner
        ↓
load discovered job
        ↓
create immutable JD source / revision
        ↓
create / initialise workspace
        ↓
return workspace ID
```

前端跳转到：

```text
/studio/{workspaceId}
```

或者现有 create/confirmation route。

Studio 继续保持权威入口。

---

# 18. 后台任务

不要新建第二套 task runtime。

继续复用：

```text
job_tasks
task_outbox
Worker
```

可能的新任务类型：

```text
discover_jobs
normalise_discovered_job
refresh_job_source
evaluate_job_signals
```

简单的 save / ignore 不需要异步任务。

---

# 19. 当前 Worker 怎么接

当前版本：

```text
PostgreSQL task/outbox
        ↓
Worker 直接消费
```

第一版 Job Discovery 就沿用这个方式。

不要求先接 Redis 才能做。

以后目标：

```text
PostgreSQL outbox
        ↓
dispatcher
        ↓
Redis / Asynq
        ↓
Worker
```

因为 task state 仍然在 PostgreSQL，所以 discovery 业务不应该关心底层到底用哪种 dispatch。

---

# 20. Redis 什么时候再加

只在有明确理由时加。

适合：

## Shared rate limiting

```text
external source sync
AI generation submission
login
```

## Queue dispatch

从当前 PostgreSQL outbox direct consumption 升级到 Redis/Asynq。

## Live notification

PostgreSQL commit 后，通过 Redis Pub/Sub 通知 SSE。

## Cache

只缓存真正测出来有收益的 read-heavy endpoint。

PostgreSQL 始终是 source of truth。

---

# 21. SSE 放到后面

Job Discovery 的基本浏览不依赖 SSE。

SSE 比较适合：

```text
source sync progress
large import progress
CV generation progress
CL generation progress
export progress
```

示例：

```text
Sync started
↓
Fetching source
↓
Normalising jobs
↓
Removing duplicates
↓
Completed
```

不要假装显示 72% 这类没有真实依据的进度。

优先显示真实 task stage。

---

# 22. Matching 设计

第一版应该可解释、可验证。

可以比较：

```text
title keywords
preferred location
remote preference
employment type
explicit user technologies
explicit exclusions
```

结果例子：

```json
{
  "strong_signals": ["Go", "PostgreSQL", "REST APIs"],
  "requires_review": ["AWS", "Kubernetes"],
  "location_match": true
}
```

不要自动推断：

```text
用户真实能力
用户一定不会的技能
录取概率
雇主最终决定
```

---

# 23. AI 可以帮什么

AI 适合：

```text
规范化混乱职位名
抽取结构化 JD requirements
总结长 JD
Go / Golang alias mapping
required / preferred 分类
```

要求：

- provider output 要 schema validation
- JD 内容按 untrusted input 处理
- 输出要 evidence-backed
- 用户可 review
- AI 不直接执行 apply

---

# 24. 安全

继续沿用现有 ownership 规则。

所有 private resource query 都必须 owner scoped：

```text
preferences
saved jobs
source imports
workspace
documents
```

对 public URL：

- 只允许 HTTP/HTTPS。
- 防 SSRF。
- 设置 timeout。
- 限制 response size。
- block private/internal IP ranges。
- 不在后端执行任意页面脚本。

JD 内容只当数据，不当 system instructions。

---

# 25. Rate Limiting

建议区分：

```text
HTTP rate limit
queue admission limit
worker concurrency limit
provider rate limit
```

它们不是一回事。

可能配置：

```text
manual import: per-user limit
external sync: low frequency
AI analysis: per-user submission limit
provider: global concurrency + RPM limit
```

---

# 26. Duplicate Detection

同一个职位可能从不同来源出现。

有 external ID 时：

```text
source + external_id
```

没有时可组合：

```text
normalised company
+
normalised title
+
location
+
description hash
```

不确定时不要静默 merge。

可以标记：

```text
possible duplicate
```

并保留 source provenance。

---

# 27. Expired Jobs

后续可加：

```text
active
possibly_expired
expired
unknown
```

已经用于生成文档的历史职位不要删掉。

Studio 使用的 JD revision 应保持 immutable。

---

# 28. Application Tracking 集成

推荐流程：

```text
Discovered Job
↓
Create Drafts
↓
Review / Export
↓
Save to My Applications
```

只有这一步再创建 / 关联 application。

导出 CV 不自动标记 Applied。

---

# 29. 建议目录改动

```text
backend/internal/
├── discovery/
│   ├── model.go
│   ├── ports.go
│   ├── service.go
│   ├── validation.go
│   └── service_test.go
│
├── adapters/
│   ├── postgres/
│   │   ├── discovered_job_store.go
│   │   └── job_preference_store.go
│   │
│   └── jobsource/
│       ├── manual.go
│       ├── email.go
│       ├── rss.go
│       └── api.go
│
└── transport/httpapi/
    └── discovery/
        ├── handler.go
        └── dto.go
```

具体 transport 路径按你当前 repo 实际命名调整。

不要提前创建一堆空 adapter。

---

# 30. 前端建议结构

```text
frontend/src/features/discovery/
├── api.ts
├── types.ts
├── hooks.ts
├── pages/
│   └── DiscoverJobsPage.tsx
├── components/
│   ├── JobCard.tsx
│   ├── JobFilters.tsx
│   ├── JobSignals.tsx
│   └── JobDetailPanel.tsx
└── routes.tsx
```

继续复用现有 Light Glass 设计语言。

---

# 31. Migration

不要修改已发布 `00001–00006`。

新增：

```text
00007_discovered_jobs.sql
00008_job_preferences.sql
00009_job_match_evidence.sql
```

但每一批只加当前 slice 真正需要的表。

---

# 32. 推荐开发顺序

## Phase 1 — Manual Discovery MVP

先完成：

```text
/discover page
manual job import
discovered_jobs table
save / ignore
Create Drafts
Studio handoff
```

不需要 Redis。

不需要 AI。

不需要外部 Job Board integration。

验收：

> 手动导入一个职位后，可以在 Discover 页面看到，并能一键转入现有 ApplyFlow workspace / Studio。

---

## Phase 2 — Preferences and Filtering

加入：

```text
job_preferences
location filters
title keywords
included technologies
excluded keywords
saved search presets
```

---

## Phase 3 — Source Adapters

一次只加一个来源：

```text
email alerts
RSS / feed
approved API
permitted company careers source
```

来源失败不能破坏已经导入的数据。

---

## Phase 4 — Structured Requirement Extraction

用现有 provider abstraction 做：

```text
requirement extraction schema
required / preferred classification
JD evidence
skill alias normalisation
```

---

## Phase 5 — Queue Upgrade

等真实模型阶段再把：

```text
PostgreSQL outbox
↓
Redis / Asynq
↓
Worker
```

接起来。

Discovery 使用同一套 shared task runtime，不单独造队列。

---

## Phase 6 — SSE

给：

```text
source sync
job analysis
CV generation
CL generation
export
```

加实时状态。

PostgreSQL 仍是最终事实来源。

---

## Phase 7 — Load / Security Testing

测试：

```text
large discovery result sets
duplicate imports
concurrent sync requests
rate limits
database indexes
worker concurrency
source timeout behaviour
SSRF protections
```

---

# 33. Testing

## Unit tests

```text
normalisation
preference matching
duplicate detection
state transitions
validation
```

## PostgreSQL integration tests

```text
ownership
unique constraints
duplicate handling
save / ignore
workspace conversion transaction
concurrent imports
```

## Contract tests

改 public API 就更新 OpenAPI tests。

## E2E

核心流程：

```text
Import job
↓
Save job
↓
Create Drafts
↓
Open Studio
↓
Generate Resume / CL
↓
Export
```

---

# 34. Performance

建议一开始就做 pagination：

```text
GET /api/discovery/jobs?limit=20&cursor=...
```

可能需要的 index：

```text
owner_id
discovered_at
status
source_type + external_id
normalised company/title
```

不要一次把全部 discovered jobs 拉进内存。

先测 PostgreSQL，再决定是否 cache。

---

# 35. Observability

建议结构化日志：

```text
request_id
source_type
import_count
deduplicated_count
task_id
duration
status
```

不要记录：

```text
full CV
full JD
API keys
auth tokens
private file contents
```

未来可看 metrics：

```text
jobs discovered per sync
duplicates filtered
source failure rate
sync latency
queue backlog
worker task latency
```

---

# 36. 暂时不要加

不要为了“看起来高级”直接加：

```text
Kafka
microservices
Elasticsearch
Kubernetes
multiple queue systems
second database
separate discovery worker runtime
```

当前模块化单体完全够用。

---

# 37. 产品结构建议

主产品可以收敛成三个入口：

```text
1. Discover Jobs
2. Create / Studio
3. My Applications
```

Interview Prep 不作为核心模块。

新的产品描述可以是：

> **ApplyFlow helps users discover relevant jobs, create tailored resumes and cover letters, review and export application documents, and optionally track applications.**

---

# 38. 集成后的架构

第一阶段：

```text
Browser
   ↓
React
   ↓
Go API
   ↓
Discovery service
   ↓
PostgreSQL
   ↓
Selected Job
   ↓
Existing Intake / Studio
   ↓
PostgreSQL outbox
   ↓
Worker
   ↓
Mock / Real Provider
   ↓
Document revisions
   ↓
PDF / DOCX export
```

未来目标：

```text
Browser
   ↓
Nginx
   ↓
React / Go API
   ↓
Discovery + Existing Business Modules
   ↓
PostgreSQL
   ↓
Outbox Dispatcher
   ↓
Redis / Asynq
   ↓
Worker
   ↓
Provider / Parser / Export
   ↓
PostgreSQL
   ↓
Redis Pub/Sub
   ↓
SSE
   ↓
Browser
```

---

# 39. 最推荐的下一步

不要第一步就做自动 Indeed / SEEK 集成。

先完成：

```text
00007 migration
        ↓
discovered_jobs
        ↓
POST /api/discovery/import
        ↓
GET /api/discovery/jobs
        ↓
Discover Jobs frontend
        ↓
Create Drafts
        ↓
existing workspace / Studio
```

这条完整链路跑通之后，再接第一个真实 job source。

这也符合你当前项目的维护原则：

> 先完成一条可运行、可验证的完整 flow，再抽象和扩展。

---

# 40. Success Criteria

用户层面：

1. 可以导入 / 发现职位。
2. 可以查看原始来源和规范化 JD。
3. 可以 Save / Ignore。
4. 可以看到可解释的 matching signals。
5. 可以点击 **Create Drafts**。
6. 可以进入现有 ApplyFlow Studio。
7. 可以生成并编辑 Resume + Cover Letter。
8. 可以导出指定 revision。
9. 可以选择性地关联到 application tracking。

技术层面：

1. Discovery 不复制现有 task runtime。
2. PostgreSQL 继续作为 source of truth。
3. Studio 继续负责文档生成。
4. Private data 全部 owner scoped。
5. Job source adapter 可替换。
6. 没有 Redis/SSE 时功能也能工作。
7. 以后 Redis/Asynq/SSE 可以复用现有 shared task architecture。
