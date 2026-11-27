# 关于页 GitHub 项目地址入口 — 设计

> 【状态】2026-11-27 核验：该设计已交付（关于页项目地址入口已落地），仅供历史参考。

日期：2026-11-22

## 背景与目标

「设置 → 关于」页当前只展示客户端版本、运行环境、更新与日志导出，没有回到源码仓库的入口。用户希望在关于页加一个「项目地址」项，点击跳转到 `https://github.com/GOSpeak-org/GOSpeak-Client`。

成功标准：

- Web 与桌面端都能在「版本」卡片看到「项目地址 / GitHub ↗」。
- 点击后：浏览器新标签打开；桌面端经宿主调用系统默认浏览器打开。
- 不破坏现有关于页测试与桌面桥接契约。

## 设计

### UI（app/web/src/components/modal/settingModal/tab_item/about.tsx）

- 在「版本」卡片内新增第三行，标签「项目地址」，值为可点击的 `GitHub ↗`。
- 链接地址常量 `REPO_URL = "https://github.com/GOSpeak-org/GOSpeak-Client"`（与 client 仓 README 中的仓库地址一致）。
- `<a href={REPO_URL} target="_blank" rel="noreferrer">`：Web 端保留默认行为；`inDesktop()` 时 `preventDefault()` 并调用 `browser.openExternal(REPO_URL)`。

### 桌面宿主（app/desktop）

- 新增 `Browser` 绑定服务（`browser.go`）：`func (Browser) OpenExternal(ctx context.Context, rawURL string) error`，复用 `updater.go` 已有的 `openInBrowser`。
- 仅接受 http/https 方案，其余返回错误；`exec.Command` 直接传参，不经 shell。
- `main.go` 增加 `mygo.Bind(Browser{})`（第 13 个绑定）。

### SDK（packages/desktop）

- 新增 `capabilities/browser.ts`：`createBrowser()` 暴露 `openExternal(url)`。
  - native：`bridge.invoke(command("Browser", "OpenExternal"), url)`。
  - 浏览器降级：`window.open(url, "_blank", "noopener,noreferrer")`，无副作用、不抛异常。
- `index.ts` 导出 `createBrowser` 与 `Browser` 类型；与 notification/dialog 一样保持独立导出，不加入 `createDesktop()`。

### 测试

- `about.test.tsx`：mock `createBrowser`；断言链接存在且 href 正确；桌面模式点击调用 `openExternal`，Web 模式不调用。
- `browser.test.ts`：验证浏览器降级调用 `window.open`，native 路径命令字符串正确。

## 验证

- `pnpm --filter @gospeak/web typecheck`
- `pnpm --filter @gospeak/web test -- about`
- `node scripts/check-desktop-commands.mjs`
- `cd app/desktop && go build ./...`
- `pnpm --filter @gospeak/web build:desktop`

## 不在范围内

- 不改反馈/邮件模板、更新检查逻辑，不引入通用设置项配置。
- 不改工作区已有的其他未提交改动。
