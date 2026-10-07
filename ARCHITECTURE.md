# ApplyFlow 工程架构

> 2026-09-28 实施状态：OpenAPI 0.9.0 定义 58 个操作，其中 42 个已有 handler。已实现认证、工作区、私有 PDF/DOCX、手动简历确认、文本 JD、独立 Mock Worker、双文档版本保存/应用、任务取消、单份重试与固定版本 PDF/DOCX 导出。当前 Worker 使用 PostgreSQL outbox 直接派发；Redis/Asynq、扫描件 OCR、截图/链接、SSE 尚未实现；英文玻璃前端已接通现有 Mock 流程。迁移为 00001–00009。个人 AI 配置后端已支持加密保存、固定短提示测试、明确启用及删除；AI 设置表单已接通，真实生成已接通，采用固定 AI revision、请求预留和用量记录。详见 [AI 设置实施说明](docs/development/ai-settings.md)。

状态：整体实施设计；第一批 OpenAPI、SQL 迁移和验证工具已落地，认证/工作区/私有文件/手动简历确认 HTTP 已实现，英文磨砂玻璃前端已接通文本 JD / Mock 文档 / 编辑导出。已落地范围见 [README](README.md)，本批协议以 [OpenAPI](api/openapi.yaml) 为准。与 [产品规划](ApplyFlow_Project_Plan.md) 配套；目录、代码边界和运行契约以本文为准。本文中的目标需通过实现和测试验收后，才可以作为生产能力声明。

本设计面向一个人或小团队能够维护的生产应用：**模块化单体、API/Worker 两个独立进程、PostgreSQL 作为业务事实来源、Redis 承担任务投递与通知**。按业务模块组织代码，使用明确的事务边界和依赖注入。

阅读顺序：

1. 本文：系统、目录、模块职责与技术决策。
2. [后端文件与代码设计](docs/architecture/backend-blueprint.md)：从 Go 文件内部结构到事务、Worker、SSE。
3. [前端、部署与验收](docs/architecture/delivery-and-frontend.md)：React 文件组织、API 协议、配置、测试、上线运行。

4. [个人资料与 AI API 配置](docs/architecture/profile-and-ai-settings.md)：个人档案、用户自带 Key、配置版本与任务集成。该文档补充并更新原统一平台 Key 方案。

5. [Application Studio](docs/architecture/application-studio.md)：核心的多输入 JD、基础简历、两份文档生成、版本审阅与导出；新工作流的事务/表/端点以此为准。
6. [正式界面设计](docs/design/DESIGN.md)：用户已确认的英文浅色混光玻璃界面。

## 1. 架构目标与边界

产品主流程为 **JD 截图/文字/公开链接 + 已确认基础简历 → AI 定制 Resume 与 Cover Letter → 审阅修改 → 导出**。申请跟踪、技能分析与面试准备是辅助/后续功能。工作区可先于申请记录存在，不要求用户先填写长职位表单。

架构保留已有基础能力，新增 intake/resume/studio/document/task 模块；文件存储和解析移到核心交付阶段。异步任务统一由 job_tasks 管理，不为每种生成或分析复制一套队列/租约系统。

| 目标 | 设计约束 |
|---|---|
| 数据不会跨账号泄露 | 每次资源访问在存储查询中带 owner 条件；间接资源通过父记录校验 |
| 接受的任务可恢复 | `202` 必须意味着任务、幂等记录和 outbox 已在同一个数据库事务提交 |
| 多次投递不会重复写结果 | 数据库原子认领、租约、fencing token、终态保护 |
| 页面重连后状态正确 | SSE 推送当前状态，数据库快照周期补偿 |
| 外部调用可控 | 输入/输出/超时/尝试次数/总并发/每日用量都有上限 |
| 可以独立扩展耗时工作 | API 和 Worker 分开部署，代码仍在同一个 Go module |
| 可追踪、可部署、可恢复 | 结构化日志、指标、迁移、健康检查、备份和故障演练 |

首版不承诺跨地域高可用、外部模型恰好调用一次、无限排队、自动评定求职者能力。单机 Compose 是部署起点；主机损坏的恢复依赖异机备份。是否升级高可用数据库由业务目标决定。

## 2. 系统结构

