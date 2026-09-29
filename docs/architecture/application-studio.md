# Application Studio — 核心工作流架构

> 2026-09-28 实施状态：OpenAPI 0.7.0 定义 57 个操作，其中 41 个已有 handler。已实现认证、工作区、私有 PDF/DOCX、手动简历确认、文本 JD、独立 Mock Worker、双文档版本保存/应用、任务取消、单份重试与固定版本 PDF/DOCX 导出。当前 Worker 使用 PostgreSQL outbox 直接派发；Redis/Asynq、自动简历解析、截图/链接、SSE 尚未实现；英文玻璃前端已接通现有 Mock 流程。迁移为 00001–00008；个人 AI 设置后端与表单已接通，真实生成已接通，运行边界见 docs/development/live-generation.md。

状态：2026-09-21 确认的目标设计；契约、表结构及文本 JD → 双文档 Mock 工作流已落地。本文与 [界面设计](../design/DESIGN.md) 配套，补齐输入、生成、版本、导出及文件职责。现有 `api/openapi.yaml` 已为 0.7.0，包含基础和 Studio 协议；下述为整体目标，具体字段与 mock 范围以 YAML 为准。当前可调用范围见本地 API 文档；固定版本导出已实现，真实 AI 生成已实现，AI 修改尚未实现。

## 1. 边界与流程

```text
Create workspace → import JD (text / image / public URL)
                 → select/import base resume (PDF / DOCX)
                 → review extracted facts and job details
                 → confirm immutable input snapshot
                 → generate resume + cover letter as independent tasks
                 → review/edit/apply candidate revisions
                 → export selected revision → optionally track application
```

`workspace` 是申请材料工作区，允许尚无公司/岗位确认的输入；`application` 是跟踪记录。生成前不要求创建 application，因此不放宽既有职位表的必填约束。工作区草稿独立持久化，关联跟踪时才建立 application，默认 Saved，不能据导出推断 Applied。

保留模块化单体、Go API/Worker、PostgreSQL、Redis/Asynq、SSE、React/Vite。不用微服务、单文件前端或 Next.js 重建本项目。面试技能分析后置，已有技能目录和自评不删除。

## 2. 模块与文件职责

以下为目标文件，不提前创建空实现。每个模块拥有用例和窄接口；外部 IO 均在 adapter 内。

| 路径（backend/internal 下） | 文件与责任 |
|---|---|
| `intake/` | `model.go` 定义 Source/ExtractedJD；`submit.go` 接受来源；`extract.go` 调用读取/OCR/提取端口；`confirm.go` 验证用户确认，建立不可变 JD revision |
| `resume/` | `model.go` 定义事实块及来源；`import.go` 发起解析；`confirm.go` 保存确认 revision；`service.go` 默认简历选择与列表 |
| `studio/` | `model.go` 定义 workspace 和输入快照；`create.go` 创建草稿；`generate.go` 原子创建生成批次/任务；`track.go` 幂等关联申请 |
| `document/` | `model.go` 定义结构化 Resume/Letter；`validate.go` 校验结构/来源；`revise.go` 人工编辑与 AI 候选；`apply.go` 乐观锁应用候选；`export.go` 固定版本请求导出 |
| `task/` | `model.go` 状态/类型/claim；`ports.go` AcceptStore/ExecutionStore；`execute.go` 生命周期；`cancel.go` 取消；业务执行器由 bootstrap 按类型注册，不以字符串反射调用函数 |
| `attachment/` | `upload.go` 创建私有上传意图；`finalize.go` 检测大小/类型/校验和；`download.go` owner 授权；`cleanup.go` 清理孤儿/临时对象 |
| `adapters/postgres/` | `studio_accept.go` 跨表生成事务；`task_store.go` claim/renew/fence；`document_store.go` 版本/指针 CAS；`intake_store.go`、`resume_store.go` 来源确认；`export_store.go` 导出结果提交 |
| `adapters/intake/` | `url_reader.go` 有界公共网页读取；`image_ocr.go` 图像识别端口实现；`resume_parser.go` 无网络隔离解析器；外部读取不得由语言模型工具自主触发 |
| `adapters/provider/` | `extract_job.go`、`tailor_resume.go`、`write_letter.go`、`revise_document.go` 分离输入/输出 schema 与版本；共享有界 client，不共用可变用户凭据 |
| `adapters/export/` | `pdf.go`、`docx.go` 从受控文档结构渲染；`templates/` 版本化模板，不接受任意 HTML/脚本/远程资源 |
| `transport/http/` | `intakes/`、`resumes/`、`workspaces/`、`documents/`、`tasks/`、`files/` 各自 handler/dto；tasks 有 sse.go；handler 不调模型、不运行解析器 |

