# ApplyFlow — approved interface design

状态：2026-09-21 用户确认 V3 为正式设计方向；2026-09-23 英文玻璃前端已接通文本 JD / Mock / 编辑 / 导出，2026-09-27 个人 AI 设置表单已接通，2026-09-28 已接通真实生成模式；其他目标状态仍待实现。本文定义产品界面，[视觉参数](../../skills/applyflow-light-glass/references/visual-system.md)定义 tokens，[工作流架构](../architecture/application-studio.md)定义数据与后端边界。

## 产品中心

默认英文。首页帮助用户把 **JD 截图、文字或链接 + 自己的基础简历 → 针对职位的 Resume 与 Cover Letter → 审阅、修改、导出**。申请管理为辅助入口，面试准备和统计后置。

用户不需要先填写长篇职位表单或全部个人资料。第一次使用上传简历并确认提取内容；以后复用已确认的基础简历。公司与岗位由 LLM 从完整 JD 理解，不要求用户另填。账户菜单提供 Profile、My resumes、AI settings。

## 已确认的视觉参考

| 页面 | 原型 | 关键行为 |
|---|---|---|
| Create | [输入页](prototypes/v3/01-job-intake.png) | 一个区域接受粘贴文字/链接、上传/拖放/粘贴截图 |
| Resume | [简历工作区](prototypes/v3/02-tailored-resume.png) | 文档预览、直接编辑、改动说明、自然语言修改 |
| Cover letter | [求职信工作区](prototypes/v3/03-cover-letter.png) | 文档预览、语气、长度、补充内容和导出 |

V1/V2 仅保留为历史，不作为实现依据。V3 的材质、色彩、信息层级已确认；生成图片中的室内物体、标志差异、较暗边角和小字不作为像素级规范。实现使用统一 ApplyFlow 字标及抽象混色光场，不需要照片、家具或装饰文字。

## 视觉语言

- 浅杏桃、柔粉、香槟、鼠尾草绿形成大面积低饱和扩散混光，无蓝色主调、无全黑背景。
- 玻璃本身为中性白色半透明；背景的颜色透过模糊进入面板。细白边、上沿高光、柔和阴影表现厚度，避免霓虹边框。
- 深色文字像写在玻璃上。正文阅读区提高白色填充，背景细节不得干扰编辑与选择文字。
- 一个主玻璃工作面，最多两层有效模糊；用留白和分隔线分区，避免卡片套卡片。
- 顶部轻量导航 Create / My applications + 账户菜单，无固定后台侧栏或统计卡墙。
- 主操作用深橄榄实色，次操作用轻玻璃。控件高度至少 44px；文字和必要边界须在实际合成背景上满足对比要求。
- 默认静态混光，局部按压/淡入即可；不引入 Three.js 作为材质前提。支持减少动态效果、减少透明度及无 backdrop-filter 降级。

## 页面和路由

| 路由 | 页面职责 |
|---|---|
| `/` | 登录后转 `/create` |
| `/login`, `/register` | 简单认证，恢复返回位置 |
| `/create` | JD 输入、基础简历选择、AI 配置状态 |
| `/create/:workspaceId` | 恢复已保存输入；提取与事实确认、失败重试 |
| `/studio/:workspaceId?document=resume` | 两份文档标签工作区；参数亦可为 `cover_letter` |
| `/applications`, `/applications/:id` | 已保存申请列表、进度与关联文档版本 |
| `/profile`, `/resumes`, `/settings/ai` | 账户菜单下的个人资料、基础简历和 AI 配置 |

旧设计中的 `/dashboard`、`/applications/new` 不再建设为独立页面；若以后需要兼容已有链接，重定向 `/create`。文档正文、API Key、JD 与简历内容不能进入 URL。

## 核心交互