```text
Browser
  │ HTTPS，同源 Cookie
  ▼
Nginx ─── /、静态资源 ─── React build
  │
  └── /api/* ─── API process
                    ├── auth / profile / AI settings / private files
                    ├── intake / resumes / studio / documents / applications
                    ├── shared task submission / query / cancellation
                    ├── SSE gateway
                    └── PostgreSQL：业务数据、任务、outbox、幂等记录
                                     ▲
                                     │ short transactions
                           Worker process
                             ├── outbox dispatcher ─── Redis / Asynq
                             ├── task consumer ◀──────────────┘
                             ├── recovery scanner
                             ├── provider adapter ─── External model API
                             ├── bounded URL / OCR / resume parsers
                             └── private storage + PDF/DOCX exporters
                                     │
                            commit task state first
                                     │
                              Redis Pub/Sub
                                     │
                         API SSE gateway + DB reconciliation
```

Redis Pub/Sub 不提供断线重放；本项目通过数据库快照补偿。[Redis delivery semantics](https://redis.io/docs/latest/develop/pubsub/)

Redis 通知丢失不会丢业务状态。数据库提交后通知失败只影响即时性，由定期状态查询补偿。数据库和 Redis 不做分布式事务。

## 3. 技术选型与理由

| 层 | 选型 | 约束 |
|---|---|---|
| 前端 | React + TypeScript + Vite + React Router | 开启 TypeScript strict；按 feature 组织 |
| HTTP 数据 | Fetch + TanStack Query | server state 放 query cache；编辑中的表单保留本地状态 |
| API | Go + Gin + `net/http` | Gin 只出现在 transport 层 |
| 数据库 | PostgreSQL + pgx/v5 + sqlc | 手写 SQL、生成类型化查询；事务由业务适配器明确组织 |
| Schema 迁移 | goose | 单独发布步骤执行；API 副本启动时不争抢迁移 |
| 队列 | Asynq + 独立 Redis queue 实例 | 固定依赖版本；数据库拥有业务重试调度权 |
| 通知、限流 | go-redis；生产中与 queue 隔离或明确资源预算 | Pub/Sub 只传 ID/version，无 JD 或简历 |
| 认证 | 短时 JWT + HttpOnly Cookie + CSRF | JWT 验签、issuer/audience/expiry 校验；明确注销语义 |
| 文件 | 开发私有目录；生产私有对象存储 | 通过 Storage 接口替换；禁止公开 bucket |
| 日志/指标 | slog + Prometheus 指标 | 不用 user ID / analysis ID 作为 metrics label |
| 契约 | OpenAPI + 生成的 TS 类型 + provider JSON Schema | 三类 DTO 独立：HTTP / 业务 / SQL |
| 部署 | Docker + Compose + Nginx | 非 root、锁定镜像版本、持久卷、TLS、健康检查 |

Asynq 提供并发消费、恢复等机制，但业务幂等仍由本项目实现；其当前文档提示 v0 API 稳定性及 Redis Cluster Lua 兼容限制，因此首版采用单主 Redis 拓扑，升级版本需验证。[Asynq 官方仓库](https://github.com/hibiken/asynq)

选型是本项目的工程判断，不是唯一可行方案。依赖在开始实现时选定兼容版本并提交 lockfile/go.sum；CI 工具版本也固定。

## 4. 目标目录与文件职责

以下是目标树，不表示这些源文件已经生成。按交付阶段逐步建立目录，避免空文件占位。

```text
jobsApplyFlow/
├── ARCHITECTURE.md
├── ApplyFlow_Project_Plan.md
├── README.md                         # 启动、常用命令、环境、文档入口
├── Makefile                          # lint/test/generate/build/dev 等统一入口
├── .env.example                      # 非秘密的配置样例
├── .gitignore                        # .env、上传文件、构建物
├── .github/workflows/
│   ├── ci.yml                        # 契约、生成漂移、测试、镜像构建
│   └── release.yml                   # 制品发布、迁移、部署、健康验证
├── api/
│   └── openapi.yaml                  # 公共 HTTP 契约，唯一来源
├── contracts/
│   ├── job-extraction.v1.schema.json # JD 提取
│   ├── resume-facts.v1.schema.json   # 已确认基础简历事实
│   ├── tailored-resume.v1.schema.json
│   └── cover-letter.v1.schema.json
├── docs/
│   ├── architecture/
│   │   ├── backend-blueprint.md
│   │   ├── delivery-and-frontend.md
│   │   ├── profile-and-ai-settings.md
│   │   └── application-studio.md
│   ├── design/DESIGN.md             # 已确认 V3 + 原型引用
│   ├── adr/                          # 后续决策记录：背景、决策、代价
│   └── runbooks/                     # 后续：积压、DB 恢复、密钥轮换
├── backend/
│   ├── go.mod
│   ├── go.sum
│   ├── Dockerfile
│   ├── sqlc.yaml
│   ├── cmd/
│   │   ├── api/main.go                # 仅进程入口、信号、退出码
│   │   ├── worker/main.go             # 同上，独立生命周期
│   │   └── migrate/main.go            # 单独执行迁移
│   ├── internal/
│   │   ├── bootstrap/
│   │   │   ├── api.go                 # API 依赖组装
│   │   │   ├── worker.go              # Worker、dispatcher、scanner 组装
│   │   │   └── lifecycle.go           # 关闭顺序、错误传播
│   │   ├── platform/
│   │   │   ├── config/config.go       # 环境解析 + 启动校验
│   │   │   ├── postgres/pool.go       # pool、超时、连接生命周期
│   │   │   ├── redis/client.go        # queue 与 shared 客户端分离
│   │   │   ├── logging/logger.go      # slog/redaction 配置
│   │   │   ├── metrics/registry.go    # 低基数指标
│   │   │   └── clock/clock.go         # 仅需可测试时间时使用
│   │   ├── auth/
│   │   │   ├── model.go               # Identity、session claims
│   │   │   ├── ports.go               # Store、PasswordHasher、TokenIssuer
│   │   │   ├── service.go             # 注册、登录、改密业务
│   │   │   └── errors.go
│   │   ├── application/
│   │   │   ├── model.go               # Application、Status
│   │   │   ├── ports.go               # 用例需要的窄接口
│   │   │   ├── service.go             # CRUD、状态更新
│   │   │   ├── validation.go
│   │   │   └── service_test.go
│   │   ├── intake/                   # JD text/image/url 提取及确认
│   │   ├── resume/                   # 基础简历解析、事实与 revision
│   │   ├── studio/                   # 工作区、生成事务、关联申请
│   │   ├── document/                 # 两类文档、候选、编辑、导出
│   │   ├── task/                     # 共用 claim/lease/fence/retry/cancel
│   │   ├── analysis/                 # 后续技能分析；下列文件为业务执行器
│   │   │   ├── model.go               # Snapshot、Claim、Result、状态
│   │   │   ├── ports.go               # AcceptStore、ExecutionStore、Provider、SkillCatalog
│   │   │   ├── submit.go              # 接受任务用例
│   │   │   ├── execute.go             # 技能分析业务；生命周期委托 task
│   │   │   ├── cancel.go              # 取消用例
│   │   │   ├── result.go              # 结果校验与技能匹配
│   │   │   ├── errors.go              # 可分类业务错误
│   │   │   └── *_test.go
│   │   ├── aisettings/                # save/test/activate/revoke，详见扩展设计
│   │   ├── profile/                   # 个人资料、偏好、教育、经历与技能
│   │   ├── interview/                 # 面试轮次、问题、topic tagging
│   │   ├── study/                     # 用户 topic notes/confidence，后续阶段
│   │   ├── analytics/                 # 只读聚合用例，后续阶段
│   │   ├── attachment/                # 核心：私有上传、校验、下载授权
│   │   ├── adapters/
│   │   │   ├── postgres/
│   │   │   │   ├── dbgen/             # sqlc 输出，禁止手改
│   │   │   │   ├── tx.go              # 事务辅助，保留原始错误
│   │   │   │   ├── auth_store.go
│   │   │   │   ├── application_store.go
│   │   │   │   ├── analysis_accept.go # 接受任务的完整事务
│   │   │   │   ├── task_store.go      # 统一 claim/renew/complete/fail
│   │   │   │   ├── studio_accept.go   # 工作区生成的跨表事务
│   │   │   │   ├── document_store.go  # 不可变 revision 与 CAS
│   │   │   │   ├── analysis_read.go   # 授权 snapshot 查询
│   │   │   │   ├── outbox_store.go    # dispatcher 认领与标记
│   │   │   │   ├── recovery_store.go  # 过期任务恢复
│   │   │   │   ├── profile_store.go
│   │   │   │   └── interview_store.go
│   │   │   ├── queue/
│   │   │   │   ├── payload.go         # schema version、task ID
│   │   │   │   ├── producer.go        # Asynq 投递
│   │   │   │   └── consumer.go        # Asynq 到 execute 的适配
│   │   │   ├── pubsub/progress.go     # 状态变化通知、订阅取消
│   │   │   ├── provider/
│   │   │   │   ├── client.go          # HTTP/SDK + 响应大小/超时控制
│   │   │   │   ├── prompt.go          # prompt version 与内容
│   │   │   │   ├── decode.go          # JSON Schema + 业务证据校验
│   │   │   │   └── errors.go          # 429/timeout/permanent 分类
│   │   │   ├── security/              # jwt.go、bcrypt.go
│   │   │   ├── intake/                # URL reader、OCR、resume parser
│   │   │   ├── export/                # 受控 PDF/DOCX 模板
│   │   │   └── storage/               # 核心 local.go / object.go
│   │   ├── transport/http/
│   │   │   ├── router.go              # middleware 顺序和路由注册
│   │   │   ├── server.go              # HTTP timeout / graceful shutdown
│   │   │   ├── errors.go              # 业务错误 → HTTP problem
│   │   │   ├── response.go            # JSON 响应，不吞编码错误
│   │   │   ├── middleware/
│   │   │   │   ├── request_id.go
│   │   │   │   ├── recovery.go
│   │   │   │   ├── access_log.go
│   │   │   │   ├── auth.go
│   │   │   │   ├── csrf.go
│   │   │   │   └── rate_limit.go
│   │   │   ├── auth/handler.go
│   │   │   ├── applications/{handler,dto}.go
│   │   │   ├── tasks/{handler,dto,sse}.go
│   │   │   ├── workspaces/{handler,dto}.go
│   │   │   ├── documents/{handler,dto}.go
│   │   │   ├── resumes/{handler,dto}.go
│   │   │   ├── intakes/{handler,dto}.go
│   │   │   ├── files/{handler,dto}.go
│   │   │   ├── analyses/{handler,dto}.go # 后续技能结果
│   │   │   ├── profile/handler.go
│   │   │   └── interviews/handler.go
│   │   └── background/
│   │       ├── dispatcher.go          # outbox polling，非业务模块
│   │       └── recovery.go            # 扫描、重投、过期幂等记录清理
│   ├── migrations/                   # 递增、不可修改已发布 migration
│   ├── queries/                      # auth.sql/application.sql/analysis.sql…
│   └── tests/integration/            # 真 PostgreSQL/Redis + 故障测试
├── frontend/                         # 详细树见前端设计文档
├── deploy/
│   ├── compose.yaml                  # 生产式本地部署基线
│   ├── compose.dev.yaml              # 热更新、仅本地端口
│   └── nginx/default.conf
└── tests/
    ├── e2e/                          # 浏览器完整业务流程
    ├── load/                         # 固定数据、mock provider
    └── fixtures/                     # 合成 JD，不放真实简历
```

`{handler,dto}.go` 表示两个文件。每个目录是一个 Go package，不按 Java 风格为每个 struct 创建目录。不建立通用 `utils`、万能 `BaseRepository` 或只有一个转发方法的层。

## 5. 依赖方向与模块边界

```text
cmd → bootstrap → transport / background / adapters
                         │                    │
                         └──────→ business ←──┘
                                      │
                                  Go stdlib
```

- `analysis` 不 import Gin、pgx、sqlc、Asynq 或具体 provider SDK。
- 接口定义在使用它的业务模块中；实现放 adapters。HTTP handler 可定义自己需要的更小 consumer interface。
- `application`、`profile`、`analysis` 互不通过调用对方 HTTP handler 通信。跨模块读取通过明确的端口或共享事务适配器实现。
- `analysis_accept.go` 是有意存在的跨表事务边界，可以同时访问 application、profile 和 analysis 的 SQL；禁止把原子事务拆成三个 service 调用。
- `analytics` 使用明确的只读聚合 SQL，不循环调用应用列表接口。
- 不建立大而全的 `shared/model`。真正跨模块的身份标识可以暂时使用 string，UUID 格式在入口与数据库约束检查；需要统一类型时再提炼最小 package。
- SQL 行模型只在 postgres adapter 使用；provider DTO 只在 provider adapter 使用；handler 显式映射 HTTP DTO。
- 依赖在 bootstrap 通过构造函数注入。禁止全局 `DB`、全局 service locator、请求中临时建连接池。

## 6. 一次核心请求如何经过文件

以 `POST /api/workspaces/{id}/generations`（0.2.0 已定义协议）为例：

| 顺序 | 文件 | 做什么 |
|---|---|---|
| 1 | router + middleware | 认证、CSRF、body 上限、限流、request ID |
| 2 | workspaces/handler.go | 解析来源 revisions、expected versions、Idempotency-Key |
| 3 | studio/generate.go | 规范化输入与稳定 request hash，调用一个 AcceptStore |
| 4 | postgres/studio_accept.go | owner/幂等/版本/配额校验；保存 run、两项 task、outbox 与输入快照 |
| 5 | workspaces/dto.go | 返回 202、run/task IDs、Location，不返回 secret |
| 6 | background/dispatcher.go | 事务外发送 task ID；数据库保留恢复意图 |
| 7 | task/execute.go → document executor | claim/续租；分别生成两份文档；校验事实来源后写候选 revision |
| 8 | tasks/sse.go | 授权快照 + state_version；通知丢失以 DB 查询补偿 |

## 7. 数据与事务不变量

1. 私有资源必须带 owner 查询与复合关系约束；文件、导出、快照同样适用。
2. workspace 和 application 分开；未确认公司/岗位的输入可保存为草稿，跟踪时再创建 application。
3. 来源、基础简历、生成输入、文档 revisions 均保留不可变版本；当前 head 是有版本的指针。
4. users.profile_version、AI settings.version、workspace.version、document.version 各有职责，不能互换。
5. job_tasks 是执行状态唯一来源；两份文档独立完成/失败/重试，父显示状态从子任务推导。
6. 202 代表任务、幂等记录、配额预留与 outbox 已原子提交；任务输入固定 provider/prompt/schema/资料范围版本。
7. 终态受 lease/fence/cancel 条件保护，provider 响应必须结构和业务校验后才成为候选。
8. AI 候选不得覆盖已存在的人工编辑；应用候选和保存编辑使用 CAS。旧 run 的结果只能留为历史候选。
9. 导出固定 revision/format/template；UI 玻璃背景不是导出内容，下载仍鉴权。
10. 同一工作区重复关联只创建一个申请；applications 既有 version/jd_version/history 触发器继续生效。
11. state_version 管状态更新，fencing token 管执行租约；续租不制造无意义 SSE 事件。
12. 数据库事务不跨模型、存储、URL 抓取和解析调用。

全局锁序：`user → ai_settings → workspace → application → documents（ID 排序） → job_tasks（ID 排序） → outbox`。Worker 写文档结果遵循 workspace/document/task 顺序；只持 task 锁的续租不能反向加锁。详细接收、完成、取消和候选应用事务见 [Studio 架构](docs/architecture/application-studio.md#4-原子性并发与版本)。

旧 backend blueprint 的 analysis 表/代码片段保留作延后技能功能的可靠任务教学，不作为新 runtime 的平行实现。

## 8. 关键决策与代价

| 决策 | 获得什么 | 承担什么 |
|---|---|---|
| 模块化单体、双进程 | 独立扩容 Worker，同时保留本地事务 | 必须守住 import 和模块边界 |
| 业务状态落 PostgreSQL | Redis 数据损失后可重投 | scanner、租约和 outbox 增加代码量 |
| DB 管理业务重试 | 一个重试计数与 next_attempt_at 来源 | Asynq 重试必须明确关闭，避免双重调度 |
| 当前状态 SSE | 刷新后恢复，协议简单 | 不能回放每一条历史进度事件 |
| 已确认事实 + 候选版本 | 可溯源、保护原始简历和人工编辑 | 需要来源确认、版本存储与审阅交互 |
| 自评技能 + 确定性匹配（后续） | 不将关键词当能力 | 不能替代用户实际经验 |
| 缓存延后 | 首版一致性更简单 | 首先需要索引、分页和真实基线 |
| 短时 JWT、无 refresh 首版 | 会话模型简单 | 过期后重新登录；清 Cookie 不撤销已复制令牌 |

## 9. 完成标准

架构落实必须同时满足：核心流程可用、跨账号隔离、数据库约束生效、失败可恢复、部署可重建、备份可恢复、测试可重复。代码按目录放好只是其中一部分。

具体故障矩阵、上线目标和验收命令见 [交付与验收](docs/architecture/delivery-and-frontend.md)。后端关键 SQL 和代码模板见 [后端蓝图](docs/architecture/backend-blueprint.md)。

## 10. 本次文档校验范围

本批已落地部分的检查与使用见 [迁移维护](backend/migrations/README.md) 和 [API 契约](api/README.md)。以下说明针对其余蓝图示例：已检查本地文档链接与代码围栏；完整 Go 文件示例经过 gofmt 语法检查，model/ports/submit 三个纯标准库示例合并后通过编译检查。SQL、Gin/pgx 适配器、Nginx、浏览器流程及故障矩阵尚未在运行环境验证，属于后续实现的验收项。
