# 前端、部署与验收

> 2026-09-22 实施状态：OpenAPI 0.5.0 定义 50 个操作，其中 34 个已有 handler。已实现认证、工作区、私有 PDF/DOCX、手动简历确认、文本 JD、独立 Mock Worker、双文档版本保存/应用、任务取消、单份重试与固定版本 PDF/DOCX 导出。当前 Worker 使用 PostgreSQL outbox 直接派发；Redis/Asynq、真实 AI、自动简历解析、截图/链接、SSE 尚未实现；英文玻璃前端已接通现有 Mock 流程。迁移仍为 00001–00006。

当前前端实现、原生页面导航与轻量轮询的取舍、目录和验收命令见 [前端开发说明](../development/frontend.md)。下面 Router/Query/SSE 等完整目标结构仍作为后续设计，不代表当前全部使用。

返回 [架构入口](../../ARCHITECTURE.md)。以下源文件和 CI 命令多数属于目标设计。第一批基础 API 契约、SQL 迁移和验证命令已落地，当前实际命令见根目录 README/Makefile；认证/工作区/私有文件/手动简历确认 HTTP 已实现，其余业务 handler 尚未实现。个人资料与用户 API Key 的页面、字段和接口见 [专项设计](profile-and-ai-settings.md)。

已确认界面以 [正式设计](../design/DESIGN.md) 和 [项目 skill](../../skills/applyflow-light-glass/SKILL.md) 为准：默认英文、浅色混光、磨砂主工作面，无固定侧栏。核心流程/端点/版本语义见 [Application Studio](application-studio.md)，下述分析示例为后续技能功能，不再定义默认首页。

## 1. 前端目标目录

```text
frontend/
├── package.json
├── package-lock.json
├── tsconfig.json
├── vite.config.ts
├── index.html
└── src/
    ├── main.tsx                     # React root，无业务逻辑
    ├── app/
    │   ├── App.tsx                  # 布局入口
    │   ├── router.tsx               # 路由、lazy loading、错误页
    │   ├── providers.tsx            # QueryClient/Auth provider
    │   ├── query-client.ts          # 默认 staleTime/retry 策略
    │   ├── AppShell.tsx              # 顶部 Create / My applications / Account
    │   ├── locales/en.ts             # 默认英文文案，稳定错误 code 映射
    │   └── theme/                   # tokens.css、glass.css、layout.css
    ├── shared/
    │   ├── api/
    │   │   ├── client.ts            # fetch、CSRF、problem decode、AbortSignal
    │   │   └── generated.ts         # 从 OpenAPI 生成，禁止手改
    │   ├── ui/                     # Button/Dialog/Input 等无业务组件
    │   ├── lib/date.ts              # 时区/业务日期展示
    │   └── config.ts               # 公开配置，绝不放 provider key
    ├── features/
    │   ├── auth/
    │   │   ├── api.ts
    │   │   ├── use-session.ts
    │   │   ├── RequireSession.tsx
    │   │   └── LoginForm.tsx
    │   ├── applications/
    │   │   ├── api.ts               # 查询、创建、编辑的 HTTP 方法
    │   │   ├── query-keys.ts         # key 集中定义，包含用户和筛选条件
    │   │   ├── queries.ts           # useApplications/useApplication
    │   │   ├── mutations.ts         # create/update/archive + invalidation
    │   │   ├── form-schema.ts       # 用户体验校验；后端仍再次验证
    │   │   ├── ApplicationForm.tsx
    │   │   ├── ApplicationTable.tsx
    │   │   └── StatusSelect.tsx
    │   ├── intake/
    │   │   ├── api.ts / queries.ts / mutations.ts
    │   │   ├── source-schema.ts      # text/url/image tagged union
    │   │   ├── JobComposer.tsx       # 粘贴文字/URL、图片拖放/粘贴
    │   │   └── SourceReview.tsx      # 原文、识别疑点与确认
    │   ├── resumes/
    │   │   ├── api.ts / queries.ts / mutations.ts
    │   │   ├── ResumePicker.tsx
    │   │   ├── ResumeUpload.tsx
    │   │   └── ResumeFactsReview.tsx # 基础简历确认；不推断技能等级
    │   ├── studio/
    │   │   ├── api.ts / query-keys.ts / queries.ts / mutations.ts
    │   │   ├── use-create-drafts.ts  # 来源版本 + 幂等提交
    │   │   ├── WorkspaceHeader.tsx
    │   │   └── GenerationStatus.tsx # 两项任务聚合，不存第二份服务端状态
    │   ├── documents/
    │   │   ├── api.ts / query-keys.ts / queries.ts / mutations.ts
    │   │   ├── document-schema.ts   # 网络结构校验，不接受任意 HTML
    │   │   ├── use-document-draft.ts# 本地编辑、保存冲突、离开保护
    │   │   ├── DocumentEditor.tsx
    │   │   ├── RevisionCompare.tsx
    │   │   ├── RefinementPanel.tsx
    │   │   └── ExportMenu.tsx        # 保存成功后固定 revision 导出
    │   ├── tasks/
    │   │   ├── api.ts / state.ts    # 通用 TaskSnapshot 与版本归并
    │   │   └── use-task-stream.ts   # SSE/轮询、取消、卸载清理
    │   ├── ai-settings/             # 安全配置表单、测试/启用，不持久化 Key
    │   ├── analysis/                # 后续技能分析
    │   │   ├── api.ts
    │   │   ├── state.ts             # 版本归并、终态判断
    │   │   ├── use-submit-analysis.ts
    │   │   ├── use-analysis-stream.ts # 适配共用 tasks hook，不另建连接实现
    │   │   ├── AnalysisProgress.tsx
    │   │   └── AnalysisResult.tsx
    │   ├── profile/                 # api、queries、SkillProfileForm
    │   ├── interviews/              # api、queries、RoundEditor
    │   ├── study/                   # 后续阶段
    │   ├── analytics/               # 后续阶段
    │   └── attachments/             # 核心私有文件上传/下载 API 与状态
    └── pages/
        ├── LoginPage.tsx
        ├── CreatePage.tsx
        ├── StudioPage.tsx
        ├── ResumesPage.tsx
        ├── ApplicationsPage.tsx
        ├── ApplicationDetailPage.tsx
        ├── ProfilePage.tsx
        ├── AISettingsPage.tsx
        └── SettingsPage.tsx
```

