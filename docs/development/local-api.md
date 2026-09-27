# 本地 API：文本 JD、双文档 Mock 与固定版本导出

已实现 40 个业务操作和 2 个健康检查。先运行 migrations，再独立启动 API；启动时校验 schema 版本为 7，不自动迁移。默认仅监听 127.0.0.1:8080。前端通过独立 Vite 服务启动，见 [前端说明](frontend.md)；API 根路径仍返回 JSON 404。

## 启动

先安装 Poppler（macOS: `brew install poppler`；Debian/Ubuntu: `apt-get install poppler-utils`），确保 `pdfinfo` 在 PATH，或设置 `PDFINFO_BIN` 为绝对可执行路径。然后创建专用本地 PostgreSQL 数据库 `applyflow`，根据本机账号设置连接字符串，然后在项目根目录运行：

```sh
export DATABASE_URL='postgresql://localhost/applyflow?sslmode=disable'
export APP_ENV=local
export PUBLIC_ORIGIN='http://localhost:8080'
export FILE_STORAGE_DIR="$PWD/var/private-files"
export HTTP_ADDR='127.0.0.1:8080'
export AUTH_SIGNING_KEY="$(openssl rand -base64 32)"
export CSRF_KEY="$(openssl rand -base64 32)"
make migrate-up
make run-api
```

两个 key 必须不同且各至少 32 随机字节，以 base64 输入；不要提交 key。上述临时 key 在重开终端后需重新配置，换 key 会使已有会话失效。程序不自动读取 `.env`。`AUTH_TTL` 默认 30m（1m–1h），`DB_MAX_CONNS` 默认 10（2–50）。Ctrl-C 触发有界关闭。

生产环境要求 HTTPS PUBLIC_ORIGIN，由反向代理终止 TLS；Cookie 自动设置 Secure。本次实现使用有界内存限流，**仅支持单 API 进程部署**；多副本前需实现共享限流。目前不信任 forwarded headers，代理后以代理 IP 限流，需在实际部署时明确可信代理策略。

## 已可调用

| 方法 | 路径 | 行为 |
|---|---|---|
| GET | `/health/live` | 进程存活 |
| GET | `/health/ready` | DB 连通且 schema 兼容 |
| GET | `/api/auth/csrf` | 获取 token 并设置签名上下文 Cookie |
| POST | `/api/auth/register` | 事务创建用户/空资料，返回身份并设置会话 Cookie |
| POST | `/api/auth/login` | 验证密码、设置会话 Cookie |
| POST | `/api/auth/logout` | CSRF 校验后清会话 Cookie，重复退出有效 |
| GET | `/api/auth/me` | 当前身份和过期时间 |
| POST | `/api/workspaces` | 幂等创建，可先不提供简历或 JD |
| GET | `/api/workspaces` | 未归档工作区分页 |
| GET | `/api/workspaces/{id}` | 读取自己的工作区（包括已归档） |
| PATCH | `/api/workspaces/{id}` | 版本控制更新标题/选择已有确认简历 |
| POST | `/api/files` | multipart 私有 PDF/DOCX 上传，purpose=base_resume |
| GET | `/api/files/{id}/download` | 校验所有权后以附件下载原件 |
| POST | `/api/resumes` | 显式 manual 导入，幂等创建，201，task_id=null |
| GET | `/api/resumes` | 账号内签名游标分页 |
| GET/PATCH | `/api/resumes/{id}` | 读取、重命名、切换默认简历 |
| POST | `/api/resumes/{id}/revisions` | 确认事实，保存不可变版本 |
| GET | `/api/resumes/{id}/revisions/{revision_id}` | 读取指定历史事实版本 |
| POST | `/api/workspaces/{id}/sources` | 文本 JD、任务/outbox 与幂等接受 |
| GET | `/api/workspaces/{id}/sources/{source_id}` | 读取原始文本来源 |
| POST | `/api/workspaces/{id}/job-revisions` | 确认当前来源的公司/岗位/JD |
| GET | `/api/workspaces/{id}/job-revisions/{revision_id}` | 读取确认的 JD 快照 |
| POST | `/api/workspaces/{id}/generations` | 原子接受 Mock 双文档生成 |
| GET | `/api/workspaces/{id}/generations/{run_id}` | 两项任务的最新状态 |
| POST | `/api/workspaces/{id}/generations/{run_id}/retry` | 仅重试失败/取消的一项 |
| GET | `/api/workspaces/{id}/documents` | 读取两份文档身份 |
| GET | `/api/documents/{id}` | 读取文档 head 和版本 |
| GET/POST | `/api/documents/{id}/revisions` | 历史分页/手工保存新版本 |
| GET | `/api/documents/{id}/revisions/{revision_id}` | 读取指定正文版本 |
| POST | `/api/documents/{id}/apply` | 明确应用当前 run 的候选版本 |
| POST | `/api/documents/{id}/exports` | 固定已保存版本，接受或复用 PDF/DOCX 导出 |
| GET | `/api/documents/{id}/exports/{export_id}` | 查询导出任务与私有文件引用 |
| GET | `/api/tasks/{id}` | 安全任务状态，可轮询 |
| POST | `/api/tasks/{id}/cancel` | 取消非终态任务，已终态保持原样 |

