# 个人资料与个人 AI API 配置

状态：0.6.0 已实现个人 AI 配置的六个后端接口、加密存储和连接测试；设置页面已接通；个人资料与真实生成集成仍是目标设计。当前支持 OpenAI、personal 模式，daily_request_limit 仅保存，尚未接入生成扣减；实施细节见 [AI 设置说明](../development/ai-settings.md)。返回 [架构入口](../../ARCHITECTURE.md)。本文替代早期“只允许部署者配置统一 Key”的限制；包含页面、数据、接口和任务集成，不表示页面已经上线。

## 1. 用户使用流程

```text
注册 / 登录
  → /create：提交 JD，上传或选择基础简历；确认提取的事实
  → /profile：补充相关资料，可跳过
  → /settings/ai：选择服务商、填写自己的 API Key、选择模型
  → 保存 → 测试已保存配置 → 启用个人配置
  → 确认本次 JD/简历/选定资料 → 生成 Resume + Cover Letter
  → 审阅修改 → 导出 → 可选保存到申请记录
```

资料不完整仍可用已确认基础简历生成材料，不要求完成所有 profile 分区；技能未填写显示“未评估”。未配置 AI 时仍可保存输入/工作区，生成动作引导设置并恢复返回位置。截图若依赖外部 OCR 同样需要能力检查。

首版每用户一套当前 AI 配置，仅支持服务端登记的服务商和模型，不开放任意 Base URL。用户可选个人 Key，部署者可选开启平台 Key 模式。两种模式必须显式选择，个人 Key 失效后不能自动切平台额度。

## 2. 页面：个人资料 `/profile`

默认界面英文，遵循 [正式设计](../design/DESIGN.md)。账户菜单进入 Profile，桌面使用页内分区标签与表单，移动端纵向分区；不增加全局固定侧栏。以下中文为设计说明，正式文案使用 Basics / Preferences / Skills / Education / Experience。每区独立保存，显示“有未保存修改 / 保存中 / 已保存 / 保存失败”；离开含未保存修改的页面时提醒。必填只针对已创建的经历条目，不强迫用户完善全部资料。

```text
个人资料
  基本信息       [姓名] [联系邮箱] [所在城市]
                 [GitHub] [LinkedIn] [个人网站]
                                            [保存基本信息]
  求职偏好       [目标岗位] [目标地点] [远程/混合/现场]
                 [全职/兼职/合同/实习] [期望薪资，可选]
                                            [保存求职偏好]
  技能           Go · 能独立使用     [编辑] [移除]
                 Redis · 正在学习   [编辑] [移除]
                                            [添加技能]
  教育           学校 / 专业 / 学历 / 时间           [添加]
  工作经历       公司 / 职位 / 时间 / 成果           [添加]
  项目经历       名称 / 角色 / 技术 / 成果 / 链接     [添加]
  简历           基础简历与已确认事实版本（核心阶段）       [上传]
```

“联系邮箱”与登录邮箱分开，修改资料不会修改登录身份。资料默认私有，不生成公开个人主页。

### 字段与校验

| 分区 | 字段 | 规则 |
|---|---|---|
| 基本 | display_name、contact_email、phone、city、country_code | 全部可选；姓名 100 字符，邮箱 254，电话 32，城市 100；country_code 使用两位国家码 |
| 外部链接 | github_url、linkedin_url、website_url | 可选 HTTPS URL，最多 2048 字符；仅显示链接，不自动抓取 |
| 简介 | summary | 可选纯文本，最多 2000 字符 |
| 偏好 | target_roles、target_locations | 字符串数组，每组最多 10 项、每项 100 字符，去空白和重复 |
| 偏好 | work_modes、employment_types | 枚举数组；remote/hybrid/onsite；full_time/part_time/contract/internship |
| 薪资 | salary_min、salary_max、salary_currency、salary_period | 可选非负 decimal 字符串，min≤max；填写金额时币种和 year/month/hour 必填；不做跨币种比较 |
| 教育 | institution、qualification、field_of_study、start_month、end_month、is_current、notes | institution 必填；月份 YYYY-MM，结束不早于开始；is_current=true 时 end_month=null；notes≤2000 |
| 工作 | company、role、start_month、end_month、is_current、description、skill_ids | company/role 必填，文本≤4000；最多 30 个技能引用 |
| 项目 | name、role、description、project_url、skill_ids | name 必填；description≤4000；HTTPS 链接；最多 30 个技能引用 |
| 技能 | skill_id、proficiency、notes | proficiency 为 new/learning/working/proficient；notes≤1000 |
| 简历 | file_id、version_label、is_default | 对接 resume/attachment；核心支持 PDF/DOCX 解析和事实确认，保存独立 revision；不暴露存储路径 |

