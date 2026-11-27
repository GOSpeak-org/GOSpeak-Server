# 三端共享反代规则层（缩版 A）实施计划

> 【状态】2026-11-27 核验：该计划已交付（6 个任务全部完成），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

> **Status**（2026-11-17 更新）：**全部 6 个任务已交付**。三端（桌面 / Android / iOS）现在共用 `app/shared/proxyrules` 这一份规则实现，内联重复实现均已删除。
>
> **原生接入已由 CI 实测编译通过**（run 44006296505 @ f230a949，全 13 个 job 绿）：`gomobile bind` 产出 AAR / XCFramework → `gradlew assembleDebug` 通过、APK 上传 → `xcodebuild`（iOS Simulator）通过、App 打包上传。本机（无 Xcode / Android SDK / NDK / JDK）只验证到 `gobind` 绑定面与 Swift 类型检查，够不到实际编译。
>
> **后续状态**：移动端原生构建已从 CI 拆出为独立手动 workflow `.github/workflows/build-mobile.yml`（产物发到固定 pre-release `gospeak-mobile-test`），不再参与 CI / gate。该 workflow 目前**只构建 Android**；iOS 构建仍暂时禁用（步骤以注释形式保留，解开即恢复）。XCFramework 仍在绑定面步骤里产出并上传，禁用期间改 iOS 原生代码不再有 CI 拦截。
>
> iOS 侧本地可验证部分已实测：用 `gobind -lang=objc` 的真实生成头文件搭 framework 布局，`swiftc -typecheck` 对 `LocalProxyServer.swift` 通过，且反向对照（故意写错函数名）会报 `cannot find in scope`，证明检查有效。
>
> 交付核验：`app/shared/proxyrules` 单测全绿（8 个测试函数 / 73 个子测试）；`app/desktop` 全部测试零回归（10/10 host:port 用例逐字一致，255 组 worker 组合零差异）；`scripts/mobile-bind-proxyrules.mjs` 16 项单测通过并实证可拦下 gobind 静默跳过；`check-mobile-bridge` / `check-desktop-commands` 仍 in sync。

**Goal:** 把反代中的三类纯规则——服务器地址校验、`Set-Cookie` 改写、`/wsworker` 目标白名单——收敛为一份 Go 实现、一份 Go 测试；桌面直接调用，移动端经 gomobile 绑定调用。代理的 HTTP/WS 骨架仍由各端原生实现，不追求「一份代码跑三端」。

**Architecture:** 新增纯 Go 包 `app/shared/proxyrules`，导出只含 `string` / `bool` / `error` 的函数。桌面 `app/desktop/proxyserver.go` 直接 import；Android / iOS 用 `gomobile bind` 产出 AAR / XCFramework，在三个决策点调用。规则层无网络、无文件、无回调，因此不触碰 gomobile 的类型边界限制。

**非目标：** 不替换各端原生代理骨架；不改鉴权方式（仍用 HttpOnly Cookie）；不把 `embed.FS` 或 worker host 回调跨语言传递；不在本轮引入完整 Go 代理绑定（见「为什么不选完整 A」）。

## 为什么不选完整 A

完整 A 指把 `proxyserver.go` 整体抽成 Go 包、gomobile 绑定后让移动端直接跑 Go 代理。当前不建议：

- `embed.FS` / `fs.FS` 与 `allowedWorkerHosts func() []string` 都过不了 gomobile 绑定边界，必须重写桌面已验证的安全逻辑。
- Go 版本与工具链缺口导致产物在本机完全无法验证，用不可验证换可维护，不划算。
- iOS 无 Xcode，XCFramework 链接无法确认。

缩版 A 只共享规则，规避上述全部三点，且拿到主要收益（规则一份实现、一份测试、三端行为一致）。

## Global Constraints

- 规则函数输入输出只能是 `string` / `bool` / `error`，禁止接口、函数回调、`fs.FS`。
- 桌面端现有安全语义不得改变；白名单从「回调注入」改为「函数计算」后结果必须与现有一致。
- 移动端原生代理骨架（HTTP/WS 转发、静态资源、连接管理）保持各端独立实现，本计划不改。
- 产物接入不得阻塞当前版本交付：工具链未就绪时，移动端可先保留内联等价实现。

## Tasks

### Task 1: 纯 Go 规则包

