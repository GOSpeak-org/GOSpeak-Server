# 语音房共享画面：多人分享者切换条 + 原生控件全屏

> 【状态】2026-11-27 核验：该设计已落地（多人切换条与原生控件已交付），仅供历史参考。

日期：2026-11-21
分支：dev-1120（工作区已有 `sidebar.tsx` 未提交改动，本变更不触碰该文件）

## 背景与目标

当前 `RoomDetail` 的主区共享逻辑在 `app/web/src/components/room/roomDetail.tsx`：

- `sharedVideo = remoteVideos.find(v => v.source === "screen") ?? remoteVideos[0]` —— 只取一个，多人同时共享时其余人被静默忽略；
- 有共享画面时整块替换成员卡片网格，没有切换入口；
- `ScreenShareView` 的 `<video>` 未开 `controls`，自定义了一个右下角全屏按钮，且点画面也能切全屏。

目标：① 多人共享时可逐个切换主画面；② 全屏改走浏览器原生控件，去掉自定义按钮与点画面全屏；③ 顺带修正「摄像头画面被当成屏幕共享展示」的错误。

## 非目标

- 不引入第三方播放器（video.js / Plyr / Media Chrome 等）。当前画面是 `track.attach()` 挂上来的 MediaStream 实时流，非文件/URL 源，播放器生态不适配。
- 不改 `screenShareStore.ts`（同一 identity 摄像头与屏幕互相覆盖是既有行为，本次不处理）。
- 不做共享画面缩略图预览；切换条只展示头像 + 名字。
- 不处理「画面套画面」的屏幕回授（用户共享了正在显示本站的屏幕，属浏览器分享方式问题）。
- 不触碰工作区已有的 `sidebar.tsx` 改动。

## 设计

### A. 候选列表（派生，不增 store 字段）

在 `roomDetail.tsx` 内由现有 store 值派生 `shareEntries`：

- 远端：`remoteVideos.filter(v => v.source === "screen")`，只认屏幕共享，摄像头不再进入主区；
- 本地：`localSharing && localTrack` 时把「我」作为一项加入，使本人也能被切到；
- 每项形状 `{ key: "remote:<identity>" | "local", identity, track, isLocal }`，key 用前缀避免本地/远端同名冲突；
- 用 `useMemo` 缓存，依赖 `remoteVideos / localSharing / localTrack / myName`。

### B. 选中状态与回退

- 新增 `activeKey: string | null`，点击切换条时设置；
- `activeEntry = shareEntries.find(e => e.key === activeKey) ?? shareEntries[0] ?? null`；
- 当前共享者离开时 `activeKey` 自然失效，回退到列表第一项，**不需要 effect 同步**；
- `sharedTrack = activeEntry?.track ?? null`；
- 标签文案：`activeEntry.isLocal ? "你正在共享屏幕" : `${activeEntry.identity} 正在共享屏幕``。

### C. 主区布局

`sharedTrack` 存在时，外层从 `items-center justify-center` 改为纵向 flex：

```
┌──────────────────────────────┐
│ [标签: XX 正在共享屏幕]         │  左上，保持现有样式
│                              │
│        共享画面 <video>        │  flex-1，ScreenShareView，开 controls
│                              │
├──────────────────────────────┤
│ [头像 名字] [头像 名字] ...     │  仅 shareEntries.length > 1 时渲染
└──────────────────────────────┘
```

- `ScreenShareView` 的 `<video>` 加 `controls` 属性；
- 切换条位于视频容器下方，不覆盖画面、不与原生控件栏重叠；
- 每项为 `<button>`：复用 `components/common/avatar` 的 `Avatar`，头像与显示名按 identity 从 `roomMembers` 取，本人取 `userStore.user`；
- 当前项加 `ring-primary` 高亮；
- `shareEntries.length <= 1` 时不渲染切换条。

### D. 全屏（改为原生）

- 删除 `roomDetail.tsx` 中的 `shareVideoRef`、`shareFullscreen` state、`fullscreenchange` 监听、`toggleShareFullscreen`；
- 不再向 `ScreenShareView` 传 `ref` / `onClick` / `fullscreen` / `onToggleFullscreen`；
- `ScreenShareView.tsx` 中注释掉右下角自定义按钮那段 JSX（含 `Maximize2 / Minimize2` 的 import 与 `fullscreen / onToggleFullscreen` 的渲染分支；实现时取改动面最小的做法，可从 props 签名一并移除）；
- 全屏入口只保留 `<video controls>` 的原生按钮。

### E. 边界情况

| 情况 | 行为 |
| --- | --- |
| 新人开始共享 | 不自动抢焦点，切换条多一项，用户主动点选 |
| 当前共享者停止 | `activeKey` 失效，自动回退到列表第一项 |
| 只有摄像头画面、无屏幕共享 | `shareEntries` 为空 → 回落到成员卡片网格 |
| 只有本人共享 | 列表一项 → 无切换条，主画面显示本人共享 |
| 无任何共享 | 保持现有成员卡片网格 |

## 测试

- `app/web/src/components/room/roomDetail.test.tsx`：
  - 单人共享不渲染切换条；
  - 多人共享渲染头像 + 名字按钮，点击后主画面切到对应 track；
  - 当前共享者离开后回退到第一项；
  - 只有摄像头 track 时回落到成员卡片网格。
- 现有 `ScreenShareView.test.tsx` 若因注释按钮导致断言失败，同步调整那些断言（仅测按钮的用例改为「不渲染自定义按钮」）。

## 验证

```bash
pnpm --filter @gospeak/web typecheck
pnpm --filter @gospeak/web test
pnpm --filter @gospeak/web build
pnpm --filter @gospeak/web build:desktop   # 桌面目标条件编译
```

浏览器手测：两人同时共享 → 切换条出现并可切换主画面；原生控件全屏可用；共享者退出后正常回退；单人共享时无切换条。