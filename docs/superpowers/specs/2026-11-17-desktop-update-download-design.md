# 桌面端「检查更新 + 下载安装包」设计

> 【状态】2026-11-27 核验：该设计已落地（关于页检查更新与下载已交付），仅供历史参考。

日期：2026-11-17
状态：设计已确认（用户选定「直接下载当前平台安装包」+「下载到 ~/Downloads 显示进度」），待写实现计划
关联：设置弹窗「关于」页新增更新检查与下载按钮

## 1. 背景与问题

- 桌面端**已有**完整的自更新后端与前端区块（app/desktop/updater.go 的 Updater、packages/desktop/src/capabilities/updater.ts、app/web/.../tab_item/desktop.tsx 的 UpdateSection），但该区块位于「设置 → 桌面端」标签，而非「关于」页。
- 现有 Updater.Check/Install 依赖 MyGo 的 updates feed：mygo.config.ts 当前**没有** updates 配置，mygo.Updater.Enabled() 为 false，Supported=false，因此现有 UI 只会显示「前往发布页」，不会给出可下载安装的按钮。
- 服务端已有独立的 checkServerUpdate()（GET /api/v1/system/update/check），只查服务端版本，不查桌面端安装包，不能复用。
- 用户诉求：在「关于」页提供「检查更新」按钮；发现新版本时显示一个「下载更新」按钮，点击后**直接下载当前平台安装包到 ~/Downloads 并显示进度**。

## 2. 发布体系事实（单一事实源）

来源：.github/workflows/release.yml 文件头注释、scripts/release-naming.mjs、.github/versions/desktop.yaml。

- 一个版本只创建一个 GitHub Release，tag 固定为**服务端 tag** gospeak-server-v<version>；桌面端 tag gospeak-desktop-v<version> 只做构建触发与版本门禁，不单独建 Release。
- 两端版本必须一致（version-bump 校验），因此 Release tag 中的版本即桌面端版本。
- 桌面端安装包资产命名：gospeak-desktop-v<version>-<os>-<arch>.<ext>，例如：
  - gospeak-desktop-v1.1.0-rc.1-darwin-arm64.dmg
  - gospeak-desktop-v1.1.0-rc.1-windows-amd64.exe（setup.exe 统一重命名后的名字待 build 步骤确认）
  - gospeak-desktop-v1.1.0-rc.1-linux-amd64.tar.gz
- 校验和：SHA256SUMS-gospeak-desktop-v<version>.txt。

> 实现前须再次核对 release.yml 中 5 个平台资产的**最终上传文件名**（dmg / deb / tar.gz / setup.exe / install.sh 的重命名分支），下载按钮按实际资产名匹配。

## 3. 架构与数据流

### 3.1 总览

在现有 Updater 绑定上**新增两个方法**（不改动现有 Check/Install，保持自更新通道不被破坏）：

| 方法 | 职责 |
|------|------|
| CheckRelease(ctx) → ReleaseInfo | 请求 GitHub Releases API，解析最新 tag + 匹配当前平台资产，返回版本/说明/下载 URL/是否可用 |
| DownloadRelease(ctx, url) → error | 流式下载安装包到 ~/Downloads，通过 updater.progress 广播 0..1 进度 |

前端在「关于」页用这两个方法驱动「检查更新 / 下载更新」UI。

### 3.2 Go 宿主（app/desktop/updater.go）

新增数据结构：

    type ReleaseInfo struct {
        Available bool   // 有新版本
        Version   string // 最新桌面版本（如 1.1.0-rc.1）
        Notes     string // release body（截断）
        Current   string // 运行中版本（= version 变量）
        AssetName string // 匹配到的安装包文件名
        URL       string // 安装包浏览器下载直链
    }

CheckRelease：
1. GET https://api.github.com/repos/NoelOrin/GOSpeak/releases/latest（带 User-Agent、超时、ctx）；
2. 解析 tag_name → 去掉 gospeak-server-v 前缀得最新版本号；对比 version（当前版本）判断是否更新（用现有 Go 版本比较或简单 semver 比较，prerelease 需正确处理）；
3. 遍历 assets[].name，按 runtime.GOOS/GOARCH 拼出期望前缀 gospeak-desktop-v<ver>-<os>-<arch>. 匹配；未匹配则 Available=false 且给出说明；
4. 取 assets[].browser_download_url 作为下载 URL；
5. notes 截断（如 2000 字符）。