`generate.go` 只规范化请求、生成稳定 hash、调用一个跨表事务端口；不依次调用多个 CRUD service 拼出“原子性”。`execute.go` 只管 claim/续租/取消/重试和完成，文档逻辑在对应执行器。事实校验为纯函数，provider DTO 不流入数据库接口。

目标 `contracts/`：`job-extraction.v1.schema.json`、`resume-facts.v1.schema.json`、`tailored-resume.v1.schema.json`、`cover-letter.v1.schema.json`；每种输出有 schema/prompt 版本。模型升级时用固定合成 fixtures 回归，不在历史任务中替换 prompt。

## 3. 数据设计

核心表已在 00005/00006 落地；下表为完整目标，AI 配置已在 00007 落地，调用预留与 token 记录在 00008 落地。所有私有关系用 owner 过滤和复合 owner FK，UUID 不能代替授权。对象内容放私有存储，结构化文本/版本放 PG。

| 表 | 关键内容/约束 |
|---|---|
| `files` | owner、purpose（jd_image/base_resume/export）、状态 uploading/ready/rejected、私有 key、检测 MIME/size/hash；ready 才可引用；对象不可原地替换 |
| `job_sources` | owner、workspace、kind（text/image/url）、原始文本/规范化 URL/图片关联、source_version、latest extraction task；来源替换提升版本，不改写旧快照 |
| `job_source_files` | owner、source_id、file_id、position；复合 owner FK、唯一 source/position 与 source/file，保持截图顺序 |
| `job_revisions` | owner、workspace、source_version、确认公司/岗位/JD、字段来源与 OCR 疑点、confirmed_at；确认后不可变 |
| `resumes` | owner、名称、current_revision_id、version；默认选择存用户设置，不改变历史任务 |
| `resume_revisions` | owner、resume、source_file_id 可空、结构化 facts、稳定 fact ID、证据位置、confirmed_at；用户编辑产生新 revision |
| `workspaces` | owner、version、job_revision_id、resume_revision_id、current_run_id 可空、application_id 可空、archived_at；独立于 application |
| `generation_runs` | owner、workspace、输入快照、各输入 revision、profile_version、AI revision、prompt/schema 版本、locale=en、两份 document ID、创建时间；输入不可变 |
| `documents` | owner、workspace、kind（resume/cover_letter）、current_revision_id、version；唯一 workspace/kind |
| `document_revisions` | owner、document、parent_revision_id、run_id、task_id 可空、origin（ai/manual）、结构化内容、source_fact_ids、change_summary、schema_version；不可变、唯一 task/result 防重复写入 |
| `job_tasks` | owner、kind、target_id、run_id 可空、status/stage/state_version、attempts、lease/fence、cancel_requested、next_attempt_at、AI revision 可空、safe_error、result reference |
| `task_outbox` | task_id、delivery generation、dispatch claim/期限/backoff；唯一 task/generation |
| `idempotency_requests` | owner、operation、key_hash、request_hash、稳定 resource IDs/响应、expires_at；唯一 owner/operation/key |
| `document_exports` | owner、document_revision_id、format、template_version、task_id、file_id；成功后固定为该版本，失败不产生下载链接 |
| `application_documents` | owner、application、document_revision、purpose；保存实际申请使用版本，不跟随工作区 head 自动变化 |