education/work/project 每类最多 50 条；列表按稳定 sort_order、ID 排序。这里的长度以字符计，HTTP 总 body 另限 128 KiB。数据库类型与后端校验共同约束，前端校验只改善体验。

技能未出现在 user_skills 中就是 not_assessed；移除表示撤销自评，不等于不会。技术熟练度与面试信心继续分开，学历或工作年限不自动转换成技能等级。

## 3. 页面：AI 服务配置 `/settings/ai`

```text
AI 服务
  使用方式       ( ) 我的 API Key   ( ) 平台提供（仅部署者开启时显示）
  服务商         [支持的服务商 ▼]
  模型           [该服务商支持的模型 ▼]
  API Key        [输入密钥，仅本次输入可显示/隐藏]
                 保存后不会再次显示完整密钥
  每日调用限制   [20]（示例，可在平台上限内调整）
  当前状态       未测试 / 测试中 / 可用 / 认证失败 / 暂时不可用
                 [保存] [测试连接] [启用此配置]
  已保存         Key 已配置 · 更新于……      [替换 Key] [删除配置]
```

保存与启用分开：首次保存为 disabled+untested；更换服务商、Key 或模型产生新 revision，自动 disabled+untested。测试成功后仍由用户点击启用。仅修改每日限额不使凭据测试失效。

Key 字段保存后清空，页面只显示 has_key=true 与固定掩码，不返回实际前缀/后缀。编辑模型时不需要重新输入已保存的 Key；更换服务商必须提供新 Key。Key 输入不写 localStorage、sessionStorage、URL、Query cache、错误追踪或表单录屏。

测试发送一条固定短提示，**不发送个人资料或 JD，可能产生少量服务商费用**，按钮附近明确说明。一次最多测试一个配置，设置响应/时间/token 上限，不自动循环重试。测试通过只证明当时的调用成功，不保证以后有额度或服务可用。

限额以调用次数和 token 为主：个人每日调用限制覆盖提取、生成与修改的所有外部尝试，测试另有低频限额。费用估算只在有已知价格时展示，不能充当服务商账单；用户在其他软件使用同一个 Key 的消耗不受本系统限制。

## 4. 个人资料 API

所有 `/api/me/*` 仅从认证身份取得 owner，禁止 body.user_id。写操作使用 CSRF 与 `expected_version`；版本冲突返回 409 和安全的 current_version。

| 方法与路径 | 请求 | 响应 |
|---|---|---|
| GET /api/me/profile | 无 | 基础资料、偏好、profile_version、分区完整情况 |
| PATCH /api/me/profile | expected_version + basics 和/或 preferences 的变更 | 200，更新后的基础资料/偏好和 profile_version |
| GET /api/me/education | 无 | items + profile_version |
| POST /api/me/education | expected_version + item | 201，item + profile_version |
| PATCH /api/me/education/{id} | expected_version + item 的变更 | 200，item + profile_version |
| DELETE /api/me/education/{id} | If-Match: "profile-N" | 200，profile_version |
| GET/POST /api/me/work-experiences | 同上 | 列表/创建 |
| PATCH/DELETE /api/me/work-experiences/{id} | 同上 | 更新/删除 |
| GET/POST /api/me/projects | 同上 | 列表/创建 |
| PATCH/DELETE /api/me/projects/{id} | 同上 | 更新/删除 |
| GET /api/me/skills | 无 | items + profile_version |
| PUT /api/me/skills/{skill_id} | expected_version + proficiency + notes | 200，item + profile_version |
| DELETE /api/me/skills/{skill_id} | If-Match: "profile-N" | 200，profile_version |

DELETE 使用 If-Match 避免依赖 DELETE body；其余写接口用 expected_version。两者含义都是比较同一个 profile_version，不是各资源独立版本。所有分区写操作锁 users 行并递增该版本；同用户同时编辑不同区可能冲突，首版接受这一取舍。