DownloadRelease：
1. 参数 url 来自 CheckRelease 返回值；**校验 URL host 必须是 github.com / objects.githubusercontent.com / github-releases.githubusercontent.com 白名单**，防止被投毒的 URL 任意下载（SSRF / 任意写文件）；
2. 目标路径 ~/Downloads/<assetName>，已存在时不覆盖，改用 <name>-<n> 或提示；
3. 流式 io.Copy，统计 downloaded/total，调用 updaterProgressEvent.Broadcast(fraction)（复用现有事件通道）；
4. 原子写：先写 .part 临时文件，完成后 rename；失败清理临时文件。

平台映射（runtime.GOOS/GOARCH → 资产 os-arch 段）：
- darwin/arm64 → darwin-arm64
- windows/amd64 → windows-amd64
- linux/amd64 → linux-amd64
- linux/arm64 → linux-arm64
- 其余 → 不支持，UI 显示「前往发布页」兜底

### 3.3 TS SDK（packages/desktop/src/capabilities/updater.ts）

- UpdateInfo 旁新增 ReleaseInfo 接口（字段对齐 Go JSON tag）。
- Updater 接口新增：
  - checkRelease(): Promise<ReleaseInfo>
  - downloadRelease(url: string): Promise<void>
- native 实现用 bridge.invoke(command("Updater","CheckRelease")) / command("Updater","DownloadRelease")；webFallback 返回 { available:false, current:"dev" } / 空实现。
- 命令名与 Go 绑定方法逐字一致，通过 scripts/check-desktop-commands.mjs 契约校验。

### 3.4 前端 UI（app/web/src/components/modal/settingModal/tab_item/about.tsx）

- 在「版本」Section 内、版本卡片下方新增更新区。
- 状态：idle | checking | available | latest | downloading | error。
- 挂载时不自动检查（避免每次打开弹窗就打 GitHub）；用户点「检查更新」才请求。
- 检查后：
  - available=true → 显示「发现新版本 vX + 说明（折叠）+ [下载更新] 按钮」；
  - available=false → 显示「已是最新版本」；
  - 错误 → 显示错误文案 + 「前往发布页」（复用 openReleasePage()）。
- 下载中：进度条 + 百分比（订阅 onProgress）；完成后提示「已下载到 ~/Downloads/<assetName>」。
- 仅 inDesktop() 为 true 时渲染该区块（Web 端不显示，避免误导）。
- 保持 token 规范：按钮用 @gospeak/components Button，圆角/字号走 token，图标 lucide-react。

## 4. 错误处理

- GitHub 不可达 / 限流 / 超时 → CheckRelease 返回错误，UI 展示「检查失败：<msg>」+ 前往发布页兜底按钮。
- 版本解析失败 / 无匹配资产 → 返回 Available=false 并在 UI 说明「当前平台暂无匹配安装包，前往发布页手动下载」。
- 下载 URL 白名单校验失败 → DownloadRelease 直接报错，不写文件。
- 下载中断 → 清理 .part，UI 回到可重试状态。
- 已存在同名文件 → 换名保存，UI 提示实际文件名。

## 5. 测试

- Go 单测（app/desktop/updater_release_test.go）：
  - Release tag → 版本解析（含 prerelease、脏字符）
  - 平台 → 资产名匹配（5 平台 + 不支持平台）
  - 下载 URL 白名单校验（合法/恶意 host）
  - notes 截断
- 前端测试（about.test.tsx 或现有 settingModal 测试）：
  - 点检查更新 → 有新版渲染下载按钮
  - 下载中进度条渲染
  - Web 环境不渲染该区块（mock inDesktop=false）
- 门禁：pnpm typecheck、node scripts/check-desktop-commands.mjs、pnpm --filter @gospeak/desktop-app build、部署后实测点击下载。

## 6. 范围外（YAGNI）

- 不启用 MyGo updates feed / 自动安装重启（现有 Check/Install 通道保持原样不动）。
- 不校验 SHA256（发布链有校验和，但「下载安装包」不是自更新，校验可作为后续增强）。
- 不改「桌面端」标签里的现有 UpdateSection（本次只动「关于」页）。
- 不引入新的分享/社交能力。

## 7. 验收标准

1. 桌面端「设置 → 关于」页可见「检查更新」按钮。
2. 点击后有网络时能正确显示最新版本与说明；已是最新时显示对应文案；失败时显示错误 + 发布页兜底。
3. 有新版时「下载更新」按钮点击后，安装包下载到 ~/Downloads，界面实时显示进度，完成后提示文件名。
4. Web 构建不渲染该区块；typecheck / 契约校验 / 构建 / 单测全绿。
5. 实测一次完整下载流程成功。
