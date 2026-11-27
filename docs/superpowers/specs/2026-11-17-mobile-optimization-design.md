# 移动端问题优化设计

日期：2026-11-17
状态：设计已确认（14 项范围与方案获用户认可），待写实现计划
关联：源于「优化一下移动端的问题」；经 UI / 原生 / 性能三路只读审查共发现 31 项问题，本规格覆盖 P0 八项 + 核心体验六项

## 1. 背景与范围

### 1.1 问题来源

对三端代码做只读审查（未改动任何文件），问题分布：

- **原生能力层 10 项**：两处功能硬伤（iOS 漏注册关键插件、Push 全链路未接线），其余为后台音频、音频路由状态、监听器生命周期等
- **UI / 响应式 9 项**：弹窗窄屏不可用、安全区遗漏、平台判定脱钩、触控目标过小等
- **性能 12 项**：常驻 rAF、列表未虚拟化、全量 store 订阅、render 阶段昂贵计算等

### 1.2 本次范围（14 项）

| # | 领域 | 问题 | 严重度 |
|---|------|------|--------|
| 1 | 原生 | iOS `ServerConfigPlugin` 未注册，无法配置服务器地址 | 高 |
| 2 | 原生 | Push 能力是死代码，web 层零引用 | 高 |
| 3 | UI | 通知弹窗窄屏不可读；创建/编辑房间弹窗不可滚动、按钮不可达 | 高 |
| 4 | UI | 全屏弹窗 Portal 在 `#app` 外，安全区内缩失效 | 高 |
| 5 | UI | `isMobile` 只看宽度，平台判定与 `inMobile()` 脱钩 | 高 |
| 6 | 性能 | 登录页 rAF 永不停止 | 高 |
| 7 | 性能 | 房间列表一次性渲染全部成员，头像无懒加载 | 高 |
| 8 | 性能 | 每条消息每次渲染重解析 Markdown | 高 |
| 9 | 原生 | Android 声明前台服务权限但无 Service | 中 |
| 10 | 原生 | 音频路由当前值是本地缓存，从不读取原生 | 中 |
| 11 | 原生 | Android `activate()` 强制听筒，与「默认外放」约定相反 | 中 |
| 12 | UI | 移动端房间无可见的编辑/删除/退出入口 | 中 |
| 13 | 性能 | `socketStore` / `userBar` / `chatWindow` 全量订阅 | 中 |
| 14 | 性能 | `MessageList` O(n²) 查找；`useCoarsePointer` 每行注册监听 | 中 |

### 1.3 明确不在范围

- **Push 后端 APNs / FCM 下发链路**：仓库当前无任何实现，本次只做前端接线与明确降级，不伪造端到端成功
- **其余 17 项低/中优先级问题**：listener 句柄泄漏、TypingIndicator 1Hz 轮询、启动即申请麦克风、触控目标 22px、管理页导航小窗、`dvh` 统一等，留待后续轮次
- **构建 / 发布链路**：用户本次未选择

## 2. 设计原则

1. **平台判定统一**：行为分支一律用 `inMobile()`（来自 `@gospeak/mobile-capabilities`），样式分支看 `html[data-platform="mobile"]`；视口宽度断点仅作为桌面窄窗的补充，不得作为移动壳的判定依据
2. **安全区收敛**：`#app` 的 `env(safe-area-inset-*)` 不覆盖 Portal 内容，全屏弹窗需自行处理；高度统一 `dvh`
3. **性能三板斧**：消灭全量 store 订阅、把昂贵计算移出 render 或加缓存、休眠常驻 rAF / 定时器
4. **可验证**：每项修复带单测或门禁；原生改动带编译/绑定校验

## 3. 原生能力（4 项）

### 3.1 iOS 注册 ServerConfigPlugin

**问题**：`app/mobile/ios/App/App/SceneDelegate.swift:47-56` 的 `registerCustomPlugins(on:)` 只注册 `AudioSession` 与 `MediaControl`，漏掉 `ServerConfig`。`capacitor.config.json:28-33` 的 `packageClassList` 只含 4 个官方插件，app-local 插件不会自动注册。结果：iOS 上 `getBaseUrl()` 静默回退到空 localStorage，`setBaseUrl()` 抛错，用户无法配置服务器。`scripts/check-mobile-bridge.mjs` 只比对方法名声明，不检查注册，CI 通过。

**方案**：
- 在 `registerCustomPlugins` 中按现有模式补 `ServerConfigPlugin` 注册（先查 `bridge.plugin(withName:)` 避免重复）
- 扩展移动端 bridge 门禁：除方法名外，校验 iOS `SceneDelegate` 与 Android `MainActivity` 的注册列表覆盖 `@gospeak/mobile-capabilities` 声明的全部插件（至少做静态文本断言）

### 3.2 Push 前端接线

**问题**：`packages/mobile-capabilities/src/capabilities/push.ts:28-86` 实现了 `register()` / `onToken()` / `onNotificationOpened()`，但 `app/web/src` 零引用；`useMobileSessionBridge.ts` 无 push 调用。权限被拒仍继续注册、监听器可能重复注册（第 34-64 行）。

