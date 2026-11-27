# app/mobile 移动客户端选型与设计

> 【状态】2026-11-27 核验：该设计已落地（client/mobile 移动壳已交付），仅供历史参考。

日期：2026-11-16
状态：设计已确认（四节设计逐节获用户认可），待写实现计划
关联：选型起点为「为移动端选型」；GPUI 路线经调研后不采用，最终选型为 Capacitor 混合方案

## 1. 选型结论

### 1.1 决策

新增 `app/mobile` 包，作为 GOSpeak 移动客户端的 **Capacitor 原生壳**；移动 UI 复用现有 `app/web` 的 React 19 源码（UI 已做响应式适配），通过扩展构建期目标常量 `GOSPEAK_TARGET=mobile` 产出移动端 bundle，再由 Capacitor 同步进 iOS/Android 原生工程。原生能力（后台音频、推送、深链等）通过新建的 `packages/mobile-capabilities` 能力层提供，模式对齐现有 `packages/desktop`。

### 1.2 选型过程（为何不是 GPUI / RN / Flutter / PWA）

用户最初要求探索 GPUI 相关方案。调研结论（均有一手来源）：

- **GPUI**：Zed 团队出品的 pre-1.0 Rust 声明式 UI 框架，crate v0.2.2，API 频繁 breaking change，「for the near future, GPUI is tied to Zed」[gpui.rs](https://gpui.rs/) [docs.rs](https://docs.rs/gpui/latest/gpui/)。组件层 GPUI Kit v0.9.1 桌面生产验证，但**官方明确移动端是实验性能力**："primary target remains the desktop… does not currently plan to make mobile a primary target or to become a full mobile application framework in the way Flutter is"；移动依赖临时兼容包 `gpui-pre-mobile`，仅验证过有限的聊天 UI 场景，完整应用布局「have not been validated on mobile yet」[GPUI Kit Mobile Guide](https://gpui-kit.com/docs/mobile)。Rust LiveKit SDK 可用（livekit 0.11.0），技术闭环成立，但移动端 AudioSession / 后台音频 / 回声消除需大量原生胶水，且 UI/状态/协议消费层几乎零复用。
- **React Native / Flutter**：媒体层成熟（livekit-react-native 等），但 UI 与状态层需重写，团队成本高。
- **PWA**：复用率最高，但无可靠后台音频与原生推送，对语音 App 是硬伤。
- **Capacitor 混合（采用）**：原生壳 + 移动优化 Web，最大化复用 React 19 / `@gospeak/components` / `@gospeak/sfu-client` / socket / 协议类型；后台音频、推送、深链由 Capacitor 插件补齐。与现有桌面端「原生宿主 + TS 能力 SDK」同构。周期与风险最低。

用户在候选中明确选择 Capacitor 混合方案。

## 2. 架构与代码复用边界

### 2.1 确认的产品决策

1. **复用整个 `app/web` 源码**，不新建独立移动 UI 源码树；UI 响应式适配已完成（现存 `sm:`/`md:`/`lg:` 等 Tailwind 响应式类 84 处）。
2. 不抽取 `@gospeak/client-core` 之类的新共享包——`app/web` 继续作为唯一 React 客户端源码。
3. `app/mobile` 只做 Capacitor 原生壳与配置，不产出独立 React 组件。
4. 平台差异收敛到能力层，业务代码用平台判定门面，不直接调原生插件。

### 2.2 包结构

```
app/
├── web/                # 唯一 React 源码（目标：web / desktop / mobile）
└── mobile/             # 新增：Capacitor 原生壳
    ├── package.json           # @gospeak/mobile（private）
    ├── capacitor.config.ts    # webDir 指向 web 的移动端产物、appId、scheme
    ├── ios/ / android/        # cap add 生成的原生工程（提交到仓库）
    └── AGENTS.md
packages/
└── mobile-capabilities/       # 新增：移动能力 SDK（native + Web 降级）
    ├── src/
    │   ├── capabilities/      # audio-session / media-control / push / deeplink / audio-route / lifecycle
    │   ├── conditional.ts     # inMobile() 等平台判定
    │   ├── bridge.ts          # Capacitor 插件调用封装
    │   └── index.ts
    ├── package.json           # @gospeak/mobile-capabilities
    └── AGENTS.md
```

`pnpm-workspace.yaml` 的 `app/*` / `packages/*` 通配已覆盖新包，无需改 workspace 配置。

## 3. 构建目标 `GOSPEAK_TARGET=mobile`

### 3.1 构建期常量

扩展现有 `GOSPEAK_TARGET` 机制（现有值 `web` | `desktop`）：

- `app/web/vite.config.ts`：新增 `mobile` 值，注入 `__GOSPEAK_TARGET__ = "mobile"` 与布尔 `__GOSPEAK_MOBILE__`。
- `packages/mobile-capabilities/src/globals.d.ts`：声明 `__GOSPEAK_MOBILE__`（与 desktop 包各自声明自己的构建常量，避免跨包耦合）。
- `packages/mobile-capabilities/src/conditional.ts`：新增 `inMobile()`，与 `packages/desktop` 的 `inDesktop()` 同构；业务代码统一用它判定平台，禁止裸调原生 API。
- `packages/desktop` 不改动（其 `inDesktop()` 在 mobile 目标下恒为 false，语义正确），保持桌面/移动能力层互不依赖。
- 平台常量在 Web 构建中被折叠剔除，mobile 独享模块在 web/desktop 构建中摇树掉。

### 3.2 构建命令与产物

- `app/web`：新增 `build:mobile` = `GOSPEAK_TARGET=mobile vite build && tsc`，产物 `dist-mobile/`；`dev:mobile` = `GOSPEAK_TARGET=mobile vite`。
- `turbo.json`：新增 `build:mobile` / `cap:sync` 任务（依赖 `^build`）。
- `app/mobile`：
  - `cap sync` 把 `dist-mobile` 拷入 `ios/`、`android/` 并安装原生插件。
  - dev：Capacitor `server.url` 指向局域网内 Vite `dev:mobile` 地址（真机同网段热重载）。
  - release：加载内嵌 bundle（Capacitor 默认行为，与桌面 `webui/dist` 内嵌同构）。
- `GOSPEAK_TARGET` 仍由 Vite 注入，构建产物目标分离，与桌面 `build:desktop` 互不干扰。

## 4. 原生能力与会话连续性

全部 6 项纳入 MVP。

### 4.1 能力清单与实现

| 编号 | 能力 | 实现方式 |
|---|---|---|
| ① | 后台音频 | iOS 开 `UIBackgroundModes: audio` + `AVAudioSession` 设 `playAndRecord`；Android 前台 Service + `FOREGROUND_SERVICE_MICROPHONE` 权限。自定义 Capacitor 插件 `AudioSession` 在语音会话开始/结束时 activate/deactivate。 |
| ② | 锁屏/通知栏控制 | `capacitor-media-session`（Android WebView 不支持 Media Session Web API，需原生插件；iOS 为 Web API 薄封装）[capacitor-media-session](https://github.com/jofr/capacitor-media-session/)，静音/挂断事件回传 Web 层。 |
| ③ | 推送 | `@capacitor/push-notifications`（APNs + FCM）；device token 绑定当前用户上传后端；点击通知走深链。 |
| ④ | 深链 | Capacitor `appUrlOpen` 事件 + `gospeak://` URL scheme；复用现有 `useDeepLinkNavigation` 逻辑，与桌面 deeplink 能力对齐。 |
| ⑤ | 音频路由 | 自定义插件封装 iOS `AVAudioSession` 输出选项 / Android `AudioManager`：列出/切换 蓝牙、扬声器、听筒。 |
| ⑥ | 会话连续性 | 复用 `@gospeak/sfu-client` 既有 15s 墙钟重连策略；新增 Capacitor `appStateChange` 生命周期监听：回前台主动 socket 重连 + 媒体会话健康检查；来电/系统打断后恢复 AudioSession。 |

### 4.2 能力层收敛（`packages/mobile-capabilities`）

模式对齐 `packages/desktop/src/capabilities/`：

- 每个能力导出 `create<Name>()`，内部同时实现 native（Capacitor 插件）与 Web 降级；降级必须无副作用且可调用，不抛异常。
- 业务代码只用 `inMobile()` + 能力门面，禁止页面直接 import Capacitor 插件。
- `bridge.ts` 统一封装插件调用与事件订阅；方法名 TS↔原生必须逐字一致（见 §6 门禁）。
- `index.ts` 具名导出 + `createMobile()` 聚合对象。

### 4.3 关键风险（实现前必须验证）

**后台 WebRTC 音频 PoC**：WKWebView / Android WebView 在锁屏后能否持续跑 WebRTC 音频，是整个混合方案成立的前提。实现计划第一阶段必须做真机 PoC：iOS（真机 + 锁屏）与 Android（真机 + 锁屏）。

- 若通过：按设计继续。
- 若不通过：⑤⑥ 需下沉为原生媒体层（引入原生 WebRTC/AudioSession 引擎，Web 层只做信令与 UI），成本上升接近 React Native 方案，需回到用户处重新决策。

此 PoC 是手动门禁，不进 CI 自动化测试。

## 5. 数据流与关键路径

- **启动**：原生壳加载 `dist-mobile` → Web 应用按 `__GOSPEAK_TARGET__=mobile` 折叠桌面分支 → `main.tsx` 注入 QueryClient/Router → Capabilities 初始化（AudioSession 等）。
- **语音会话**：用户加入房间 → 能力层 activate AudioSession → `@gospeak/sfu-client` joinRoom（JS LiveKit，复用 Web 全部逻辑）→ media-session 原生控件同步 → 锁屏可控制。
- **后台/前台**：`appStateChange` → 后台时保持 LiveKit 会话（依赖 PoC 验证）；回前台做健康检查和必要重连。
- **深链入会**：`gospeak://invite/d/<CODE>` → `appUrlOpen` → 复用 `useDeepLinkNavigation` 导航到邀请落地页。

## 6. 测试 / CI / 门禁

- **Web 层**：沿用 `vitest` / `tsc --noEmit` / `biome check`，与 `app/web` 同一套；`build:mobile` 纳入 `pnpm typecheck` 覆盖面。
- **原生壳**：CI 仅做 `cap sync` + 原生项目编译（macOS runner 编 iOS、Linux runner 编 Android），不跑真机测试；真机 PoC 与手动验收是独立门禁。
- **桥接契约门禁**：参照 `scripts/check-desktop-commands.mjs`，新增 `scripts/check-mobile-bridge.mjs`，在 CI + pre-push 校验 `packages/mobile-capabilities` 的插件方法名与 iOS/Android 原生实现逐字一致。
- **turbo 任务**：`build:mobile`、`cap:sync` 纳入 CI changes 检测与聚合 gate。
- **桌面不回退**：改 `app/web` 后仍需跑 `pnpm --filter @gospeak/web build:desktop`（现有约束不变）；mobile 目标为新增验证。

## 7. 文档

- 新增 `app/mobile/AGENTS.md`（壳结构、构建、Capacitor 配置、原生工程维护约定）。
- 新增 `packages/mobile-capabilities/AGENTS.md`（能力模式、条件编译、桥接契约、Web 降级要求）。
- 更新根 `AGENTS.md`：目录骨架加 `app/mobile` 与 `packages/mobile-capabilities`，构建常量表加 `mobile` 值。
- 本设计文档位于 `docs/superpowers/specs/`。

## 8. 明确不做（YAGNI / 边界）

1. 不做第二套移动 UI 源码（复用 `app/web`）。
2. 不引入新状态管理（沿用 zustand + react-query）。
3. 不改后端协议、不新增 WS 事件、不新增 SFU provider（仍 LiveKit 唯一）。
4. 不恢复「多 provider 动态切换」「访客体系」等已删除能力。
5. 不在 MVP 做后台 WebRTC 原生引擎下沉（除非 PoC 失败触发重新决策）。
6. 不做应用内更新 / 热更新分发（后续迭代）。