`job_tasks` 是未来异步执行状态的唯一来源，包括 extract_jd、parse_resume、tailor_resume、write_cover_letter、revise_document、export_document；未来技能分析也复用此运行时。旧蓝图中的 analyses 状态字段为早期教学示例，不另实现一套任务状态/队列；技能结果可保留专有结果表和 task 引用。

两份文档子任务状态聚合成工作区展示：有非终态显示 preparing；全部 completed 显示 ready；部分成功且其余终态失败/取消显示 partially_ready；无成功且全终态显示 failed/cancelled。这是读取时聚合，不在父记录维护第二套可漂移状态机。

## 4. 原子性、并发与版本

接受生成请求：

1. 同一短事务，先按 owner/operation/key 处理幂等重放。相同 key/body 返回原 run；不同 body 返回 409，不因后续来源变更而破坏旧重放。
2. 锁 `user → ai_settings → workspace → documents（ID 排序）`，验证 owner、expected_workspace_version、已确认且 ready 的来源、expected_profile_version、expected_ai_config_version 和各 revision。
3. 验证当前服务商能力、凭据可用和用量限额；保存选中的资料事实快照和白名单传输字段。简历与资料冲突先由用户确认，不让模型决定真伪。
4. 预留两项生成用量，插入 run、两项 job_tasks、各自 outbox、幂等记录，原子更新 workspace.current_run_id/version；全部成功才返回 202。
5. dispatcher 在事务外发送 `{v:1, task_id}`。生成/解析/上传不持有数据库锁等待网络。

锁的全局顺序：`user → ai_settings → workspace → application → documents（ID 排序） → job_tasks（ID 排序） → outbox`，省略不需要的节点。基础简历与来源 revision 不可变，受 user/workspace 锁控制其可用状态。对象不能在活跃任务仍引用时被物理删除。

同一 document 最多一个 active 生成/修改 task（部分唯一索引），同一 source revision 最多一个 active 提取 task；导出按 revision/format/template 去重。业务重试复用已接受快照；用户更换来源后重新生成是新 run。部分失败的 Retry 针对原 run 的失败项创建新 task 和 outbox，不重新生成成功项。

新结果始终写为不可变候选 revision。首次生成时，仅在 document 无 head、仍匹配 current_run_id 与 expected document version 时允许初始化 head；否则保留候选让用户 Apply。人工编辑与 Apply 均用 expected_version CAS 创建/选择 revision、提升 document.version，不修改原版本。旧任务晚到不能覆盖人工修改或新版 run。

Worker 完成事务的锁顺序仍为 workspace → document → task，条件检查 fence、lease、取消和来源 run 后再写 revision/result；仅持有 task 锁的续租流程不能反向锁 workspace/document。取消/完成的线性化点为 task 条件更新，失败则整个结果事务回滚。导出文件先写唯一临时私有 key，再通过同样 fence 提交 ready 引用；迟到上传只成为可清理孤儿。

所有 visible task 状态提升 state_version。业务重试由 PG 管理，Asynq MaxRetry(0)；租约失效由 scanner 恢复，旧 fence 不能提交。调用可能重复计费，不能宣称外部模型 exactly once。额度为每个外部 attempt 预留/结算，重试同样受限。

`Save to my applications` 锁 owner/workspace，检查必填公司/岗位和选定文档版本，创建或返回已有 application，同时写 application_documents。唯一关联与事务保证重复点击不重复创建。归档工作区取消其未完成任务；归档已关联申请时通过同样锁序取消关联工作区任务，历史版本仍可读取。

## 5. 输入与事实来源