1. **Create**：输入区自动辨识纯链接与 JD 文本；图片支持文件选择、拖放和剪贴板粘贴。显示文件名、预览、替换/移除操作，不上传剪贴板里用户未提交的其他内容。
2. **直接生成**：只需完整 JD 与已确认简历，一次点击 Generate 提交不可变快照并生成。不显示独立确认步骤或公司/岗位补填框。公司与岗位元数据允许为空，LLM 在原有两次文档生成调用中理解完整 JD；没有写明时使用中性措辞，不编造、不增加识别调用。
3. **基础简历**：支持 PDF/DOCX 上传，提取后用户确认；无可用文字时允许手工补充，禁止生成虚构工作经历。用户可以用简历完成首次流程，不必填完整 Profile。
4. **生成**：两份文档独立显示真实阶段。一份失败时仍可编辑和导出另一份，仅重试失败项。不要用假百分比、假已保存或假已连接状态。
5. **审阅**：Resume / Cover letter 切换保留各自草稿。What changed 展示来源与改动；Needs your input 提问而非补造技能、任职日期、业绩。直接编辑可撤销，AI 修改产生候选版本，由用户应用。
6. **导出**：导出用户已保存的指定版本为 PDF 或 DOCX，按钮等待未保存内容提交成功；导出制品包含清晰文本和可用分页，不把 UI 玻璃背景印入文件。导出不代表投递。
7. **跟踪**：Save to my applications 将工作区关联到申请记录并固定所选材料版本；重复操作返回同一条申请，不重复创建。后续换版本需明确操作。

## 默认文案与状态

| 情境 | 默认英文与动作 |
|---|---|
| JD 输入 | Paste a job description or job link… / Or drop a screenshot here |
| 未选简历 | Upload your resume / Choose an existing resume |
| AI 未配置 | Connect your AI provider；返回后恢复已保存输入 |
| 链接不可读 | We couldn't read this link. Paste the text or upload a screenshot. |
| OCR 不确定 | Check the highlighted text before continuing. |
| 缺失经历 | Add this only if it reflects your experience. |
| 保存冲突 | A newer version is available. Compare changes.；保留本地草稿 |
| 连接断开 | Reconnecting. Your drafts are still being prepared. |
| 部分失败 | Your resume is ready. Retry the cover letter. |

文案集中维护在英文资源文件，错误逻辑使用稳定 code。界面语言和生成材料语言是两个概念，第一版均默认英文，后续扩展不能根据浏览器语言静默改变。

## 响应式与验收

桌面约 1440px：输入面板约 900–960px；工作区最多 1280px，预览与修改栏约 2:1。窄屏约 390px：单列文档，修改选项作为可展开区域；主要操作不覆盖正文或软键盘，不靠双栏横向滚动。账户菜单支持键盘、焦点恢复和清晰选中态。

实现验收须覆盖首次使用、三种 JD 输入、提取确认、AI 配置返回、直接编辑、版本冲突、部分成功、重试、刷新恢复、导出和关联申请。验证真实混光背景上的正文/焦点对比、减少透明度、长职位名、长文分页、键盘与屏幕阅读器状态提示。静态原型不代表这些已通过。

## AI settings 已实现交互

`/settings/ai` 使用一个玻璃工作面：Save / Test / Enable 三步说明，服务商和模型、一次性 Key 输入以及折叠的 Usage preferences。主操作随状态变化；Key 保存成功清空，不显示已存密钥的前后缀。测试费用说明紧邻测试按钮，启用后引导用户在 Create 显式选择 My AI connection。删除使用就地确认，未知测试结果只允许复查相同测试；冲突保留编辑并引导读取最新配置。

### AI 服务商选择补充（0.8.0）

沿用单一玻璃表单：Provider 下拉显示 OpenAI 与 aiwanwu，Model 跟随服务商变化。显示固定请求地址；选择中转时说明 Key、测试提示及生成所用的 JD/事实会发送给该第三方。切换平台清空输入 Key，旧 Key 不自动复用，保存后重新测试并明确启用。Create 的调用说明显示实际服务商，不统一称为 OpenAI。

### 完整简历文本与预览（0.9.0）

上传文本型 PDF/DOCX 后自动读取全部文本，按来源块供用户一次检查并确认；不再默认要求手动重写经历。扫描页或读取失败明确说明并提供手工补录，不悄悄丢页。Studio 默认呈现纸张式正文预览，Edit document 切换至表单编辑，切换预览不丢弃未保存修改；导出仍固定已保存 revision。当前 PDF 使用独立简洁排版模板，不承诺复刻原 PDF 设计。

生成前从所选简历保存的原文件重新提取正文，不能把旧版手填摘要当作完整 CV。若当前 revision 缺少原文件内容，创建新 revision，保留手填补充后用于新生成；完整正文已存在时复用 revision。提取失败必须停止，不退回短摘要。原文件、旧 revision 与历史生成不修改，不自动付费重试。
