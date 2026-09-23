# Light Glass Studio — tokens and material

V3 已确认的起点；完整页面流程在仓库 `docs/design/DESIGN.md`。参数可随真实渲染微调，保持浅色混光、无蓝色主调、中性透色玻璃和清晰深色文字。

## Theme tokens

```css
:root {
  color-scheme: light;
  --af-font: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  --af-bg: #f3eee6;
  --af-light-peach: #f4c6ad;
  --af-light-rose: #e9c6cc;
  --af-light-sage: #ced8c0;
  --af-light-champagne: #f6e5bc;
  --af-surface: #faf8f3;
  --af-glass: rgb(255 255 255 / 48%);
  --af-glass-reading: rgb(255 255 255 / 74%);
  --af-glass-menu: rgb(255 255 255 / 94%);
  --af-glass-border: rgb(255 255 255 / 78%);
  --af-border: rgb(69 66 48 / 16%);
  --af-control-border: #777568;
  --af-text: #25261f;
  --af-text-secondary: #505247;
  --af-text-muted: #595b50;
  --af-accent: #515b36;
  --af-on-accent: #ffffff;
  --af-selection: #e1e7d3;
  --af-focus: #46532c;
  --af-danger: #9d3025;
  --af-success: #2f6546;
  --af-warning: #80501e;
  --af-radius-control: 14px;
  --af-radius-panel: 28px;
  --af-radius-pill: 999px;
  --af-shadow: 0 16px 48px rgb(65 51 30 / 10%);
  --af-highlight: inset 0 1px 0 rgb(255 255 255 / 92%);
  --af-duration: 160ms;
  --af-easing: cubic-bezier(.2, 0, 0, 1);
  --af-space-1: 4px;
  --af-space-2: 8px;
  --af-space-3: 12px;
  --af-space-4: 16px;
  --af-space-6: 24px;
  --af-space-8: 32px;
  --af-space-12: 48px;
}
```

## Glass and background

```css
.af-shell {
  min-height: 100dvh;
  font-family: var(--af-font);
  color: var(--af-text);
  background:
    radial-gradient(ellipse at 16% 24%, var(--af-light-peach), transparent 62%),
    radial-gradient(ellipse at 80% 10%, var(--af-light-rose), transparent 58%),
    radial-gradient(ellipse at 82% 86%, var(--af-light-sage), transparent 62%),
    radial-gradient(ellipse at 36% 95%, var(--af-light-champagne), transparent 55%),
    var(--af-bg);
}

/* Solid fallback; only shared surfaces own the backdrop blur. */
.af-glass {
  background: var(--af-surface);
  border: 1px solid var(--af-border);
  border-radius: var(--af-radius-panel);
  box-shadow: var(--af-highlight), var(--af-shadow);
}
.af-glass-reading, .af-glass-menu { background: var(--af-surface); }

@supports ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .af-glass {
    background: var(--af-glass);
    border-color: var(--af-glass-border);
    backdrop-filter: blur(24px) saturate(135%);
    -webkit-backdrop-filter: blur(24px) saturate(135%);
  }
  .af-glass-reading { background: var(--af-glass-reading); }
  .af-glass-menu { background: var(--af-glass-menu); }
}

.af-primary {
  background: var(--af-accent);
  color: var(--af-on-accent);
  border-radius: var(--af-radius-pill);
  min-height: 44px;
}
.af-control { border: 1px solid var(--af-control-border); }
:where(.af-shell, .af-portal) :focus-visible {
  outline: 2px solid var(--af-focus);
  outline-offset: 3px;
}

@media (prefers-reduced-transparency: reduce) {
  .af-glass, .af-glass-reading, .af-glass-menu {
    background: var(--af-surface);
    border-color: var(--af-border);
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  :where(.af-shell, .af-portal) *,
  :where(.af-shell, .af-portal) *::before,
  :where(.af-shell, .af-portal) *::after {
    animation: none !important;
    transition: none !important;
    scroll-behavior: auto !important;
  }
}
```

阅读区只增加白色填充，通常不再加一层 backdrop-filter。菜单同时用 af-glass 与 af-glass-menu。装饰细边不能作为输入框唯一识别手段；不把 secondary 文本整体再降 opacity。真实背景合成和用户选择态需要浏览器检查，token 数值本身不是 AA 认证。

## Layout and typography

- 顶栏约 64–72px，高度随内容响应；无固定左侧栏。桌面外边距 32–64px，手机 16–20px。
- 输入工作面 max-width 960px，文档工作区 max-width 1280px。大屏预览/修改栏约 2:1，小屏单列；避免正文被固定按钮盖住。
- Create 标题约 36–48px，手机 28–34px；工作区标题 28–36px；正文/表单 16px，辅助文字 14px；长文行高 1.6–1.75。
- 面板 padding 28–40px，手机 20px；文档每行约 60–80 字符。长文允许自然滚动，不强制挤进一屏。
- 控件至少 44px；按钮可胶囊，textarea 保持圆角矩形。错误紧邻字段，选中/状态不能只靠颜色表达。
- 背景默认不动。局部按压 scale(.98)、菜单小幅淡入即可。不要让页面像会晃动的展示卡。

## Export is a document, not a screenshot

PDF/DOCX 不复用玻璃 CSS、按钮或背景图。语义化标题/段落/列表通过独立模板生成；默认白底深字、正常页边距、可选中文字。检验长段落、分页、字体回退、链接和内容完整性。