- [x] 创建 `app/shared/proxyrules/`（独立 Go module `gospeak/proxyrules`，go 1.26.0；**未引入 `go.work`**，避免扰动 server 1.26 与 desktop 1.27.3 的 toolchain 解析）。
- [x] 实现 `ValidateBaseURL(raw string) error`：只接受绝对 `http(s)` URL，空串视为清空。与桌面原实现逐字等价。
- [x] 实现 `RewriteSetCookie(value string) string`：去掉 `Domain=` 与 `Secure`，保留 `HttpOnly` / `Path`；处理单条含多个属性。**偏差**：SameSite 一律归一为 `SameSite=Lax`（不是「保留」）——桌面 `fixCookies` 本就强制 Lax，且去掉 Secure 后 `SameSite=None` 会被浏览器丢弃。
- [x] 实现 `IsTrustedWorker(target, baseURL, allowed string) bool`：比较归一化后的 `host:port`。**偏差**：多一个 `allowed` 参数（逗号分隔 `host:port` 列表）——只比 baseURL 会误拒后端下发的其它 worker host；用字符串而非 `[]string`，因为 gobind 会**静默跳过** `[]string` 参数且仍 exit 0。
- [x] 单测 `proxyrules_test.go`：覆盖上述边界，另加 workspace 大小写、带空格属性名、CSV 空白项等。`NormalizedHostPort` 返回 `(string, error)` 而非 `(string, bool)`（gobind 硬性要求）。

### Task 2: 桌面接入

- [x] `app/desktop/proxyserver.go` 三处内联判断改为调用 `proxyrules`；`normalizedHostPort` 私有实现删除。go.mod 用 `require` + `replace` 指向本地目录。
- [x] 跑 `cd app/desktop && go test ./...`，全部测试通过，期望值零修改（仅测试调用点换成共享函数）。
- [x] 对比改动前后输出：`NormalizedHostPort` 10/10 用例逐字一致；worker 规则 255 组组合（17 目标 × 5 baseURL × 3 白名单）零差异。

### Task 3: gomobile 工具链与产物

- [x] gomobile 经模块 `tool` 指令锁定（`gobind` + `gomobile`，版本随 `golang.org/x/mobile` 走），写入 `app/mobile/AGENTS.md`。
- [x] `gomobile bind` 由 CI 的 `build-mobile` 产出 AAR / XCFramework（本机无 Xcode 与 Android SDK，故本地只验证 gobind 绑定面）。
- [x] CI 已加绑定面校验：阻断性门禁放在 `desktop-go`（ubuntu、有 setup-go、在 gate 内、按 `go` 过滤器触发，因为 `app/shared/**` 只在该过滤器里）；`build-mobile` 保留产物产出尝试。产物缓存与 demo 链接验证随 AAR/XCFramework 一并暂缓。

### Task 4: Android 接入

- [x] gradle 以 `flatDir` 依赖 `../../build/proxyrules/proxyrules.aar`，四处决策点改为调用绑定库；缺产物时 `GradleException` 直接给出要跑的命令。
- [x] `LocalProxyServer.java` 的 `sameHostPort` / `hostPort` / `rewriteCookie` / `parseBase` 内联实现已删除。
- [x] 本地无法运行 gradle（无 JDK / Android SDK）；编译验证由 CI `build-mobile` 的 `assembleDebug` 承担。

### Task 5: iOS 接入

- [x] `Proxyrules.xcframework` 已加入 `App.xcodeproj`（PBXFileReference + Frameworks 阶段 + FRAMEWORK_SEARCH_PATHS），并使 `App/Proxyrules-Bridging-Header.h` 暴露 C 接口给 Swift。
- [x] `LocalProxyServer.swift` 的 `rewriteSetCookie` 内联实现已删除，worker 判定与 `ServerConfigPlugin` 的地址校验改调绑定库。
- [x] 本地用真实 gobind 生成头文件搭 framework 布局，`swiftc -typecheck` 对改动后的 `LocalProxyServer.swift` 通过；反向对照验证检查有效。iOS 完整构建由 CI 承担。

### Task 6: 收尾

- [x] 更新 `app/mobile/AGENTS.md`、`packages/mobile-capabilities/AGENTS.md`、`scripts/AGENTS.md`、`app/desktop/AGENTS.md` 与根 `AGENTS.md` 的规则层说明。
- [x] 跑 `scripts/check-mobile-bridge.mjs`、`scripts/check-desktop-commands.mjs`、`scripts/mobile-bind-proxyrules.mjs` 与各包测试。
- [x] 更新本计划 Status；全部任务已交付，已从 `docs/superpowers/plans/README.md` 索引中移除（历史见 git log）。

## Verification

- `cd app/shared/proxyrules && go test ./...`
- `cd app/desktop && go test ./...`
- `gradlew compileDebugJavaWithJavac`
- `xcodebuild`（需完整 Xcode）
- `node scripts/check-mobile-bridge.mjs`
- `pnpm typecheck` + web / SDK 全量测试

## 剩余风险

- gomobile 版本与 Go 1.27.x 的兼容性需实测，可能需要单独工具链。
- iOS 上 URLSession 会合并多个 `Set-Cookie`，纯函数只能改写拿到的字符串，无法还原被合并前的多条——与当前实现的限制相同，本计划不解决。
- 规则层只覆盖三类判断；HTTP/WS 转发行为仍由各端原生实现，不承诺跨端完全一致，需靠真机验收。