其余 OpenAPI 操作尚未实现，返回 404。自动简历提取、真实 AI 尚未实现，前端已接通现有 Mock 流程。简历事实与完整个人资料设置是不同功能，后者仍待接入。

## 客户端调用顺序

1. GET `/api/auth/csrf`，保留响应 Cookie 和 JSON `csrf_token`。
2. POST register/login，发送 `Origin: http://localhost:8080`、`X-CSRF-Token` 和 JSON。登录后旧 CSRF 失效，重新 GET csrf。
3. 携带 Cookie 调用 workspace。创建时额外带 UUID `Idempotency-Key`；同 key/body 返回原响应，同 key 不同 body 返回 409。
4. 修改例子：`{"expected_version":1,"title":"Northstar"}`。过期版本返回 409；客户端保留草稿后重新获取。`resume_revision_id:null` 清除选择，省略则不改。
5. 列表使用 `?limit=20&cursor=...`；游标签名且绑定账号，只作为翻页边界，不是授权。修改中的列表不保证历史快照。
6. 退出也需要当前 Origin、CSRF 和 Cookie。JWT 无 refresh/服务端吊销：清除 Cookie 不会撤销已被复制的 JWT，后者到期失效。

普通请求上限 128 KiB。拒绝重复 JSON key、未知或大小写错误字段、多个 JSON 值、无效编码、非法 UUID。错误为 application/problem+json，携带 request_id，不回显密码、Cookie、数据库连接或 SQL 参数。

## 文件职责与当前实现取舍

- `cmd/api` / `bootstrap`：配置、依赖组装、数据库池、健康检查和关闭。
- `auth` / `studio`：认证和工作区用例、输入规则、窄 Store 接口，无 Gin/SQL 依赖。
- `adapters/postgres`：显式 SQL 与完整事务，owner 过滤、CAS 和幂等响应。
- `adapters/security`：bcrypt、固定 HS256 JWT 验证、签名 CSRF。
- `transport/httpapi`：Gin 路由、中间件、DTO、严格 JSON、错误映射和签名分页。

当前 auth/workspace/files/resume/intake/document/task 用例，暂用参数化 pgx/database/sql 查询，不引入空 sqlc 输出；业务扩展后再统一 sqlc 查询生成。HTTP 公共中间件与按功能分开的 handler 文件保持一个 transport package，新增业务再按 feature 拆分，业务层已独立。

## 验证

```sh
make check
make test-migrations-local
```

前者验证 OpenAPI、契约、Go 单元测试/vet/build；后者在 `/tmp` 临时 PG 中验证迁移与真实 Gin handler + 数据库集成。HTTP 测试使用 httptest 请求，不依赖真实账号、外部模型或公开监听端口。测试覆盖注册事务、账号隔离、CSRF/会话轮换、Cookie 属性、幂等重放及并发、CAS、分页篡改、无效 JSON/超大输入和归档限制。单元测试覆盖 JWT 签名/issuer/audience/expiry/算法、CSRF 绑定及限流并发。