- JD 文字按纯文本处理，首版建议 64 KiB；截图 PNG/JPEG/WebP 最多 5 张、每张 10 MiB、解码后每张最多 25MP；基础简历 PDF/DOCX 最多 10 MiB/20 页。这是待契约化的起始限制，前端/代理/API/parser 保持一致。
- 上传经过检测 MIME、解码/解压上限、拒绝宏/加密/不可读内容；解析器隔离运行，无网络，有 CPU/内存/时间限制。PDF 扫描件无法可靠提取时提示上传可选中文字文件或手工确认，不静默生成空简历。
- 链接首版仅公开 HTTPS 网页；用户提交才抓取，不预抓取个人资料链接。不登录第三方网站、不绕过反爬，不承诺任意招聘站均可读取。失败返回粘贴文本/截图入口。
- URL reader 禁止 URL 凭据、非标准端口及非公网地址；每次 DNS 解析、重定向和连接均检查 IPv4/IPv6 私网、回环、链路本地、metadata 等地址，连接绑定已验证地址并校验 TLS host，禁用任意环境代理。最多 3 次重定向、10 秒总时限、2 MiB 解压后 HTML；出口网络再阻断内网。不加载脚本、iframe、图片或网页指定的外部资源。
- 提取结果保留原文与位置。简历事实块含 fact_id、类型、来源 revision、页码/文本范围及用户确认状态；JD 只能提供岗位要求，不能成为用户经历的事实来源。
- provider 输出必须引用有效事实 ID；数字、公司、日期等新增实体需校验，无法支撑的声明标记 needs_input，不进入默认可导出文档。引用有效不等于语义一定正确，因此仍提供来源对照与人工审阅。
- JD、简历、网页文本均为不可信内容，不能改变系统指令或启用工具。模型只返回限定 schema，不读取任意 URL、不执行代码、不投递申请。

## 6. Provider 与资料范围

复用个人 Key 的保存/测试/启用/撤销机制；目录声明模型能力（文本、视觉、结构化输出），截图流程必须有可用 OCR/视觉路径，缺能力明确提示，不静默切到平台 Key。解析可用本地组件时不发送外部 AI。

| 操作 | 允许发送的内容 |
|---|---|
| 连接测试 | 固定短提示，无用户内容 |
| JD 提取/OCR | 本次 JD 文本或所选截图 |
| 简历结构提取（如需外部模型） | 当前简历必要内容，调用前明确提示 |
| Resume / Cover Letter | 已确认 JD、所选简历事实、用户选定的相关资料、改写偏好 |
| 文档修改 | 固定版本内容、来源事实及用户本次指令 |

联系邮箱、电话、住址等默认本地合并到导出模板，不作为模型必要输入。姓名也可本地合并信件落款；薪资偏好等与当前写作无关的资料不发送。UI 明确说明本次服务商与传输类别，修改数据范围需同步 disclosure/schema/prompt 版本。API Key 永不放入 prompt、导出、queue payload 或日志。

## 7. HTTP 协议草案

所有路径同源 cookie/CSRF、owner 授权、no-store、统一 problem；普通字段错误 400、超限 413、类型不支持 415、陈旧版本/幂等冲突 409。Studio 操作已加入 OpenAPI 0.5.0，实际请求/响应以 YAML 为准；下面仍列整体目标语义，不代表 handler 已实现。

