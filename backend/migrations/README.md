# 数据库迁移维护

迁移由 Goose 执行，每个文件的 Up/Down 默认各自处于事务中。PL/pgSQL 函数使用 StatementBegin/StatementEnd，所有文件嵌入 Go 迁移命令。

| 版本 | 内容 |
|---|---|
| 00001 | users + user_profiles，规范化登录邮箱和 profile_version |
| 00002 | skills + skill_aliases + user_skills |
| 00003 | applications + application_status_history + 版本/历史触发器 |
| 00004 | 8 个基础技能和别名，使用固定 UUID |
| 00005 | files、resumes/revisions、workspaces、job_sources/files/revisions |
| 00006 | documents/revisions、generation_runs、job_tasks、task_outbox、幂等、导出及申请文档关联 |

## 事务责任

**注册**：同一个事务 INSERT users 和空 user_profiles。应用生成 bcrypt hash；数据库只验证哈希格式，不能验证输入实际执行过 bcrypt。

**资料/技能写入**：先 `SELECT profile_version FROM users WHERE id=$owner FOR UPDATE`，比较客户端版本，执行资料/技能变更，再 `UPDATE users SET profile_version=profile_version+1, updated_at=...`，返回新版本并提交。更新 user_profiles/user_skills 时显式维护 updated_at。回滚必须同时撤销数据与版本。本批只提供表和约束，下一阶段实现该 service/repository 事务。

**职位写入**：repository 必须用 `WHERE id=$id AND owner_id=$owner AND version=$expected`，并按业务规则排除 archived 记录。`maintain_application_versions` 自动递增 version、判断 jd_version 并更新时间；`record_application_status` 自动写初始状态/真正状态变化。repository 不再手动提升版本或额外 INSERT history，避免双重执行。

用户归属外键阻止“历史属于 Alice、职位属于 Bob”这种错误关联；它不会阻止没有 owner 条件的 SELECT。跨账户 HTTP 授权仍由下一阶段实现并测试。

## 权限与时间

迁移使用拥有 DDL 权限的专用账号。当前历史触发器是 SECURITY INVOKER，运行账号需要 INSERT history 权限；不要误以为触发器已提供独立防篡改审计。应用禁止直接编辑历史，生产角色/独立审计要求在部署前另行落实。运行账号不需要 DDL 或更改触发器权限。

时间存 timestamptz，业务 date_applied 存 date。history.occurred_at 在本批代表状态实际记录时间，不从 date_applied 倒推。如果后续支持补录过去的阶段，应增加独立、有校验的命令。

所有可选文本的 null 表示未填写；HTTP 对 URL/email 做完整解析，数据库只是最后的长度/格式防线。notes 和 JD 允许空串。UUID 使用 PostgreSQL 内建 gen_random_uuid，不依赖扩展安装。

## 升级、回滚与测试

已发布编号文件不可修改；新增文件表达变更，采用 expand/contract。Down 只用于临时环境；删除种子技能若已有用户引用会因 RESTRICT 失败，这是有意保护数据。账号删除目前也会被仍有职位的外键阻止，不能直接 DELETE users 作为未来删除账户方案。

`make test-migrations-local` 或 `make test-integration` 用实际 Goose 验证 clean up、重复 up、约束、owner 条件、并发 CAS、事务回滚、down-all 和重新 up。测试在随机 schema 中进行，默认 search_path 不包含 public，不清理现有 schema。

本地当前可用 PostgreSQL 是 13.20，CI 目标是 17；本地旧版本只用作临时兼容性测试，不建议用来部署。CI 文件尚需推送到 GitHub 后运行，不能把 CI 配置的存在当作 CI 已通过。

参考：[Goose SQL annotations](https://pressly.github.io/goose/documentation/annotations/)、[PostgreSQL constraints](https://www.postgresql.org/docs/current/ddl-constraints.html)。

## Studio 的约束与事务责任

- 新的 resumes/workspaces/documents 由 maintain_studio_version 自动提升 version 和 updated_at。repository 必须带 owner/expected_version，不能自行再加一次版本。
- job_sources、已确认简历/JD revisions、generation_runs、document_revisions 禁止 UPDATE。修改新建 revision；删除/保留期并未实现。ready/rejected 文件不可替换内容。
- 复合 FK 防止跨账号指针，也防止 workspace 指向另一工作区的 JD/run、document 指向另一文档的 revision。任务和文档种类、revision 的 run 工作区另有触发器检查。
- 文件元数据表不代表已完成上传。resumes 和截图关联要求 ready 且 purpose 正确；图片 source 的 1–5 张完整性、唯一事实 ID/证据、完整文档 schema 在接受事务/service 校验。
- 一份文档最多一项活动生成/修改；解析按 source/resume 去重。任务只可从 queued 开始，claim 同时增加 attempts/fence；续租不增加可见 state_version。终态不能 UPDATE，重试失败文档创建新 task。
- 完成 SQL 仍必须有 `status='running' AND fencing_token=$fence AND lease_until>clock_timestamp() AND NOT cancel_requested`。触发器校验状态/租约不能代替请求携带的 fencing token。结果 revision 与完成在一个事务；候选不会自动更新 head。
- 接受生成：锁 user/workspace/documents → 比对/预留/固定输入 → run + 两项 task + 两项 outbox + 幂等响应 → 更新 workspace → commit。新增表仅支持 mock；真实 AI 上线前新增凭据版本 FK、用量账本和传输范围快照。
- idempotency_requests 的唯一键是 owner/operation/key_hash；service 比较 request_hash 决定重放或 409，记录保留至少 24 小时且不早于关联任务终态。SQL 唯一约束本身不实现响应重放。
- 导出唯一 revision/format/template；显式重试可以替换失败 task_id，不能改目标版本。file_id 必须是同 owner、ready、用途 export 且 MIME 匹配。上传/模型/导出调用一律在数据库事务外。

Studio 测试覆盖旧库 4→6 升级保留已有职位、6→4→6、原有全量 up/down、复合关系与不可变数据、并发 CAS、活动任务唯一、租约/取消/终态、半套事务回滚、幂等唯一与导出固定版本。所有测试使用独立随机 schema；没有新增线上运行服务。

## 00007 — Personal AI settings

Adds owner-bound encrypted credentials, immutable configuration revisions, versioned settings and durable connection-test receipts. API and worker readiness require `migrations.LatestVersion` (7). Credential encryption happens in the API with a separate persistent master key; SQL never receives plaintext. Old migrations remain unchanged. See [configuration and lifecycle](../../docs/development/ai-settings.md).

## 00008 — Personal generation

Binds immutable generation runs to owner-matched AI revisions and adds durable per-task request reservations/usage. API and worker require schema 8. Provider calls use an at-most-once dispatch marker per task; explicit retries create new tasks. See [live generation](../../docs/development/live-generation.md).

## 00009 — aiwanwu provider

Requires schema version 9 in API and worker. Allows only valid registered provider/model pairs: OpenAI (`gpt-4.1-mini`, `gpt-4.1`) or aiwanwu (`gpt-6-sol`). Existing official revisions remain valid and unchanged. Down migration refuses if relay history exists; it does not remove credentials or generation history to force a downgrade.

## JD-only generation (00010)

Job revision company/role strings may be empty. The full JD remains required and immutable; the model interprets company and role within the existing two document calls. Create never blocks on a metadata form. Missing names must not be invented. Application tracking fields retain their existing constraints. Migration 00010 refuses rollback while empty job metadata exists; it never rewrites historical snapshots.