PATCH 中省略字段表示不修改，nullable 字段显式 null 表示清空，数组表示整体替换。拒绝未知字段，basic/preferences 不能借此修改技能或经历列表。GET 聚合读取使用单 SQL 或一致性读取事务，确保数据与返回版本对应。

```json
{
  "expected_version": 7,
  "basics": {
    "display_name": "Alex",
    "city": "Sydney",
    "contact_email": "alex@example.com"
  },
  "preferences": {
    "target_roles": ["Backend Engineer"],
    "work_modes": ["hybrid", "remote"]
  }
}
```

新账号 profile_version 从 1 开始，GET 返回空字段/空数组，不要求先创建 profile。资料更新不改写历史分析快照；分析结果展示“基于资料版本 N”，后续用户可重新分析。

## 5. AI 配置 API

| 方法与路径 | 用途 |
|---|---|
| GET /api/ai/providers | 已认证用户获取部署支持的服务商、模型、默认值、限制；不代理任意 URL |
| GET /api/me/ai-config | 只读安全配置、状态、version、revision、平台模式是否可用 |
| PUT /api/me/ai-config | 保存/替换当前配置，expected_version 防覆盖 |
| POST /api/me/ai-config/test | 对指定 revision 的已保存配置发起有界连接测试 |
| PATCH /api/me/ai-config | 切换 mode、enabled 或 daily_request_limit；启用需当前 revision 测试通过 |
| DELETE /api/me/ai-config | If-Match: "ai-N"，撤销个人凭据，返回新的 version |

PUT 示例（provider_id/model_id 为服务端目录返回值，这里的值仅是示例）：

```json
{
  "expected_version": 0,
  "provider_id": "provider_a",
  "model_id": "model_a",
  "api_key": "<user-entered-secret>",
  "daily_request_limit": 20
}
```

已有配置时 api_key 省略代表保留；空串/null 非法，不能拿掩码当密钥提交。PUT 是明确的“保存配置”操作而非通用 JSON merge；配置参数必填、secret 是 write-only 特例。保存不自动发起外部请求。

GET/保存响应示例：

```json
{
  "version": 1,
  "mode": "personal",
  "enabled": false,
  "revision": 1,
  "provider_id": "provider_a",
  "model_id": "model_a",
  "has_key": true,
  "test_status": "untested",
  "last_tested_at": null,
  "daily_request_limit": 20,
  "platform_available": false
}
```

首次 GET 返回 version=0、revision=null、has_key=false、enabled=false。删除后保留无 secret 的 tombstone/version，版本不重置为 0；重建使用新版本，避免旧页面覆盖新配置。默认 mode=personal，即使部署者有平台 Key 也不自动选择。

测试请求为 `{expected_version, revision}`，受理后短事务设置 test_status=testing 与 test_run_id，再在事务外调用 provider，最多 8 秒，结束后仅以匹配的 revision+test_run_id 条件写回。测试请求需要 Idempotency-Key，避免网络重试重复测试计费。重复请求不再次调用：执行中返回 202，客户端轮询 GET；首次同步完成返回 200。测试结果缓存按用户/key/body 保存至少 24h，不保存密钥。进程崩溃时 testing 通过 test_deadline 超时转 inconclusive，用户明确再次点击才重试。

PATCH 示例：`{expected_version: 2, mode: "personal", enabled: true}`。所有可见测试状态/启用/限额变更均提升 config version；revision 仅在凭据/模型变更时提升。迟到测试不得把已删除或更新的配置标记为可用。

DELETE 撤销当前及仍保留的旧 personal revisions，并禁用配置；返回 200 `{version, enabled:false}`。删除只移除本系统副本，不替用户去服务商撤销 Key，UI 说明仍可到服务商控制台撤销。

错误沿用统一 problem：400 validation_error；401 会话过期；404 不存在/他人资源；409 config_version_conflict/config_test_required/config_changed；429 本系统测试/用量限制；503 基础设施故障。外部测试认证失败、模型不可用、额度不足作为 200 的 test_status=failed + 安全 error_code 返回，**不借用 401 让用户误退出本系统**。

## 6. 数据模型与版本

