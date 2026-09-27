# HTTP 契约 0.6.0 — Foundation + Studio

`openapi.yaml` 是本批实现的唯一公共协议来源，版本 0.6.0。它可被 OpenAPI 工具读取，其中 40 个操作已实现，覆盖文本 JD、Mock 双文档和版本编辑；其余 handler 尚未实现。实际列表见 [本地 API](../docs/development/local-api.md)。

## 核心决策

- 56 个操作（原有 17 + Studio 33 + AI 设置 6）；所有写操作明确要求 CSRF header 与同源 Origin 策略。
- 注册返回 201 + Session；登录返回 200 + Session；令牌只通过 HttpOnly Cookie；注销返回 204。登录邮箱 trim/lower，密码绝不 trim。
- 个人资料仅 basics。新用户 profile_version=1；注册事务创建空 profile。偏好和经历暂不接受。
- profile/skill 写操作锁 users、检查版本、修改数据并提升 profile_version 一次，全部原子提交。DELETE 的 If-Match 承载相同版本号，其他写操作放在 JSON expected_version。
- 技能查询按 alias 查找、按 canonical ID 去重。未收录技能返回空列表，首版不允许用户创建全局技能。没有自评就是未评估。
- 职位部分更新用 PATCH。只读字段不能通过创建/编辑请求注入。owner 永远来自会话。
- 状态更改可前进也可后退以纠错；首次保存记录初始状态，历史由数据库触发器写入。只改 JD/notes/归档不记录伪状态变化。
- 普通编辑提升 version，只有 JD 改变才提升 jd_version。触发器维护数值，SQL WHERE 的 owner/version 条件负责权限与乐观锁。
- archived=true 的列表只看归档，false 只看未归档；详情和历史仍可读，归档后编辑返回 409。
- 列表为 cursor 分页，无 total/count 保证；筛选变化必须重置 cursor。状态历史单独分页，不让历史增长撑大详情。
- 日期/URL/email/UTF-8 byte 上限需要 handler 做语义校验；OpenAPI JSON Schema 的 maxLength 是字符数，不能单独保证 bcrypt/JD 的字节限制。
- 本批没有分析任务 DTO，不返回 password_hash 等存储字段。后续 AI 任务按操作区分数据范围，联系方式默认本地合并，不发送到模型。

## 版本错误与重试

缺失/非法 expected_version 或 If-Match 返回 400；格式正确但陈旧返回 409。资源非本人或不存在返回 404，不携带其他用户的 current_version。

创建职位没有幂等键契约；前端不能自动重试结果未知的 POST。编辑/归档重复请求可因版本提升返回 409，客户端查询最新状态确认。后续分析任务才引入专用 Idempotency-Key。

本批生成请求只允许显式 mock 模式，文本 JD、生成 Worker 与双文档版本 HTTP 已实现。平台/个人模式、凭据固定版本、用量扣减和旧凭据清理需要下一阶段完整契约与迁移，不能把当前测试通过视为真实 AI 已完成。

## 验证

`make check-contract` 验证 OpenAPI、引用、重复 YAML key、操作 ID、路径参数、CSRF、缓存策略和 schema 示例；`make test-contract` 检查字段注入、版本要求、nullable、日期、URL 和枚举。HTTP 权限和 cookie/CSRF 行为要等第二步 handler 实现后再做集成测试。

## Studio 已定义的边界

本次把工作区、三类 JD 来源、基础简历导入/确认、两份文档的生成/编辑/候选应用、任务/SSE/取消、PDF/DOCX 导出及保存申请写入 OpenAPI。工作区独立于 application，不改变旧职位的必填公司/岗位约束。

- JSON 默认上限 128 KiB；简历事实确认/文档编辑 256 KiB，操作含 `x-max-body-bytes`。截图/简历单文件 10 MiB，multipart 总请求 11 MiB；图片解码 25MP、简历 20 页仍需解析器验证。
- SourceCreate 使用 text/url/image 互斥联合，不允许混合字段；图片 ID 去重且最多 5 个。HTTPS 格式校验不等于 SSRF 防护。
- 新建工作区、来源、简历导入、生成、单份重试、AI 修改、导出和关联申请要求 UUID Idempotency-Key。保存响应与对应任务/outbox 在同一事务；重放先于版本检查，相同 key 不同 body 返回 409。
- 生成固定 job/resume revisions、profile_version、locale=en、execution_mode=mock。尚不接受 expected_ai_config_version 或任何密钥字段；真实模型阶段另行扩展。
- Resume 与 Cover letter 子任务独立，重试仅针对失败项。TaskSnapshot 的终态时间、状态、结果种类必须一致；SSE data 使用同一 schema，客户端按 task_id/state_version 归并。
- 人工保存校验 base_revision_id 与 head，应用候选校验 document/run，二者都执行版本 CAS。历史 revision 不 UPDATE；原文件不替换。导出锁定 revision/format/template。
- GET workspace documents 和 GET document revisions 提供发现文档及候选的入口。下载只返回 ready 文件；所有资源均查 owner，不能依赖 UUID 隐蔽性。