依赖方向：pages → features → shared。feature 不能 import pages；跨 feature 编排由 page 或明确的上层 hook 完成。避免所有业务都塞进 shared。

页面组合组件，组件展示和收集输入，hook 管理交互，api.ts 管理端点调用。不要在 `ApplicationDetailPage.tsx` 内同时写 fetch、SSE 重连、表单校验和 JSX 长列表。

## 2. 状态所有权

| 数据 | 放在哪里 | 例子 |
|---|---|---|
| URL 可分享状态 | Router search params | status filter、search、cursor |
| 服务端数据 | Query cache | workspace/document head、task snapshot |
| 未提交表单 | 表单组件/hook | 文档编辑缓冲、JD 输入、notes；不能被后台 query 刷新替换 |
| 不可变版本 | 按 owner/document/revision 缓存 | 原文对比、生成候选、导出目标 |
| 临时 UI 状态 | useState | dialog 是否展开 |
| 会话身份 | `/api/auth/me` 的 query | 当前用户、过期时间 |
| 认证令牌 | HttpOnly Cookie | JS 不读取 JWT |

退出或切换账号时关闭 SSE、取消在途查询、清空敏感 query cache。持久化 cache 默认关闭，避免简历/JD 留在浏览器长期存储。后端授权是实际安全边界，RequireSession 只是用户体验。

## 3. 一个前端文件的内部代码

以下 `features/tasks/state.ts` 是通用任务状态归并文件示例；生成、OCR、解析与导出复用它，不复制每种任务的 SSE 生命周期：