| 表/变更 | 关键字段与约束 |
|---|---|
| user_profiles | user_id PK/FK、display_name、contact_email、phone、city、country_code、summary、三个外部 URL、timestamps |
| user_preferences | user_id PK/FK、岗位/地点/模式/类型数组、salary_min/max numeric、currency、period |
| user_education | id、owner_id、学校/学历/专业/月度日期/在读标识/notes/sort_order |
| user_work_experiences | id、owner_id、公司/角色/日期/description/sort_order |
| user_projects | id、owner_id、name/role/description/url/sort_order |
| work_experience_skills / project_skills | owner_id、parent_id、skill_id，复合 owner FK；唯一 parent/skill |
| user_skills | 复用原表，proficiency 与 notes；禁止用面试 confidence 代替 |
| user_ai_settings | user_id PK、version、mode、enabled、current_revision_id、daily_request_limit、timestamps；无 secret |
| user_ai_config_revisions | id、owner_id、revision、provider_id、model_id、secret_ref、test_status/test_run_id/test_deadline/tested_at、revoked_at；唯一 owner/revision |
| ai_credentials | id、owner_id、ciphertext、nonce、key_version、revoked_at；不进入通用资料查询 |
| ai_test_requests | owner_id、key_hash、request_hash、revision_id、run_id、state、safe_result、expires_at；唯一 owner/key_hash |
| job_tasks / generation_runs | ai_source、ai_revision_id、credential_id、provider_id/model_id、提交时 AI setting version；仅引用，不复制 secret；本地导出 task 无 AI 凭据 |
| ai_usage_ledger | owner_id、task_id/test_run_id、attempt、reserved/actual tokens、日期桶、结算状态，事件唯一约束 |

users.profile_version 是个人资料统一版本；AI 使用独立 settings version。锁顺序为 `user → ai_settings → workspace → application → documents（ID 排序） → job_tasks（ID 排序） → outbox`，所有涉及双方的事务遵守此顺序。凭据读取/轮换不反向锁 user。

## 7. 密钥保护与服务商连接

用户 Key 必须能被 Worker 解密使用，因此不能像密码那样只存 hash。采用认证加密/信封加密，主密钥从部署 secret/KMS 取得，数据库保存 ciphertext、nonce、key version；AAD 绑定 owner_id 与 credential_id。主密钥不能存到同一数据库或代码库。轮换逐条重加密并保留可恢复路径，备份密文与恢复密钥分别管理。[OWASP Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)

API 只在保存时接受明文；测试执行与 Worker 在调用前短暂解密。常规 GET、队列、任务快照、trace、审计、导出与错误响应都不含密钥。审计仅记录用户、操作、revision、结果和 request ID。禁止记录该接口请求体，前端关闭 Key 字段的 session replay。

服务商/模型目录由后端维护，provider_id 映射固定 HTTPS origin 和 adapter，拒绝未知模型、用户 headers、任意 endpoint 和重定向。未来若开放自定义地址，须另行加入域名/IP allowlist、DNS 与连接时校验、私网/metadata 地址阻断及出口限制；首版直接拒绝 base_url 字段。[OWASP SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)

## 8. 核心任务集成

