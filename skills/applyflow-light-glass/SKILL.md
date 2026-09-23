---
name: applyflow-light-glass
description: Design, implement, or review ApplyFlow's English-first AI resume and cover-letter workflow using light pastel diffusion and frosted glass. Use for ApplyFlow frontend layouts, components, styling and interaction states; not backend-only or unrelated design work.
---

# ApplyFlow — Light Glass Studio

## 已确认方向

用户已确认 V3：**JD 截图、文字或链接 + 基础简历 → AI 定制 Resume 与 Cover Letter → 审阅修改 → 导出**。默认英文，申请跟踪为辅助功能，面试与统计后置。浅杏桃、柔粉、香槟与鼠尾草绿混光透过中性磨砂玻璃，深色文字像写在玻璃上。保留简洁工作流和实验性的材质，不恢复旧版白灰蓝后台或全黑方案。

用户后续要求优先于本 skill。无需重复询问已确认的配色和页面语言。适用于设计/实现/审查；仅请求原型或文档时不要顺带创建整站。

## 工作依据

- 在 ApplyFlow 仓库读取 `docs/design/DESIGN.md`（正式页面/状态）、`docs/architecture/application-studio.md`（版本/任务/数据边界）与相关已有代码。V3 三张图片为材质和构图参考，V1/V2 是历史。
- 写样式时读取 [visual-system.md](references/visual-system.md)，把 tokens 纳入单一主题入口，不复制到各页面。
- 沿用 React + TypeScript + Vite；pages → features → shared。核心 feature 是 intake/resumes/studio/documents/tasks；不把请求、编辑缓冲、SSE 与长 JSX 塞入一个页面文件。
- OpenAPI 仅代表已定义契约，不代表 handler 已实现。新功能先明确协议，不能把静态原型按钮实现成假成功。

## 视觉与交互

- 玻璃填充始终是中性白色半透明。色彩来自背后扩散光场；细白边、上沿高光、柔和阴影表现厚度。没有蓝色主调、霓虹边框、全黑底。
- 使用抽象混色背景，不复制生成图里的家具、植物、装饰文字或重暗角。一个主工作面、最多两层有效模糊；长文区提高白色填充但保留透色。
- 顶栏 Create / My applications + 账户入口；无固定后台侧栏、统计卡墙。一个主要操作随当前步骤变化。
- 默认英文文案集中维护；生成材料也默认英文，但不将界面语言与文档语言混为一项设置。
- JD 入口合并文字/链接/截图；图片支持上传、拖放和粘贴。首次上传简历，以后复用。只逐步询问缺失信息，不强迫先填写完整 profile。
- Resume / Cover letter 是同一工作区的两个标签；桌面文档预览 + 窄修改栏，手机单列 + 可展开修改区。正文深色、可选择和编辑，输入边界/焦点清楚。
- 主按钮深橄榄实色，次操作轻玻璃；线性 SVG 图标，不用 emoji 或装饰图标盒。非交互面板不 hover 上浮。默认不加载外部字体。
- 默认静态背景，局部 120–180ms 的 opacity/transform 动效即可。无须引入 Three.js；正文不等待动画出现。

## 不能被视觉简化掉的产品语义

- 显示解析确认：公司、岗位、JD、OCR 疑点和基础简历事实。链接不可读保留输入，提供文字/截图替代。
- AI 基于已确认事实改写；未证实的技能/业绩/日期提示用户，不能编造。技术自评缺失不代表不会。
- 两份材料独立保存与重试；半成功仍可使用完成的一份。生成进度显示真实阶段，不能造百分比。
- 基础简历不改写；AI 修改生成候选 revision，不覆盖人工编辑。保存冲突保留本地缓冲并提供对比。
- 导出固定已保存 revision；未保存先完成保存。导出文件使用清晰文档模板，不带玻璃背景。导出不等于投递或 Applied。
- 工作区草稿独立保存；Save to my applications 幂等关联跟踪记录，不重复创建。
- Profile / My resumes / AI settings 在账户菜单可发现；AI 未连接明确引导，返回恢复输入。Key 仅短期输入 state，不入 URL、cache、storage、日志或预览。
- 调用前说明服务商及发送的数据类别。默认本地合并联系方式；“已保存”“已连接”只能来自真实结果。

## 验收

检查桌面约 1440px 与窄屏约 390px、长职位名、长文、软键盘、键盘焦点、标签切换编辑保留、版本冲突、半成功、失败重试、刷新恢复与导出。普通文字在实际合成背景上至少 4.5:1，必要边界/焦点至少 3:1；支持无 backdrop-filter、减少透明度与减少动态效果。Portal 浮层也应继承主题及降级规则。

使用已有 lint/typecheck/build 与必要行为测试；纯样式不新增复述 CSS 的测试。能渲染则视觉检查，未运行浏览器或未实现功能时明确说明，不能用原型验证代替实际页面验收。