**方案**：
- 在登录成功后统一入口调用 `push.register()`，检查 `requestPermissions()` 返回的 `receive === "granted"` 后再注册，未授权返回明确失败原因
- 保存 listener handle，在 `start()` 失败/重试时去重（与问题 #2 的句柄泄漏部分一并处理，不做完整泄漏重构）
- `onToken` 经可替换 adapter 上报（adapter 接口先行、默认实现为「记录日志 + 返回未配置」）；`onNotificationOpened` 接 `useDeepLinkNavigation` 已有跳转逻辑
- SDK 注释与 `AGENTS.md` 明确写清：完整推送需后端 APNs/FCM，当前仅前端就绪

### 3.3 Android 前台服务

**问题**：`app/mobile/android/app/src/main/AndroidManifest.xml:4-9` 声明 `FOREGROUND_SERVICE` / `FOREGROUND_SERVICE_MICROPHONE`，但无任何 `<service>`；`MediaControlPlugin.java:76-95` 只用 `notificationManager().notify()` 发普通通知。Android 12+ 切后台/锁屏时麦克风无前台服务承载，可能被回收。

**方案**：
- 新增前台 Service，`foregroundServiceType="microphone"`，Manifest 声明 `<service>`
- `MediaControlPlugin.activate()` 启动服务并以 `startForeground()` 发锁屏通知，`deactivate()` 停止服务
- 保留现有通知样式与静音 / 挂断 action 回传，不改动原生插件方法名（bridge 契约不变）
- 真机 PoC 验证后台麦克风持续采集（本机无 Android SDK，仅做编译门禁与代码审查）

### 3.4 音频路由同步

**问题**：`audio-route.ts:49,66-68,73-76` 的 `current()` 只返回闭包内初始为 `"speaker"` 的局部变量；`useMobileAudioRoute.ts:23-26` 在设置页挂载时读到过期值。Android `AudioSessionPlugin.java:26` 的 `activate()` 调用 `setSpeakerphoneOn(false)`，与「默认外放」约定及 iOS `.defaultToSpeaker` 相反，且 `deactivate()` 不恢复原状态。

**方案**：
- SDK 增加异步 `refresh()`：调原生 `current()` 并更新本地缓存；`useMobileAudioRoute` 挂载与路由变化后调用
- Android `activate()` 改为 `setSpeakerphoneOn(true)`，保存进入房间前的 speakerphone 状态，`deactivate()` 恢复

## 4. UI / 响应式（4 项）

### 4.1 通知弹窗窄屏布局

**问题**：`notificationModal.tsx:64,82,122` 左侧 `w-72 shrink-0`（288px）几乎占满 360px 宽屏，右侧正文仅 ~32px；`80vh` 用 `vh`。

**方案**：用容器查询（对齐 `settingModal` 现有 `@container` 模式），窄容器单栏（列表 / 详情切换或列表压缩为横向标题条），宽容器双栏 `@md:w-72`；高度统一 `dvh`，补 safe-area 内缩。

### 4.2 表单弹窗可滚动

**问题**：`createRoomModal.tsx:179-205`、`editRoomModal.tsx:76-106` 的 `DialogContent` 是 `fixed top-1/2 -translate-y-1/2`，无 `max-h` 与内部滚动；字段单列后总高 1000px+，键盘弹起后提交按钮不可达。`packages/components/src/components/dialog.tsx:21` 是共同基类。

**方案**：对这两个弹窗的 `DialogContent` 加 `flex max-h-[100dvh] flex-col overflow-hidden`，表单区 `min-h-0 overflow-y-auto`，提交按钮 sticky 底栏；与 `useMobileKeyboard` 的 `scrollIntoView` 配合，不改全局 Dialog 基类以免影响其他弹窗。

> 约束提醒：圆角/字号/图标尺寸走 `theme.css` token，禁止任意值；-tailed 工具类允许用于布局。

### 4.3 全屏弹窗安全区

**问题**：`settingModal.tsx:59,61,73` 的全屏弹窗 `h-[100dvh] w-full rounded-none p-0`；Radix Portal 挂到 `document.body`，位于 `#app` 外，`global.css:23-29` 的安全区内缩不适用；`sm:h-[min(90vh,820px)]` 又退回 `vh`。

**方案**：给全屏 `DialogContent` 自身加 `pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]`（或把 Portal container 指向 `#app`）；所有高度改 `dvh`。

### 4.4 平台判定与房间操作入口

**问题**：`layout.tsx:368-373,393-398,436-489` 的 `isMobile` 只由 `matchMedia("(max-width: 767px)")` 决定，iPad / 横屏大屏手机走桌面布局；分隔条只监听 `mousedown/mousemove`，触屏拖不动。`roomItem.tsx:102-120,222-238` 的编辑/删除/退出只绑 `onContextMenu`，移动端无可见入口。