| 方法/路径 | 语义 |
|---|---|
| POST `/api/files` | multipart 上传，201 ready 或安全错误；文件 IO 有独立上限，未就绪文件不能用于生成 |
| GET `/api/files/{id}/download` | 授权后下载 private 文件，安全 filename、attachment disposition、nosniff |
| GET `/api/workspaces` | cursor 分页恢复未归档工作区，默认按最近更新；不是统计首页 |
| POST `/api/workspaces` | Idempotency-Key 创建可恢复草稿，201 |
| GET/PATCH `/api/workspaces/{id}` | 获取/修改选择与偏好，写入带 expected_version |
| POST `/api/workspaces/{id}/sources` | tagged union：text/URL/ordered file IDs；expected workspace version，Idempotency-Key；202 + source/task IDs |
| POST `/api/workspaces/{id}/job-revisions` | 确认提取结果，expected workspace/source version，201 immutable revision |
| GET/POST `/api/resumes` | 分页列表/导入 ready 文件；导入带 Idempotency-Key；当前 manual 模式 201 + resume，task_id=null；自动解析待 Worker 实现 |
| GET `/api/resumes/{id}` | 当前解析状态、已确认 revision 与来源 |
| PATCH `/api/resumes/{id}` | 重命名/设为默认，expected resume version；默认选择在同用户事务内更新 |
| POST `/api/resumes/{id}/revisions` | 确认/编辑基础事实，expected resume version，201 |
| POST `/api/workspaces/{id}/generations` | versions + selected revisions + locale；Idempotency-Key，202 run/两项 task IDs |
| GET `/api/workspaces/{id}/generations/{runId}` | 输入版本、两项任务与候选文档状态 |
| POST `/api/workspaces/{id}/generations/{runId}/retry` | 仅指定失败/取消的 document kind；expected workspace version、Idempotency-Key；202 |
| GET `/api/documents/{id}` | head、version、候选列表（分页） |
| GET `/api/documents/{id}/revisions/{revisionId}` | 指定不可变内容与来源 |
| POST `/api/documents/{id}/revisions` | 人工保存结构化内容、expected_version；201，不接受任意 HTML |
| POST `/api/documents/{id}/refinements` | 基于固定 revision 的改写指令/选项、expected_version、Idempotency-Key；202 candidate task |
| POST `/api/documents/{id}/apply` | candidate revision + expected_version，200 head |
| POST `/api/documents/{id}/exports` | revision + PDF/DOCX + template version、Idempotency-Key；202 task；已生成同制品可返回 200 |
| GET `/api/tasks/{id}`、GET `/api/tasks/{id}/events` | 授权快照及 SSE `task_state`，id/state_version 归并，无 token 流重放保证 |
| POST `/api/tasks/{id}/cancel` | 取消意图，返回当前状态；完成先提交则保留 completed |
| POST `/api/workspaces/{id}/track` | expected_version + 文档 revisions + Idempotency-Key；201 新申请，200 已关联申请 |
| POST `/api/workspaces/{id}/archive` | expected_version；取消活动任务且保留历史，200 |

返回未知结果的非幂等写操作不得自动重试。幂等记录起始保留至少 24 小时且不早于关联任务终态；响应告知有效期，过期后先查询工作区/文档状态，再由用户确认新操作。hash 包含规范化版本/选择/选项，不包含 API Key。

## 8. 导出与存储

文档保存语义化结构，导出由受控模板生成可选中文字的 PDF 和可编辑 DOCX；不承诺还原上传简历的原始排版。允许复制纯文本作为辅助。模板版本加入制品 key，导出固定 revision，与之后的编辑互不影响。

导出器不访问远程字体/图片/URL，不解释用户 HTML。先完成上传和校验再提交 files.ready 与 export.result，下载检查 owner 和任务结果。返回导出文件前做打开/分页/文字提取/边界检查，长段落和多页简历有固定 fixtures。未完成对象不能暴露公共链接。

原始文件、确认快照和导出都属于私有数据。清理任务仅删除确认无引用且超过宽限期的对象，活跃任务 pin 住其来源。账号删除/保留期需要覆盖这些新表、对象与活跃任务，后续功能上线前另行落实，不能只清 application。

## 9. 验收与开发顺序

1. 扩展契约与迁移：workspace、来源、简历事实、文档版本、统一任务/outbox；保留现有迁移不回写。为 tagged union、版本、owner FK、幂等与候选应用写有意义测试。
2. 实现认证/CSRF、private files、工作区保存、基础简历确认；先用文本 JD + mock provider 跑通两份草稿、编辑与单份导出。
3. 实现个人 Key、真实模型、OCR 与 URL reader，补齐 PDF/DOCX 输入输出。独立完成两项生成、部分失败恢复、取消和重试；全部支持后才算首个完整核心版本。
4. 按正式玻璃设计接入前端，SSE + polling 刷新恢复、两个账号隔离、两份文档版本冲突和完整导出 E2E；最后接入保存申请。
5. 面试准备、统计、缓存及性能实验在主流程验收后实施。

