# Frontend — Light Glass Studio

2026-09-23：React + TypeScript + Vite 前端已实现，并接通现有 Go API/Worker。它使用真实账户和私有文件；文档生成仍是明确标注的 Mock，并非真实 AI。

## 本地启动

需要 Node.js 22.13+、现有 Go/Python/PostgreSQL 环境。先按 [API 说明](local-api.md)设置数据库、随机会话密钥、私有文件目录，再调整 API 的 Origin 与浏览器一致：

```sh
export PUBLIC_ORIGIN=http://127.0.0.1:5173
export HTTP_ADDR=127.0.0.1:8080
export FILE_STORAGE_DIR="$PWD/var/private-files"
make setup-tools
make migrate-up
make run-api
```

另一个终端设置相同 DATABASE_URL、FILE_STORAGE_DIR，运行 `make run-worker`。第三个终端：

```sh
npm ci --prefix frontend
make run-frontend
```

访问 http://127.0.0.1:5173/register。请使用同一个 hostname；不要混用 localhost 与 127.0.0.1。Vite 将 `/api` 转发到 127.0.0.1:8080，保留 Origin，浏览器只访问同源。没有启动 Worker 时，任务会保持排队状态，界面不伪造进度。

## 可用流程

注册/登录 → 粘贴文本 JD → 上传 PDF/DOCX → 手动确认真实简历事实 → 确认公司/岗位 → Generate sample drafts → 切换 Resume / Cover letter → 编辑/保存 → PDF / Word 导出 → 私有下载。

已确认的简历可以复用。创建入口显示最近 5 个工作区；工作区 URL 可直接打开，刷新从数据库恢复已保存的来源、确认信息与文档。未保存的文本只在内存，离开时有浏览器提醒，不承诺恢复未提交输入。账户菜单可进入 My resumes。Profile、AI settings 和 My applications 清楚说明当前未接通；不收集不能保存的 API Key。

截图/链接解析、自动简历提取、真实 AI、自然语言改写、完整个人资料和申请跟踪仍待后端实现。截图拖放/粘贴与纯链接会提示改用文本，不能提交为假成功的截图/链接任务。

## 代码边界

- `src/app/App.tsx`：会话边界和路由分派；`AppShell.tsx`：顶部导航/账户菜单。
- `src/pages`：页面组合；业务流程在 `features/studio/use-create-workflow.ts` 与 `api.ts`。
- `features/resumes`：私有上传、事实确认、已确认版本选择。
- `features/documents`：结构化编辑、独立编辑缓冲、保存冲突对比、历史版本与固定版本导出。
- `features/tasks`：真实任务状态、取消和独立重试；`shared/api/use-resource.ts` 负责有界间隔轮询、卸载取消和旧响应隔离。
- `shared/api/client.ts`：同源 Cookie、CSRF、错误解码和内存幂等命令；不自动重试写操作。
- `shared/api/generated.ts`：通过 `make generate-frontend-types` 从 OpenAPI schema 生成；只是静态类型，JSON Schema 的条件校验仍由服务端执行。
- `shared/i18n/en.ts`：公共文案及稳定错误码的英文映射；主题 token、玻璃材质和布局集中在 `app/theme`。

当前使用原生链接和浏览器 history，跨页面重新加载；文档标签在同一页面切换，两份编辑器保持挂载。此小规模实现不引入 Router/Query 框架或持久化查询缓存。后续引入客户端路由时须保留离开保护和账户切换清理语义。与目标架构相比，语言资源放在 shared 层，避免 features/shared 反向依赖 app。

API Key、JD、简历正文和认证信息不进入 URL、localStorage 或 sessionStorage。URL 只含工作区 UUID 和文档标签。导出先等待保存成功，冲突时保留本地编辑并允许对比；不会自动覆盖最新版本。用户选择历史候选前必须先保存本地编辑。

## 验证

```sh
make check-frontend
```

包含格式检查、请求安全行为测试、TypeScript 类型检查和生产构建。CI 已加入该检查，但本次会话未运行远端 CI。

真实浏览器流程只对临时开发栈运行，会创建合成账户。准备合成 PDF/DOCX，安装 Playwright Chromium 后：

```sh
cd frontend
npx playwright install chromium
BASE_RESUME=/absolute/path/to/synthetic-resume.pdf npm run test:browser
```

可用 CHROMIUM_PATH 指向已有测试浏览器。测试覆盖注册、上传、事实确认、文本 JD、Mock 双文档、标签切换保留草稿、保存、PDF/DOCX 下载、刷新和 390px 窄屏；也用两个真实标签页验证版本冲突不丢本地编辑。截图与合成下载保存在被忽略的 `frontend/test-results/`。本次实际环境为 macOS Chromium + PostgreSQL 13.20。

## 发布边界

`npm --prefix frontend run build` 输出 `frontend/dist`。生产需要由同一 HTTPS Origin 托管静态文件并代理 `/api`，未知页面路径 fallback 到 index.html；静态文件服务不能暴露 FILE_STORAGE_DIR。Vite dev/preview 只用于本地开发，不是已完成的生产部署。跨浏览器、真实手机软键盘、负载和完整无障碍审计仍需上线前验收。