**方案**：
- 布局与交互语义改 `inMobile()` 优先，宽度断点作为桌面窄窗补充；拖拽改用 Pointer Events（`pointerdown/pointermove/pointerup` + `setPointerCapture`）
- 房间行尾新增可见「…」按钮打开同一菜单；单接入房与菜单入口分离；同步修正 `layout.tsx:282` 与 `manageNav.tsx` 的过时注释

## 5. 性能（6 项）

### 5.1 登录页 rAF 休眠

**问题**：`useAuthParallax.ts:26-37` 的 `requestAnimationFrame(tick)` 永不停止；触屏设备无 `mousemove`，从进入登录页起 60fps 空转。

**方案**：无 hover（`matchMedia("(hover: none)")`）直接 return；桌面环境在 `targetX/targetY` 差值小于阈值时停止 rAF，下次 `mousemove` 唤醒。保留 `prefers-reduced-motion` 处理。

### 5.2 房间列表虚拟化

**问题**：`roomItem.tsx:269-275` + `roomList.tsx:220-236` 一次性渲染全部房间及每个房间全部成员（20 房 × 20 人 ≈ 400 个按钮 + 头像）；`avatar.tsx:64-70` 的 `<img>` 无 `loading="lazy"` / `decoding="async"`；`room:updated` 等事件让 `rooms` 换新后整列表重渲。

**方案**：用 `@tanstack/react-virtual` 虚拟化房间列表；成员默认折叠、点开再渲染；头像加 `loading="lazy" decoding="async"`。注意保留双击/菜单交互与 #4.4 的可见菜单入口。

### 5.3 Markdown 解析缓存

**问题**：`MarkdownText.tsx:72-92` + `MessageItem.tsx:25-34` 每条可见消息每次渲染执行 `marked.parse` → `DOMPurify.sanitize` → `DOMParser` → `TreeWalker`；`MessageItem` 未 `memo`，父组件订阅 `messages.length` 触发整面板重渲。

**方案**：`React.memo(MessageItem / MarkdownText)`；以 `(text, selfName)` 为 key 做 LRU 缓存（上限约 200 条）缓存解析后 HTML。

### 5.4 store 订阅收敛

**问题**：
- `socketStore.ts:690-694` 监听整个 `useDomainStore`，进一个域连续触发 7-10 次 `room:list`
- `userBar.tsx:32` `useVoiceChatStore()` 无 selector
- `chatWindow.tsx:35-50` 订阅完整 `pmMessages` Record，`buildRows` 无 `useMemo`

**方案**：`socketStore` 仅监听 `currentDomainUUID` 变化（selector 或 prev/next 比较）；`userBar` 用 selector 只取音量/静音四字段；`chatWindow` selector 到 `pmMessages[active] ?? []`，rows 用 `useMemo`。

### 5.5 MessageList 查找优化与 CoarsePointer 提升

**问题**：`MessageList.tsx:56-84` 线程视图用 `all.find()` 造成 O(n²)，`buildRows` 每次渲染重建；`MessageItem.tsx:34` 每行调 `useCoarsePointer()`，20-30 行各自注册 `matchMedia` 监听。

**方案**：构建 `uuid → 消息` Map 做 O(1) 查找；`visibleMessages` / `rows` 用 `useMemo`；`useCoarsePointer` 在 `MessageList` 调用一次，经 props 下传给 `MessageItem`。

## 6. 验证策略

- 静态门禁：`pnpm typecheck`、`pnpm lint`、`pnpm test`（web / components / mobile-capabilities）、`pnpm build:mobile`
- 原生门禁：`node scripts/mobile-bind-proxyrules.mjs --target=all`、`swiftc -typecheck`（iOS 改动）、bridge 注册门禁（本次扩展）
- 单测：新增/修改的 hook、selector、缓存、弹窗交互补 vitest；Android 前台服务与 iOS 插件注册以代码审查 + 编译门禁为主
- 真机走查：窄屏（360×780）与横屏（≥768px）下登录页、通知弹窗、创建/编辑房间、设置弹窗、房间列表、语音房；iOS 服务器配置、Android 后台语音与锁屏控制

## 7. 风险与取舍

1. **Push 端到端不可用**：后端无 APNs/FCM，本次只交付前端接线 + 明确降级；不写入"推送已可用"类文档
2. **Android 前台服务需真机验证**：本机无 Android SDK/NDK/JDK，仅做编译门禁；后台麦克风行为需 CI `build-mobile` 产物 + 真机 PoC 确认
3. **房间列表虚拟化改变 DOM 结构**：需回归双击入房、「…」菜单、成员展开、滚动位置保持
4. **原生插件注册门禁扩展**：静态文本断言可能与未来重构冲突，失败信息须明确指出文件与缺失插件名

## 8. 规格自检

- 占位符扫描：无 TODO / 待定
- 内部一致性：范围表 14 项与 §3-§5 逐项对应
- 范围检查：14 项可按一个实现计划覆盖，但建议按原生 / UI / 性能三阶段推进，便于分批验证
- 模糊性检查：Push 后端边界、Android 真机验证边界已明确