```typescript
export type TaskStatus =
  | "queued"
  | "running"
  | "retry_wait"
  | "completed"
  | "failed"
  | "cancelled";

export interface TaskSnapshot {
  task_id: string;
  state_version: number;
  status: TaskStatus;
  stage: string;
  cancel_requested: boolean;
}

export function isTerminal(status: TaskStatus): boolean {
  return ["completed", "failed", "cancelled"].includes(status);
}

export function mergeSnapshot(
  current: TaskSnapshot | undefined,
  incoming: TaskSnapshot,
): TaskSnapshot {
  if (!current) return incoming;
  if (current.task_id !== incoming.task_id) {
    throw new Error("Task cache key mismatch");
  }
  return incoming.state_version > current.state_version ? incoming : current;
}
```

正式实现中基础类型由 OpenAPI 生成，保留 merge 函数手写。`JSON.parse` 后仍需 runtime schema 校验，TypeScript 类型不验证网络数据。

`use-task-stream.ts` 的职责和生命周期：

```text
输入 ownerID + taskID
  读取 GET snapshot；终态直接结束
  连接 EventSource（同源 Cookie）
  task_state → runtime validation → mergeSnapshot → Query cache
  completed → invalidate task result + 对应 workspace/document；不直接替换编辑缓冲
  failed/cancelled → close stream，显示操作入口
  onerror → close stream，启动带 jitter 的指数退避
  同时开启有界 GET polling，SSE 恢复后停止 fallback polling
  unmount / taskID 变化 / logout → close + clear timers + abort GET
```

使用一套自管重连策略：收到 onerror 后 close 原 EventSource，再自行重试，避免原生重连和手写重连同时进行。连续失败后降低频率并显示“连接中断，任务仍在后台执行”。GET 返回 401 则进入登录流程；403/404 停止重连。永远不要因断线向后端提交 cancel。

GET 与 SSE 的响应可能乱序，二者写入 cache 都必须走 mergeSnapshot，不能让迟到的 GET 把 completed 回退为 running。每个 task 一个 query key，ownerID 也包含在 key 中。

`use-create-drafts.ts`：一次用户操作生成一次 UUID key；网络重试复用 key 与原 body。用户显式点击“重新生成”才生成新 key。收到 version conflict 时刷新来源并让用户重新确认本次生成，不悄悄替换 body 后复用旧 key。可在 sessionStorage 保存不含 JD 的待确认 `{workspace ID, revisions, versions, key}`，支持刷新后的不确定提交重试，并在退出时清理。

## 4. HTTP 契约

OpenAPI 是端点和 DTO 的唯一协议来源；生成 TS 类型，但不让 generated DTO 取代 Go 业务模型。