工作区接收和两项文档任务的完整事务见 [Studio 设计](application-studio.md#4-原子性并发与版本)。现有分析例子改为共用 job_tasks 运行时；不是增加第二套执行状态。

提交请求新增 `expected_ai_config_version`，客户端从 GET ai-config 获取。SubmitInput 与 request hash 都加入该字段；相同 key 的重放仍先于来源版本校验，不受后来配置更新影响。

接受事务在 user 锁后读取 ai_settings：

1. 重放已存在的幂等结果；新请求检查 workspace、已确认 JD/resume revisions、profile、AI config 版本。
2. personal 模式要求 enabled、当前 revision 测试通过且凭据未撤销；platform 模式要求部署开关与用户明确选择。
3. 原子预留两份文档用量、保存已确认 JD/简历及选定资料事实快照、记录 ai_source/revision/credential 引用、provider/model 与 prompt/schema/数据范围版本。
4. 任务和 outbox 一起提交。队列 payload 不带 Key，API handler 不调用模型。

Worker 根据任务 owner + 固定 revision 取得凭据，调用前及重试前检查 revoked；provider adapter 收到运行时凭据，不使用全局可变“当前用户 Key”。不同操作使用不同数据范围：JD 提取仅发送本次 JD/截图；简历解析若使用外部模型，发送所选文件必要内容；文档生成发送已确认 JD、基础简历事实和选定相关教育/经历/项目；文档修改再含固定版本与修改指令。联系方式及姓名落款默认本地合并，薪资等无关资料不发送。UI 在调用前显示服务商与范围，技能自评不由模型推断。详见 [传输范围](application-studio.md#6-provider-与资料范围)。

配置改变后的任务语义：

| 操作 | 已接受任务 | 新任务 |
|---|---|---|
| 修改模型/替换 Key | 继续使用已锁定旧 revision；旧密文仅保留至引用任务终止及清理完成 | 使用新 revision，测试通过并启用后接受 |
| 普通禁用配置/切模式 | 已接受任务仍按原来源执行 | disabled 时不接受；切模式后显式使用新来源 |
| 删除个人配置 | 所有 personal revisions 撤销，queued/retry_wait 取消，running 设置取消意图 | personal 模式禁止；不会自动用平台 Key |
| 服务商 401/403 | 标记安全错误并停止重试，禁用对应 revision 的后续使用 | 提示修复 Key |

删除 UI 明示“停止使用已保存的 Key，并取消相关未完成任务”。正在发出的外部调用无法保证撤回；取消前后按数据库已有终态规则处理。Worker 续租检查 credential 撤销，结果提交也检查取消/fence，禁止取消后再接受结果。已完成分析结果保留，但不保留密钥。

配置版本更新、凭据撤销、相关任务取消 intent 在短事务内一致提交；解密和 provider 调用在事务外。平台模式使用部署 secret reference，也锁定 provider/model/policy 版本；个人模式从不回退到平台模式。

## 9. 文件落点

```text
backend/internal/profile/
  model.go / ports.go / service.go / validation.go
backend/internal/aisettings/
  model.go           # Settings、Revision、SafeView，无明文 JSON 输出
  ports.go           # Store、SecretVault、ConnectionTester
  save.go            # 版本检查、创建 immutable revision
  test.go            # 测试幂等与 conditional writeback
  activate.go        # mode/enable/limit 更新
  revoke.go          # 撤销 + 任务取消事务
backend/internal/adapters/postgres/
  profile_store.go / ai_settings_store.go / ai_usage_store.go
backend/internal/adapters/security/
  credential_vault.go # 认证加密、AAD、key version
backend/internal/transport/http/
  profile/{handler,dto}.go
  ai_settings/{handler,dto}.go
frontend/src/features/profile/
  api.ts / queries.ts / mutations.ts / form-schema.ts
  BasicsForm.tsx / PreferencesForm.tsx / SkillsEditor.tsx
  EducationEditor.tsx / WorkExperienceEditor.tsx / ProjectsEditor.tsx
frontend/src/features/ai-settings/
  api.ts / queries.ts / mutations.ts / form-schema.ts
  AISettingsForm.tsx / ConnectionTestStatus.tsx / DeleteConfigDialog.tsx
frontend/src/pages/
  ProfilePage.tsx / AISettingsPage.tsx
```

前端 query key 含 owner；Key 仅存输入组件的短期 state，提交成功/离开页面即清空。资料版本冲突保留本地草稿、刷新服务器版本供用户比较，不自动覆盖。

## 10. 验收与实施顺序

核心阶段先提供可确认的基础简历与必要资料，再完成个人 Key 保存/测试/启用和任务集成。用户无需先完成全部手工分区；PDF/DOCX 解析已移到核心范围。完整资料编辑逐步扩展，面试与统计后置。原 tracker 工期估算不再适用，按新任务拆分后估算。

必测：其他账号不能读取/修改配置或资料；响应与日志无 Key；错误主密钥拒绝解密；相同测试 key 不重复计费调用；迟到测试不能激活旧 revision；多标签页 profile/config 冲突；资料不全仍可保存职位；不同用户 Key 不串用；配置变更不改变旧任务；撤销与执行竞争；平台不自动兜底；外部 401 不注销本站会话；无效/自定义 endpoint 被拒绝；资料版本与快照一致；移除技能变为未评估；备份/导出不泄露明文 Key。

本文是上述两个模块的协议细化；第一批基础资料/技能的 OpenAPI 已在 `api/openapi.yaml` 落地，本文其他接口仍待后续批次；目前没有可运行 HTTP 接口。