依赖锁定于 go.mod/go.sum；[Gin releases](https://github.com/gin-gonic/gin/releases) 与 [JWT releases](https://github.com/golang-jwt/jwt/releases) 是更新时核对兼容性的来源。

## 简历调用流程

登录并重新获取 CSRF 后：

1. POST `/api/files`，multipart 字段 `purpose=base_resume` 和 `file`。不要自行设 multipart boundary；保留 Cookie，发送 Origin 和 X-CSRF-Token。返回文件 id。
2. POST `/api/resumes`，发送 UUID Idempotency-Key 和 `{"file_id":"<file UUID>","name":"Base resume","import_mode":"manual"}`。返回 201，`resume.current_revision_id=null`、`task_id=null`。这表示待用户填写确认，不表示自动解析中。
3. POST `/api/resumes/<resume UUID>/revisions`，如下提交事实。每个事实使用稳定 UUID；编辑事实沿用其 ID，新增事实用新 ID。

```json
{
  "expected_version": 1,
  "facts": [{
    "id": "10000000-0000-4000-8000-000000000001",
    "category": "experience",
    "text": "Built and maintained Go APIs.",
    "evidence": {"source": "user", "page": null, "excerpt": ""}
  }]
}
```

返回新 revision 和 resume_version。每次确认提交完整事实数组（1–200 条），不是局部合并。发生 409 时保留草稿并重新读取当前版本。

4. PATCH `/api/workspaces/<workspace UUID>`，`{"expected_version":1,"resume_revision_id":"<confirmed revision UUID>"}`。已有工作区不会自动切换到后续简历版本。
5. 可选 PATCH resume：`{"expected_version":2,"is_default":true}`。未确认不可设默认；切换时旧默认版本也推进，旧草稿需重新读取。`name` 可一并更新，null 不合法。

## 私有文件与运行边界

FILE_STORAGE_DIR 必须为绝对路径，目录权限 0700；新文件 0600。本地未设置时使用 API 当前目录下 var/private-files；生产必须显式配置。目录不能通过反向代理静态暴露，需持久化挂载并与 DB 一起备份。文件 key 随机生成且不返回前端。下载检查 DB owner、文件长度与 SHA-256，防止误读损坏原件。

上传限 10 MiB，整个 multipart 限 11 MiB，单进程最多 2 个并行上传，每账号每小时最多 20 次；下载按已验证文件大小有界读取。PDF 校验调用 Poppler pdfinfo（3 秒超时、64 KiB 输出上限），拒绝无效、加密和超过 20 页文件。DOCX 限 512 个 ZIP 项、每项 8 MiB、总解压 32 MiB；拒绝宏/嵌入对象/非超链接外部关系，不执行 XML 实体或任何文档内容。DOCX 实际分页尚不计算。ready 只代表通过格式检查并保存，不代表已提取或验证经历真实性。校验器尚非独立容器；部署公网前应为处理不可信文档的进程配置资源限制与隔离。

未知上传结果不自动重试。文件写入后 DB 失败或进程中断可能留下无引用文件；自动清理尚未实现，避免在 DB 提交不确定时误删。上传切片无需修改既有迁移；当前个人 AI 设置切片新增 00007，schema 为 7。

新增集成测试覆盖私有下载、文件伪装/超限/路径、手动导入幂等与并发、事实来源/重复 ID/嵌套非法字段、256 KiB 确认上限、确认 CAS、默认切换及旧工作区版本固定。

## 文本 JD 到 Mock 双文档

先按上文配置环境，分别在两个终端运行（第二个终端也需设置同一 DATABASE_URL）：

```sh
make run-api
# 另一个终端；Worker 需与 API 使用同一绝对 FILE_STORAGE_DIR，并安装导出 Python 依赖。
make run-worker
```

Worker 是独立进程；API 不在 HTTP 请求里执行生成。只启动 API 时返回的 queued 会保留在 DB，直到 Worker 启动。当前使用 PG outbox 直接派发，不需要 Redis。Worker 无外部 AI 调用，启动日志标明 mock-v1。

完成简历确认并选入 workspace 后，使用 GET workspace 读取当前 version：

1. POST `/api/workspaces/{id}/sources`，带新的 Idempotency-Key：`{"expected_version":2,"source":{"kind":"text","text":"Backend Engineer: Go and PostgreSQL."}}`。返回 source、workspace_version 和 task_id。
2. GET `/api/tasks/{task_id}` 轮询至终态。文本准备任务返回原文，company/role_title 为 null；没有猜测公司岗位的 AI。也可以直接依据原文确认，不依赖提取成功。
3. POST `/api/workspaces/{id}/job-revisions`：`{"expected_version":3,"source_id":"<source UUID>","expected_source_version":1,"company":"Northstar","role_title":"Backend Engineer","job_description":"Build Go services."}`。返回 revision 和新的 workspace_version。source 必须仍是当前版本；换 JD 会清空已确认 JD 选择。
4. POST `/api/workspaces/{id}/generations`，带新的 Idempotency-Key：

```json
{
  "expected_version": 4,
  "job_revision_id": "<confirmed job revision UUID>",
  "resume_revision_id": "<selected confirmed resume revision UUID>",
  "expected_profile_version": 1,
  "execution_mode": "mock",
  "locale": "en"
}
```

这里版本号仅示例，使用实际响应中的版本；profile_version 由 auth/me 返回。生成前选中的 JD/resume revision 与请求必须一致。一次接受原子创建两个任务和 outbox、固定输入 run、工作区版本和幂等响应。若响应丢失，用相同 key/body 重试；不能先换 expected_version。活跃文档已有任务时，新 key 的生成请求返回 409。

5. GET `/api/workspaces/{id}/generations/{run_id}` 同时读取两项最新任务。completed 的 result 给出 document_id/revision_id。两项独立完成，客户端保留已成功项。
6. GET `/api/workspaces/{id}/documents`、GET `/api/documents/{id}`、GET `/api/documents/{id}/revisions/{revision_id}` 读取文档身份、head 和正文。版本列表为 GET `/api/documents/{id}/revisions?limit=20&cursor=...`，游标绑定账号和文档。
7. POST `/api/documents/{id}/revisions`，发送 expected_version、base_revision_id 和完整 content。Resume 使用 kind/title/sections；Cover Letter 使用 kind/salutation/paragraphs/closing。具体 schema 见 OpenAPI。保存产生新 revision 和 head；409 时保留本地草稿。
8. 首次生成只在 document 没有 head 且版本/run 仍匹配时初始化。后续结果作为候选，不自动覆盖；POST `/api/documents/{id}/apply`，`{"expected_version":3,"revision_id":"<current-run candidate UUID>"}` 明确应用。
9. POST `/api/tasks/{id}/cancel` 取消未完成任务；本地 mock 取消立即终态，运行中的计算可能在内存继续，但提交被拒绝。成功/失败/已取消任务原样返回。
10. POST `/api/workspaces/{id}/generations/{run_id}/retry`，带新 Idempotency-Key 和 `{"expected_version":5,"kind":"cover_letter"}`，只重试当前 run 的失败/取消项；成功的一份保持不变。工作区版本会推进，重新 GET workspace 获取。

所有写操作仍需要当前 Cookie、Origin、X-CSRF-Token。来源限 60 次/账号/分钟，生成限 30 次/账号/分钟。文本 JD 限 64 KiB UTF-8，普通 JSON 128 KiB，文档保存 256 KiB。

## Worker 边界与验证

- `intake`：文本与确认输入规则；`studio/generation.go`：生成请求和响应模型。
- `task/worker.go`：独立任务循环、领取、恢复、失败处理；`generation/execute.go`：调用 Provider、校验结构与事实引用；`adapters/provider/mock.go`：确定性模拟正文，无网络/模型调用。
- `adapters/postgres/{intake,generation,document,task}_store.go`：接受事务、持久查询、版本更新和任务状态；`flow_store.go` 提供共享幂等事务辅助。
- Worker 每次领取一个任务，锁使用 SKIP LOCKED，30 秒租约、10 秒执行预算；短 Mock 任务不续租。到期进入 retry_wait，1 秒后重新投递，最多 3 次领取；旧 fence、取消或已完成任务不能再提交。
- 本地派发在同一事务消费 outbox 并领取任务；重试增加 delivery_generation。Redis/Asynq 派发、长任务续租和外部调用预算仍待真实 AI 阶段实现。
- Provider 计算在事务外；完成事务锁 workspace → document → task，保存 revision 与 task.result 原子提交。数据库提交失败不留下假成功或半份正文。
- 运行环境仍为单主机；API 内存限流仍限制为单 API 进程。多 Worker 抢任务已在测试中验证，但未进行公网部署/负载测试。SSE 尚未实现，前端已接通现有 Mock 流程，当前通过 JSON 轮询使用。

`make check` 通过契约、Go 单元测试、vet 和三个二进制构建。真实 PostgreSQL 集成测试覆盖完整两份草稿、超 32 KiB 来源幂等重放、并发接受、提交事务失败回滚、单份重试、跨账号读取/取消、手工编辑时晚到候选、游标隔离、重复/过期 Worker、崩溃预算与结果事务回滚。当前本地验证为 PostgreSQL 13.20；CI 配置 PostgreSQL 17，尚未在本次会话执行 CI。

## 固定版本 PDF/DOCX 导出（0.5.0）

先运行 `make setup-tools` 安装锁定的 ReportLab/python-docx 依赖。Make 默认使用项目 `.venv/bin/python`；直接运行 Worker 时设置 `EXPORT_PYTHON` 为该 Python 的绝对路径。API 与 Worker 必须配置同一个绝对 `FILE_STORAGE_DIR`，并共享持久存储。Worker 启动时检查渲染依赖；无需 LibreOffice，LibreOffice 仅用于开发时 DOCX 排版验收。

1. 保存编辑，取得目标 revision UUID。
2. POST `/api/documents/{id}/exports`，带 Cookie、Origin、X-CSRF-Token 和新的 UUID Idempotency-Key：

```json
{"revision_id":"<saved revision UUID>","format":"pdf","template_version":"1"}
```

`format` 可为 `pdf` 或 `docx`。202 返回 export 身份和 task_id；GET `/api/tasks/{task_id}` 轮询执行状态，再 GET `/api/documents/{id}/exports/{export_id}` 取得 file_id，通过 GET `/api/files/{file_id}/download` 鉴权下载。生成期间修改 head 不改变已接受的导出内容。

同 revision/format/template 复用同一导出：活动任务返回 202；新 key 请求已完成文件返回 200；失败或取消后，新 key 创建替代任务，保留 export ID。原 key 始终重放原始响应快照，因此须 GET 查询最新状态，不能把重放当成刷新。导出提交限每账号每分钟 30 次。

实现职责：`internal/export` 校验输入并编排渲染/私有存储；`adapters/render` 是固定模板的 Python 子进程；`adapters/postgres/export_store.go` 管理接收、去重、重试和带 fence 的完成事务；`task.Router` 共用既有 Worker。先写私有文件，再在同一数据库事务写文件元数据、export 引用和完成结果。过期 lease、取消或事务失败不能产生可下载的假成功；提交不确定时保留无引用文件，清理机制仍待实现。

模板 1 使用简洁 A4 黑灰排版，PDF 文本可选择、DOCX 正文可编辑。首版字体仅覆盖受支持的英文/Latin 字符；中文、emoji 等超出字体覆盖时明确失败 `export_unsupported_character`，不会悄悄替换。固定 JSON 文本经过转义，不读取外部图片、HTML 或 URL。每次渲染限 8 秒、输出 10 MiB，并通过共享 PDF 排版预检限制 100 页；Word 字体替换可能使实际 DOCX 分页不同。Python `-I` 是解释器隔离模式，不等于操作系统沙箱。

验证包括：两种格式的固定版本内容、并发/重放、跨账号隔离、取消重试、过期执行者和数据库提交失败；一页简历、一页求职信、跨页简历均完成 PDF 与 DOCX 渲染检查。本地 PostgreSQL 13.20 的完整集成测试含 race 检测通过；CI PostgreSQL 17 尚未在本次会话执行。

## 上传 415 的排查

重启更新后的 API，再查看响应 JSON 的 `code` 或日志的 `error_code`：

- `invalid_pdf`：PDF 无法读取，也可能有打开密码；重新导出无密码 PDF。
- `pdf_restricted`：检测到 PDF 加密或权限限制；使用无保护副本。
- `pdf_page_limit`：PDF 超过 20 页。
- `unsupported_source_file`：不是可接受的 PDF/DOCX，或 DOCX 结构/安全校验失败；旧 .doc、图片和 .pages 不支持。
- `unsupported_media_type`：请求不是正确的 multipart 上传；前端用 FormData，不能手动填写 Content-Type 边界。
- `file_validator_unavailable`（503）：PDF 校验进程执行失败或超时；检查 PDFINFO_BIN 指向可执行的 Poppler pdfinfo。不要将此故障归咎于用户文件格式。

日志只记录稳定错误码，不记录正文、校验器原始输出或私有路径。仅有旧版 415 日志无法判断具体文件问题。

## 个人 AI 配置（0.6.0）

新增服务商目录和个人配置读取/保存/测试/启用/删除六个接口，详见 [配置与调用说明](ai-settings.md)。已有 Mock 流程不要求部署加密主密钥；配置该能力时需要额外的独立持久主密钥。AI 设置前端已接通，真实生成仍未接通。