现有 50 个基础与 Studio 操作见 `api/openapi.yaml`；核心 files/intake/resumes/workspaces/documents/tasks/export/track 端点以 [Studio HTTP 草案](application-studio.md#7-http-协议草案)为准。以下列表保留基础/资料以及后续 interview/analysis 操作的范围参考，未进入 YAML 的项尚无契约实现：

```text
POST   /api/auth/register
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/auth/me
GET    /api/auth/csrf
GET    /api/applications
POST   /api/applications
GET    /api/applications/{id}
PATCH  /api/applications/{id}
POST   /api/applications/{id}/archive
POST   /api/applications/{id}/analyse
GET    /api/analyses/{id}
GET    /api/analyses/{id}/result
GET    /api/tasks/{id}/events
POST   /api/tasks/{id}/cancel
GET    /api/me/profile
PATCH  /api/me/profile
GET    /api/ai/providers
GET    /api/me/ai-config
PUT    /api/me/ai-config
PATCH  /api/me/ai-config
POST   /api/me/ai-config/test
DELETE /api/me/ai-config
GET    /api/me/skills
PUT    /api/me/skills/{skill_id}
DELETE /api/me/skills/{skill_id}
POST   /api/applications/{id}/interviews
GET    /api/applications/{id}/interviews
POST   /api/interviews/{id}/questions
PATCH  /api/interviews/{id}/questions/{question_id}
```

这是对早期产品规划 API 的细化：普通部分更新使用 PATCH，首版删除操作使用 archive；账号硬删除/全量数据导出后置；文档 export 与私有 files 已移到核心阶段并在 Studio 文档中定义。archive 事务设置 archived_at 并请求取消关联活动任务，后续不接受新任务；Worker 依据任务取消标记处理。账号删除须等待活动任务停止/失效并清理对象文件。

统一错误响应：

```json
{
  "type": "urn:applyflow:problem:version-conflict",
  "title": "Source data changed",
  "status": 409,
  "code": "version_conflict",
  "request_id": "opaque-id"
}
```

Content-Type 使用 `application/problem+json`。validation 可附加 field errors，但不回显密码或完整原文。

| 情况 | HTTP | 客户端处理 |
|---|---|---|
| 非法 JSON、缺 key、非法枚举 | 400 | 显示输入问题 |
| 缺失/无效/过期身份 | 401 | 登录 |
| CSRF 失败 | 403 | 刷新安全上下文 |
| 不存在或非本人资源 | 404 | 统一不可访问提示 |
| 陈旧版本、key 冲突、已有活动任务 | 409 | 刷新/查看现有任务 |
| 输入过大 | 413 | 提示缩短 JD |
| 用户请求频率/额度限制 | 429 | Retry-After，明确等待时间 |
| 系统 admission 暂停、DB 不可用 | 503 | 同 key 有界重试 |
| 未分类内部错误 | 500 | request ID，服务器记录详细原因 |

列表采用 `(updated_at, id)` cursor pagination，limit 默认 20、最大 100。cursor 编码查询边界，验证格式并绑定筛选参数；不能当作授权依据。频繁编辑时列表不承诺数据库历史快照，客户端可去重并提供刷新。

## 5. 认证与数据访问

- 使用 bcrypt 哈希密码，显式处理其输入长度限制；密码不做无声截断。
- JWT 固定允许的签名算法，校验 issuer/audience/exp，强密钥来自部署 secrets。按 kid 支持有界密钥轮换，不从任意 token URL 加载密钥。
- Cookie 设置 HttpOnly、Secure、SameSite、Path；正式同源部署不开放任意 Origin 的 credentialed CORS。
- 所有写请求包括 login/logout 都有 Origin 校验和 CSRF 策略。采用签名的 double-submit token：认证后的 token 绑定会话标识，匿名登录表单也有独立的签名 CSRF 上下文。JS 可读 CSRF token，不可读认证 Cookie。
- 只有配置的代理地址可提供可信 forwarded IP；否则限流 IP 可被伪造。
- 短时 JWT 首版无 refresh。注销清 Cookie；如需要“立即退出所有设备”或密码变更立即吊销，增加 session/version 校验并关闭对应 SSE，不在纯 stateless 方案中宣称支持。
- SQL owner filtering 适用于详情、结果、SSE、cancel、文件、面试子资源；注册 email 有统一规范和唯一约束。
- render JD/notes 默认纯文本；富文本使用经过审查的 sanitizer，不直接 dangerouslySetInnerHTML。
- 仅在用户明确提交 JD 链接后由有界 URL reader 读取公开 HTTPS 网页；DNS/连接/每次重定向验证公网地址并有出口限制，失败引导文字或截图。详见 Studio 输入规则。个人资料 URL 不自动抓取；provider endpoint 仍由服务端固定。
- 生产中仅 Nginx 公开端口，PostgreSQL/Redis/内部指标不暴露公网。

## 6. 配置与启动校验

配置集中在 `platform/config/config.go`，bootstrap 读取一次，业务代码不能散落 os.Getenv。API 和 Worker 分别校验所需项。

| 配置 | 示例/含义 | 校验 |
|---|---|---|
| APP_ENV / PUBLIC_ORIGIN | local / https://… | production 必须 HTTPS origin |
| DATABASE_URL | secret | 必填，日志脱敏 |
| DB_MAX_CONNS | 按副本预算 | 所有 pool 总和小于 DB 可用连接 |
| REDIS_QUEUE_URL | 队列专用 | Worker/dispatcher 必需 |
| REDIS_SHARED_URL | 限流/通知 | 多副本 API 必需 |
| AUTH_SIGNING_KEY / CSRF_KEY | secrets | 强随机、不同用途不同 key |
| AUTH_TTL | 例如 30m | 有上限；前端已处理重新登录 |
| PLATFORM_AI_ENABLED / PROVIDER_KEY / MODEL | 可选平台模式 | 只在平台模式开启时必需；用户模式读取加密个人凭据 |
| CREDENTIAL_KEY_VERSION / CREDENTIAL_MASTER_KEY | secret/KMS 引用 | 保存/测试与 Worker 所需，独立于数据库保存，禁止日志输出 |
| PROVIDER_TIMEOUT | 例如 60s | 小于 TASK_TIMEOUT |
| TASK_TIMEOUT / TASK_MAX_AGE | 90s / 30m 起点 | 适配模型实际延迟 |
| WORKER_CONCURRENCY | 例如 4/副本 | 与副本数和限流共同验证 |
| LEASE_DURATION / RENEW_INTERVAL | 45s / 10s | renew 明显小于 lease |
| MAX_ATTEMPTS | 例如 3 | 正数、小上限 |
| SSE_HEARTBEAT / RECONCILE | 15s / 10s | 小于 proxy idle timeout |
| SHUTDOWN_GRACE | 例如 30s | 编排器 stop grace 更长 |
| MAX_JD_BYTES | 例如 64 KiB | HTTP/provider/storage 一致 |
| MAX_UPLOAD_BYTES / MAX_IMAGE_PIXELS / MAX_RESUME_PAGES | 10 MiB / 25MP / 20 页起点 | 代理、HTTP、解码/解压/解析限制一致 |
| URL_FETCH_TIMEOUT / MAX_URL_BYTES / MAX_REDIRECTS | 10s / 2 MiB / 3 | 解压后大小限制，独立公网出口策略 |
| EXPORT_TIMEOUT / TEMPLATE_VERSION | 按 PDF/DOCX 压测设定 / 固定版本 | 禁用远程资源，产物校验后发布 |

这些数字是可测试的起始配置，不是性能保证。总 DB 连接预算包括 API、Worker、scanner、迁移、运维预留。SSE 不长期占用 DB connection；每次快照查询及时归还连接。

依赖故障策略：DB 不可用则业务请求 503、readiness=false；shared Redis 限流失败时登录/新分析 fail closed，普通授权读取仍可服务；通知失败降级轮询。queue Redis 失败时允许数据库接受少量有界积压，但 admission 达到阈值就拒绝新分析；已经 202 的任务继续保留恢复。

## 7. 部署与 Nginx

生产式 Compose：Nginx、API、Worker、PostgreSQL、queue Redis、shared Redis。开发环境可合并 Redis，但必须采用保护队列的内存策略并禁用业务缓存。生产 queue Redis 使用 noeviction 和持久化，内存耗尽时让 enqueue 显式失败，由 outbox 重试。

网络原则：public 只接 Nginx；API/Worker 在 private 网络访问 PG/Redis。Worker 因调用 provider 需要受控公网出口。root filesystem 可读只读，临时目录和持久卷显式开放；进程使用非 root 用户。

Nginx 核心 location 片段如下；完整配置还需要 TLS、上游解析策略、日志脱敏与安全 header。因为没有 `^~ /api/`，SSE regex 才能优先匹配。

```nginx
location ~ ^/api/tasks/[^/]+/events$ {
    proxy_pass http://api:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    gzip off;
    proxy_read_timeout 60s;
}

location = /api/files {
    proxy_pass http://api:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $remote_addr;
    client_max_body_size 11m; # 单文件 10 MiB + multipart 开销；API 独立检查
    proxy_read_timeout 60s;
}

location /api/ {
    proxy_pass http://api:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_read_timeout 15s;
    client_max_body_size 128k;
}

location / {
    root /usr/share/nginx/html;
    try_files $uri $uri/ /index.html;
}
```

当存在更前置的负载均衡器时，先配置可信代理链再确定 forwarded IP 策略。附件上传用独立 location 和上限，不直接扩大所有 API 的限制。SSE 心跳间隔必须短于所有链路 idle timeout。[Nginx proxy 官方说明](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)

缓存策略：带 hash 的静态资产长期 immutable；index.html 不长期缓存；API 私有数据不被共享代理缓存。生产构建由多阶段镜像将 frontend dist 放入 Nginx；不运行 Vite dev server。

## 8. 健康检查与可观测性

| 信号 | 判断标准 |
|---|---|
| `/health/live` | 进程 HTTP event loop 可响应，不依赖 DB/Redis |
| `/health/ready` | 未 drain、schema 兼容、DB 可达；能力降级单独报告 |
| Worker health | consumer/dispatcher/scanner 最近心跳，DB/queue 连通性 |
| API metrics | request rate、error ratio、duration，按路由模板聚合 |
| 任务 metrics | oldest queued age、outbox lag、active、retries、lease recovery、failure reason |
| Provider metrics | duration、429、timeout、usage、预算预留与结算差额 |
| SSE metrics | active connections、disconnects、reconcile failures、write timeout |

日志包含 request_id、task_id、attempt、fence、trace_id（如启用 tracing）；owner ID 仅在确有排障需要时记录。不能把这些高基数字段做 metrics label。DB DSN、Cookie、Authorization、密码、JD/CV、完整 prompt/provider body 均不进入默认日志。

建议最初的验收目标：在记录了硬件与数据量的环境下，普通 CRUD p95 < 300ms、接收分析 p95 < 500ms；正常通知状态延迟 < 2s，丢通知后在一个 reconciliation interval 加查询时间内恢复。模型耗时单独统计。未完成测量前不对外宣称达到这些目标。

当 outbox 最老记录、queued 最老任务或 provider failure ratio 超阈值时报警。单纯 CPU 正常不代表任务正常执行。

## 9. 测试金字塔与故障矩阵

测试重点放在业务不变量和故障恢复；不为简单 getter 或每层转发方法造 mock 测试。

| 层级 | 验证内容 | 依赖 |
|---|---|---|
| 单元测试 | request hash 稳定、技能匹配、错误分类、状态归并 | 纯函数、fake clock/provider |
| PostgreSQL integration | ownership、并发唯一约束、幂等、事务、fencing | 真实 PostgreSQL |
| Redis integration | 投递、丢通知、限流、队列重连 | 真实 Redis + PG |
| HTTP contract | DTO、错误码、CSRF、deadline、SSE 清理 | httptest + 数据库 |
| Browser E2E | 登录→三类 JD/基础简历→确认→两份草稿→编辑/导出→保存申请 | 完整栈 + mock provider |
| Load | CRUD、提交吞吐、任务吞吐、SSE 连接 | 固定合成数据 + mock provider |

Studio 核心还必须验证 [专项故障矩阵](application-studio.md#9-验收与开发顺序)：跨账号文件/版本/导出、OCR/链接失败、部分成功、候选不覆盖编辑、导出固定版本。下列 analysis 专用条目属于可复用故障模式及后续技能分析验收；移植至 job_tasks 后测试，不新建第二套运行时。

必须覆盖：

1. 两个账号分别访问对方的 application/result/events/cancel/interview/file，全部被拒绝。
2. 同 key 相同 body 并发提交，只生成一个 analysis/outbox；不同 body 返回 409。
3. 不同 key 同 application 并发提交，只有一个 active analysis。
4. DB commit 后、enqueue 前进程退出；重启 dispatcher 后仍执行。
5. enqueue 后、outbox 标记前退出；重复 delivery 只允许一个有效 claim。
6. provider 调用中 Worker 被强杀；lease 过期后恢复，attempt 有界。
7. 旧 worker 在 lease 失效后返回成功，不能提交覆盖结果。
8. 取消与完成分别先提交；验证两个合法结果，禁止取消后又 completed。
9. 更新 JD 后旧任务完成；旧结果只在历史里出现。
10. Pub/Sub 最终通知被丢弃，SSE 定期 snapshot 仍显示 completed。
11. Redis queue 数据丢失，DB queued recovery 重投；不在生产中执行破坏性演练。
12. provider 429、timeout、非法 JSON、过大响应；重试和费用预算均受限。
13. 结果写入成功但 skill 写入失败；整个事务回滚。
14. SSE 慢客户端、auth 过期、服务 drain 后无 goroutine/subscription 泄漏。
15. GET 与 SSE 乱序、页面卸载、账号退出；前端不回退状态或残留连接。
16. 迁移从上一发布版本升级成功；从备份恢复后可读业务数据并恢复未完成任务。

并发测试使用 barrier、故障注入点与可控 provider，不靠大量 sleep 碰概率。Go race detector 用于共享内存并发问题，不能替代数据库竞争测试。

## 10. CI、发布和回滚

目标 Makefile 命令：

```text
make generate        # sqlc + OpenAPI TS；随后检查 generated diff
make lint            # gofmt、go vet、固定版本 linter、TS strict/lint
make test-unit       # Go unit + frontend unit
make test-integration# 临时 PG/Redis，migrations，integration tests
make test-race       # Go race detector
make test-e2e        # 完整栈 + mock provider
make build           # Go binaries、frontend assets、images
```

以上是待实现的命令契约，现在不能直接运行。CI 还验证 OpenAPI/schema、依赖漏洞与 secrets，上传失败测试日志但先脱敏。不依赖开发者机器的数据库或真实模型 key。

发布顺序：构建固定 digest 制品 → 备份/迁移预检查 → 执行向后兼容 migration → 更新 API/Worker → 验证 readiness + 合成业务 smoke → 观察队列/错误率 → 完成发布。

采用 expand/contract：先加可兼容字段和索引，再部署使用新字段的代码，最后在后续发布移除旧字段。大表并发建索引遵循 PostgreSQL 的事务限制，单独设计 migration。回滚优先回滚应用镜像；已经有新数据的 schema 不自动执行破坏性 down migration。

队列 payload、prompt、result schema 都有版本；滚动发布时新 worker 要能处理仍存活的旧版本任务，或者先 drain。旧 API 也必须能读取新 worker 写入的结果，必要时先部署兼容读取代码。

## 11. 运维与恢复

上线前至少准备三个 runbook：

- **任务积压**：查看 DB 队列年龄/outbox lag → 检查 dispatcher/Redis/provider quotas → 必要时暂停 admission → 修复后有界恢复。禁止直接把所有 running 改回 queued。
- **数据库恢复**：隔离应用写入 → 恢复备份 → 校验 schema 和关键记录 → 启动 scanner → 逐步开放请求。已发生的外部模型费用不能随数据库回滚撤销。
- **密钥轮换**：添加新 key → 切换签发 → 保留旧验证 key 至有效期结束 → 删除旧 key；紧急泄漏时强制吊销会话。

备份必须存放到不同故障域，定期演练恢复。先约定可接受的数据损失窗口 RPO 与恢复时间 RTO；个人部署起始目标可设 RPO 24h、RTO 4h，只有演练通过才认为成立。文件与 DB 元数据备份应可对应，孤儿对象允许清理但不能误删已被引用的版本。

## 12. 推荐实施顺序

1. 已完成 Studio 契约 0.5.0 与迁移 00005/00006；后续真实 AI 凭据/用量需扩展契约和新增迁移，既有文件不回写。
2. 已完成最小 API 配置/健康/日志、auth/CSRF、private files（PDF/DOCX）、workspace、手动基础简历事实与 owner 隔离；自动简历提取待实现。
3. 已完成文本 JD + mock provider、task/outbox Worker、两份文档、revision 编辑/候选应用和任务取消/恢复/单份重试；已完成固定版本 PDF/DOCX 导出，英文玻璃前端已接通，下一步接入真实 AI。
4. 租约/取消/恢复/幂等、个人 Key、真实模型、截图 OCR、安全 URL reader、PDF/DOCX 输入输出。
5. 正式英文玻璃 UI 与 SSE/轮询联调，验证部分失败、刷新、手工编辑冲突、完整导出与保存申请。
6. 主流程验收后再增加面试记录/技能分析/统计，按实测引入缓存与性能优化。

完整个人资料不是首次生成的必填门槛；已经确认的基础简历是事实来源。默认首页不是 Dashboard。各切片先形成可运行行为再提炼共享代码。