完整目标设计见 [Studio 架构](../docs/architecture/application-studio.md)。其真实 AI 部分仍是未来设计；与当前字段有差异时以本 YAML 为实际批次协议来源。

## 尚需 handler/service 实现的语义

SQL 负责复合 owner 关系、不可变 revisions、唯一活动任务、状态转换和版本递增。认证/工作区已实现 owner 读取过滤、expected_version 条件、创建请求 hash 比对/重放与 JSON 字节上限；其他 Studio 模块的同类逻辑、事实 ID 与来源校验、完整 URL/SSRF、解析、文件删除仍待实现；固定版本导出可读性已有样本验证。数据库测试验证约束和事务模式；HTTP 集成测试另验证认证/工作区授权、CAS 和幂等。模型/文件等未实现模块不在已通过范围内。

## 0.3.0 手动简历导入

这是对尚未实现的 0.2.0 自动导入契约的一次显式调整：POST `/api/resumes` 必须传 `import_mode: manual`，返回 **201** 和 `task_id: null`；不创建 parse_resume/outbox。待 Worker 可运行后再增加自动模式，不能让客户端永远等待一个不会执行的任务。文件上传当前仅接受 base_resume（PDF/DOCX），截图上传保留为后续能力；FileUpload schema 已同步收窄。PDF 20 页上限由 pdfinfo 校验；DOCX 以解压大小/结构限制为准，页数要在后续渲染时判断。

确认通过 POST `/api/resumes/{id}/revisions`，需要完整事实数组、来源和 expected_version。默认简历必须先确认；切换默认会同时推进旧默认的版本。已有工作区固定引用旧 revision，不随基础简历更新。事实来源是用户声明，不代表系统已核实经历。

## 0.4.0 文本 JD 与 Mock 文档

新增 15 个已实现操作：来源创建/读取、JD 确认/读取、生成/读取/单份重试、文档列表/读取/版本列表/版本读取/保存/应用，以及任务读取/取消。SourceCreate 当前仅接受 text；URL/图片仍为目标能力，不能交给本次 Worker。生成模式仍必须显式 mock，不接受个人 Key。

202 表示事务已保存 source/run、task/outbox、工作区版本和幂等响应；需独立启动 Worker 才会推进。文本提取只返回原文，company/role 为 null，用户必须确认。Mock 结果仍使用 schema 的 origin=ai（自动结果），generation.execution_mode=mock、标题/正文和 change_summary 标明模拟；这不表示调用了 AI。手工编辑 origin=manual，引用的 fact_ids 必须来自原 run 的已确认简历，允许空引用但不视为真实性证明。

当前取消会立即使任务变为 cancelled，并阻止旧执行者提交；终态不改动。只支持轮询 GET task；SSE/改写/归档仍为 contract-only。固定 head 的手工编辑在新 run 运行期间仍可保存，保留原 run 来源；新结果成为候选，Apply 只能选择当前 run 的版本。

## 0.5.0 固定版本导出

createDocumentExport/getDocumentExport 两个操作已实现，累计 34/50。PDF/DOCX 均锁定已保存 revision，复用私有下载和共享 Worker；新 key 对失败/取消导出可重试，对 ready 文件返回 200，原 key 重放原始快照。模板 1 的字体覆盖、资源限制、依赖和运行步骤见 [导出说明](../docs/development/local-api.md#固定版本-pdfdocx-导出050)。

## 0.6.0 个人 AI 配置

新增六个已实现接口，累计 40/56：服务商目录、配置读取/保存/测试/启用/删除。加密主密钥、CAS 版本、幂等测试和删除边界见 [实施说明](../docs/development/ai-settings.md)。生成仍只接受 mock，个人日限额目前仅保存，不宣称真实生成或用量控制已接通。
