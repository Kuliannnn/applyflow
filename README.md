# ApplyFlow

以 **JD 截图/文字/链接 → AI 定制简历与 Cover Letter → 审阅编辑 → 导出** 为核心的求职工具。默认英文，采用已确认的浅色混光磨砂玻璃设计。申请跟踪是辅助流程。

当前交付 **基础层 + Application Studio 契约/数据库结构（0.5.0）**，已实现文本 JD → Mock 双文档草稿 → 编辑/版本保存的后端流程，英文磨砂玻璃前端已接通此流程。

## 已完成的范围

- [OpenAPI 3.1 契约](api/openapi.yaml)：50 个操作，覆盖原有认证/资料/职位，以及私有文件、JD 来源、基础简历、工作区、双文档任务、版本、导出与关联申请。
- [SQL migrations](backend/migrations)：00001–00004 保留；新增 00005（私有文件、简历事实、工作区/JD 来源）和 00006（文档版本、生成批次、任务/outbox、幂等与导出）。
- [迁移命令](backend/cmd/migrate/main.go)：Goose + pgx，嵌入 SQL、有界超时、数据库迁移锁，不提供生产 down 命令。
- 契约测试、真实数据库约束/并发测试及 CI 配置。

Studio 的公开契约与表约束已落地；34 个业务操作已有 handler，独立 Worker 能处理文本 JD 和 Mock 双文档生成；真实 AI、自动简历解析、用户 AI Key、完整个人资料仍待实现；面试记录与统计排在核心流程之后。可调用范围和启动步骤见 [本地 API 说明](docs/development/local-api.md)；其余操作仍只有契约。0.5.0 的生成请求显式要求 execution_mode=mock，已可验证模拟执行链路，不接受个人 Key 或伪装真实 AI。

## 运行前端

已实现英文 Light Glass Studio：注册/登录、文字 JD、简历事实确认、双文档编辑、版本冲突对比及 PDF/Word 导出。运行 `npm ci --prefix frontend` 和 `make run-frontend`；API 需设置 `PUBLIC_ORIGIN=http://127.0.0.1:5173`，并独立启动 Worker。完整步骤和当前功能边界见 [前端说明](docs/development/frontend.md)。

## 运行 API

按 [本地 API 启动说明](docs/development/local-api.md)配置数据库、Origin 与两个独立随机 key，运行 `make migrate-up` 后执行 `make run-api`。已支持注册/登录/退出、CSRF、当前身份和工作区创建/读取/修改/分页；默认仅本机单进程。

## 本地验证

需要 Go 1.24+、Python 3.13（验证工具）和 PostgreSQL。CI 使用 PostgreSQL 17；本地测试可通过 `PG_BIN` 指定安装路径。

```sh
make setup-tools
make check
make test-migrations-local
```

`make check` 检查 OpenAPI、契约行为、Go 格式/vet/编译。`make test-migrations-local` 在 `/tmp` 创建全新的数据库，只监听私有 Unix socket，测试完成后停止并清理；不会使用 `.env` 或已有业务数据库。若找不到 PostgreSQL：

```sh
PG_BIN=/path/to/postgresql/bin make test-migrations-local
```

已有专用测试数据库也可以使用：

```sh
export TEST_DATABASE_URL='postgresql://localhost/applyflow_test?sslmode=disable'
make test-integration
```

测试数据库名称必须以 `_test` 结尾；测试只创建和清理随机 schema，不清空 public。没有数据库连接时 `make test-integration` 会失败而不是假装通过。

## 应用迁移

先由部署者创建目标数据库，配置进程环境：

```sh
export DATABASE_URL='postgresql://localhost/applyflow?sslmode=disable'
make migrate-status
make migrate-up
make migrate-status
```

示例只适用于本机开发；生产连接按部署策略启用 TLS 和独立凭据。`.env.example` 仅作说明，程序不自动读取 `.env`，防止误连目标。迁移命令只支持 `up` 和 `status`；SQL 的 Down 段用于隔离环境验证，不能当作生产数据回滚计划。

## 维护规则

1. 修改公共字段前先改 `api/openapi.yaml`，再改对应 SQL/测试。YAML 是直接维护的源文件，没有另外一份生成脚本来源。
2. 已发布 migration 不修改，新增编号文件。Goose 自动记录版本；嵌入 SQL 后必须重新构建迁移制品。
3. 请求 DTO 不接收 `owner_id`、数据库 ID 或版本回写字段；owner 来自会话。数据库外键不替代 HTTP 权限检查。
4. 职位版本与状态历史由数据库触发器维护，repository 不重复写历史。个人资料版本由 service 在锁 users 的同一事务中维护；详细规则见 [迁移说明](backend/migrations/README.md)。
5. 每批只增加当前可验收功能，不提前创建空 handler/service/adapter。先写一个能完整运行的流程再抽取共用代码。
6. Go 依赖固定在 go.mod/go.sum；Python 验证依赖固定在 `scripts/requirements.lock`。更新依赖后重跑契约和数据库测试。

## 设计入口与下一步

- [正式界面设计](docs/design/DESIGN.md)：已确认 V3，英文、浅色混光、磨砂玻璃、极简文档工作流。
- [Application Studio 架构](docs/architecture/application-studio.md)：新增模块、表、输入解析、任务、文档版本、导出及完整目标 API 设计。
- [前端 skill](skills/applyflow-light-glass/SKILL.md)：实现与审查时采用的设计约定。

已支持 PDF/DOCX 私有上传、显式手动导入、资料确认及版本选择；自动提取尚未接入。文本 JD 的模拟双文档生成与版本保存也已完成；固定版本 PDF/DOCX 导出已完成，正式英文玻璃前端已接通；下一步接入真实 AI。以 0.5.0 契约约束实现，不先搭建统计后台。真实 AI 凭据/用量和截图/链接解析在后续切片接入。

相关设计：[架构](ARCHITECTURE.md)、[API 边界与语义](api/README.md)、[产品规划](ApplyFlow_Project_Plan.md)。设计文档的完整目标大于本批已实现范围。

Mock 工作流使用两个独立进程：`make run-api` 与 `make run-worker`。Worker 当前直接消费 PostgreSQL 的持久 outbox，不需要 Redis；真实模型阶段再引入 Redis/Asynq 派发和续租。Mock 仅保留/排版已确认事实，明确标注模拟草稿，不具备真实 AI 的匹配改写能力。详见 [运行与调用顺序](docs/development/local-api.md#文本-jd-到-mock-双文档)。