必测：跨账号 file/revision/task/events/export 访问；SSRF 重定向与 DNS 地址切换；损坏/超大/解压膨胀文件；OCR 错字确认；同 key 并发只一个 run；半成功不重复已完成模型调用；人工编辑遇迟到结果不被覆盖；旧 run 不成为当前内容；取消/撤销 Key 与完成竞态；队列丢失与 worker 崩溃恢复；PDF/DOCX 可打开且内容与固定 revision 一致。业务测试以 mock provider 为主，另用人工审阅的合成素材评估真实模型质量。

## 当前落地切片：手动基础简历（0.3.0）

`internal/resume/service.go` 定义事实、版本和输入规则，`adapters/postgres/resume_store.go` 负责同用户锁、幂等导入、默认切换与 revision/head 原子更新；`internal/files` 使用存储和校验端口，`adapters/localfiles` 提供私有本地文件及 PDF/DOCX 校验。HTTP DTO 在 `transport/httpapi/resumes.go` 和 `files.go`，无 Worker 假实现。

正常新用户已可上传原件、手动填写/确认事实，再把确认 revision 选入工作区。截至 0.5.0 已补充文本 JD 与 Mock 双文档工作流；自动简历提取/OCR、真实 AI 与 UI 未实现。自动导入的目标仍是 task/outbox 事务；当前显式 manual 模式绕开解析任务，绝不生成空转 queued 记录。OpenAPI 0.5.0 为当前可执行契约。

原件先以随机 key 独占创建、fsync 文件和目录，再写 ready 元数据。DB 提交结果不确定时保留文件，以防误删已提交引用。异常退出可能留下无引用文件；自动清理尚未实现，后续需宽限期与 DB 引用复查，不能仅按文件年龄删除。备份和恢复必须同时包含私有目录及数据库。当前部署为单主机单 API 进程，未来对象存储替换 Blobs 端口。

## 当前落地切片：文本 JD 与 Mock 双文档（0.5.0）

独立 `cmd/worker` 已运行任务状态机；当前派发 adapter 使用 PostgreSQL outbox 直接领取，不引入 Redis 运维依赖。Claim 与消费 outbox 原子提交，SKIP LOCKED 防止并发领取；30 秒租约和 10 秒执行预算适用于无外部 IO 的 Mock。恢复器使过期运行任务进入 retry_wait，再发新 delivery；超过 3 次领取失败终止。Redis/Asynq、续租、provider 配额仍是后续真实 AI 阶段，不宣称本批已交付。

`generation.Executor` 通过窄 Store 和 Provider 接口读取固定输入、生成并校验内容；SQL adapter 只做持久化和并发控制，Mock Provider 只组装已确认事实。任务读取不暴露 lease/fence、input_snapshot 或私有存储路径。当前取消直接写 cancelled，使晚到结果在提交条件处失败。

文本来源也产生真实 task/outbox，返回原文等待用户确认，公司/岗位不猜测。两项文档任务各自保存结果。首次 head 只在空 head、版本与 current_run 匹配时初始化；人工编辑可在新 run 进行中继续保存原 head，保留旧 run 的来源；新结果只能成为候选，Apply 校验当前 run。单份失败/取消可显式重试，成功兄弟任务不重做。生成、结果、重放、取消、恢复与编辑竞态均有真实 PG 集成测试。

schema 仍为 6，既有 migration 未改。OpenAPI 0.5.0 共 50 个操作，34 个已实现，源输入当前明确仅 text；SSE、导出、归档、改写与真实 AI 仍为未来能力。
