# 屏幕共享按需订阅 Implementation Plan

> 【状态】2026-11-27 核验：该计划已交付（6 Task 全部合入 client release），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 屏幕共享不再"有人共享就自动播放"：加入房间时关闭远端视频自动订阅，只有成员主动点击正在共享的成员卡片后，才通过 SFU 订阅该成员的视频流并播放；切换/退出时退订旧流。

**Architecture:** 在 `packages/sfu-client` 的 LiveKit 实现中以 `autoSubscribe: false` 入房；`RoomEvent.TrackPublished` 时只自动订阅音频，视频 publication 仅登记为"可订阅"。新增 provider-agnostic 的 `RemoteVideoPublicationInfo` / `setRemoteVideoSubscribed(identity, source, subscribed)` / `onRemoteVideoPublicationChanged` 契约；web 层 `screenShareStore` 记录"可订阅条目"与"已订阅 track"两类状态，点击卡片时调用订阅，订阅成功后的 `TrackSubscribed` 事件才把 track 放入播放列表。`roomDetail` 的成员卡片仅在有共享条目时可点击观看，共享视图提供切换与退出。

**Tech Stack:** React 19 · TypeScript · zustand · LiveKit client 2.24.1 · Vitest 5 · Vitest + @testing-library/react

**Spec:** 用户请求（对话原文）："在共享屏幕时 成员需要点击正在共享屏幕的 成员卡片才进行播放 而不是有人播放就开"；用户已选定方案："SFU 层按需订阅"。

## Global Constraints

- 仅支持 LiveKit（`SFU_PROVIDER=livekit`）；不得引入第二 SFU provider 或恢复多 provider 动态切换叙事。
- 不改 WS 事件协议、不改后端；变更范围限 `packages/sfu-client` 与 `app/web/src/components/room`、`app/web/src/stores`。
- 音频行为不得改变：进房后所有远端音频仍自动订阅，音量/静音链路保持原样。
- 协议单一事实源：不得手写 wire 事件名；本任务不新增 WS 事件，故无生成物变更。
- UI token / 圆角规则遵守 `packages/components/src/theme.css`，使用工具类，不写任意值。
- 全部中文回复；代码内除复杂逻辑不加注释、不用 emoji。
- 从 `origin/release` 签出短期功能分支提交（已建 `feature/screen-share-on-demand-subscribe`）。
- agent 测试记录写入 `agent_test_logs/`（如有需要）。

## Review Focus

- **远端音频在 autoSubscribe=false 后仍必须自动订阅**：否则"听不到人说话"是严重回归 → Task 1 测试 `TrackPublished` 音频 publication 被 `setSubscribed(true)`。
- **无人点击时绝不播放共享**：进房后远端 video publication 未订阅，`onRemoteVideoTrack` 不触发 → Task 2 测试 `TrackPublished` 视频不自动订阅；Task 5 测试渲染时无共享画面。
- **点击卡片后确实发生订阅**：点击后 `setRemoteVideoSubscribed` 被调用且参数正确 → Task 4/Task 5 覆盖。
- **对方停止共享 / 切换观看对象后进退订**：`TrackUnpublished`、`TrackUnsubscribed`、切换 active 时旧流被退订且 store 被清理 → Task 1/Task 2/Task 4 覆盖。
- **共享者进房晚于自己（join 竞态）**：`TrackPublished` 事件要能被捕获并进入"可订阅"列表 → Task 2 测试事件登记；Task 4 测试 store 消费。
- **订阅失败不卡死 UI**：`setRemoteVideoSubscribed` 抛错时 store 状态回退、不进入共享视图 → Task 4 测试失败路径。

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `packages/sfu-client/src/types.ts` | 新增 `RemoteVideoPublicationInfo`、`setRemoteVideoSubscribed` / `onRemoteVideoPublicationChanged` / `getExistingRemoteVideoPublications` 契约 |
| `packages/sfu-client/src/livekit-client.ts` | `autoSubscribe:false` 入房；音频自动订阅；视频 publication 事件桥接；按 identity+source 订阅/退订 |
| `packages/sfu-client/src/livekit-client.video-subscription.test.ts` | SFU 层按需订阅单测 |
| `app/web/src/stores/screenShareStore.ts` | 记录"可订阅条目 + 已订阅 track"两类状态 |
| `app/web/src/stores/screenShareStore.test.ts` | store 状态机单测 |
| `app/web/src/components/room/VoiceSessionProvider.tsx` | 消费 publication 事件、订阅入口、离开清理 |
| `app/web/src/components/room/useWatchScreenShare.ts` | 暴露 `watchScreenShare(identity)` / `stopWatchingScreenShare()` |
| `app/web/src/components/room/roomDetail.tsx` | 成员卡片共享角标、点击订阅、共享视图切换/退出 |
| `app/web/src/components/room/roomDetail.test.tsx` | 点击前不播放 / 点击后订阅 / 共享状态角标 |

---

### Task 1: SFU 契约扩展与 LiveKit 入房改造

**Files:**
- Modify: `packages/sfu-client/src/types.ts`
- Modify: `packages/sfu-client/src/livekit-client.ts`
- Test: `packages/sfu-client/src/livekit-client.video-subscription.test.ts`

**Interfaces:**
- Consumes: 现有 `SFUClient` / `RemoteVideoTrackInfo`（保持不变）。
- Produces:
  - `RemoteVideoPublicationInfo = { identity: string; source: "screen" | "camera" }`
  - `SFUClient.onRemoteVideoPublicationChanged?(cb: (info: RemoteVideoPublicationInfo & { available: boolean }) => void): void`
  - `SFUClient.getExistingRemoteVideoPublications?(): RemoteVideoPublicationInfo[]`
  - `SFUClient.setRemoteVideoSubscribed?(identity: string, source: "screen" | "camera", subscribed: boolean): Promise<void>`

- [ ] **Step 1: 写失败测试：入房 autoSubscribe=false 且音频自动订阅、视频不自动订阅**

在 `packages/sfu-client/src/livekit-client.video-subscription.test.ts` 中参考 `livekit-client.reconnect.test.ts` 的 `vi.mock("livekit-client")` 风格，构造含 `RoomEvent.TrackPublished` 事件的 mock Room；断言：
1. `room.connect` 第三参为 `{ autoSubscribe: false }`
2. 触发 `TrackPublished`，音频 publication 的 `setSubscribed(true)` 被调用
3. 视频 publication 的 `setSubscribed` 不被调用

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/sfu-client test -- livekit-client.video-subscription.test.ts`
Expected: FAIL（方法/参数尚不存在）

- [ ] **Step 3: 实现 SFU 层改造**

在 `types.ts` 增加上述接口成员；在 `livekit-client.ts`：
- `joinRoom` 的 `connect` 调用改为 `this.room.connect(url, token, { autoSubscribe: false })`
- 注册 `RoomEvent.TrackPublished`：音频 publication 立即 `setSubscribed(true)`；视频 publication 记入内部 `remoteVideoPublications` Map 并触发 `onRemoteVideoPublicationChanged({ ...info, available: true })`
- 注册 `RoomEvent.TrackUnpublished`：从 Map 删除并触发 `available: false`
- `getExistingRemoteVideoPublications` 从当前 remote participants 的 videoTrackPublications 构造（不过滤是否 subscribed）
- `setRemoteVideoSubscribed(identity, source, subscribed)`：查 Map 中 publication，调用 `publication.setSubscribed(subscribed)`；未找到时 no-op
- `leaveRoom` / 不可恢复断连清理中重置内部 Map 与回调（按现有回调清理模式追加）

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/sfu-client test -- livekit-client.video-subscription.test.ts`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add packages/sfu-client/src/types.ts packages/sfu-client/src/livekit-client.ts packages/sfu-client/src/livekit-client.video-subscription.test.ts
git commit -m "feat(sfu): subscribe remote video tracks on demand"
```

### Task 2: SFU 层按需订阅/退订行为测试

**Files:**
- Modify: `packages/sfu-client/src/livekit-client.video-subscription.test.ts`

**Interfaces:**
- Consumes: Task 1 产出的 `setRemoteVideoSubscribed` 与 `onRemoteVideoPublicationChanged`。
- Produces: 无新接口。

- [ ] **Step 1: 写失败测试**

扩展 Task 1 的测试文件，断言：
1. `setRemoteVideoSubscribed("bob", "screen", true)` 对 Bob 的屏幕共享 publication 调 `setSubscribed(true)`
2. 同一 identity 的 camera publication 不受影响
3. `setRemoteVideoSubscribed("bob", "screen", false)` 调 `setSubscribed(false)`
4. 触发 `TrackUnpublished`（视频）后，`onRemoteVideoPublicationChanged` 收到 `available: false`
5. `getExistingRemoteVideoPublications()` 返回当前房间已有视频 publication（含未订阅的）

- [ ] **Step 2: 运行测试确认失败或覆盖不足**

Run: `pnpm --filter @gospeak/sfu-client test -- livekit-client.video-subscription.test.ts`
Expected: FAIL 或新增断言未覆盖

- [ ] **Step 3: 补足实现中遗漏的行为**

仅修改 `livekit-client.ts` 中 Task 1 已声明的方法，使新断言通过；不得改契约签名。

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/sfu-client test`
Expected: PASS（含全部既有 sfu-client 测试）

- [ ] **Step 5: 提交**

```bash
git add packages/sfu-client/src/livekit-client.video-subscription.test.ts packages/sfu-client/src/livekit-client.ts
git commit -m "test(sfu): cover video publication subscribe and unsubscribe"
```

### Task 3: screenShareStore 改为"可订阅条目 + 已订阅 track"状态机

**Files:**
- Modify: `app/web/src/stores/screenShareStore.ts`
- Create: `app/web/src/stores/screenShareStore.test.ts`

**Interfaces:**
- Consumes: Task 1 的 `RemoteVideoPublicationInfo` 类型。
- Produces:
  - `ScreenShareState.remotePublications: RemoteVideoPublicationInfo[]`（可订阅的屏幕共享条目，仅 source=screen）
  - `ScreenShareState.remote: RemoteVideoTrackInfo[]`（已订阅并收到 track 的列表）
  - `upsertPublication(info: RemoteVideoPublicationInfo & { available: boolean }): void`
  - `replacePublications(list: RemoteVideoPublicationInfo[]): void`
  - `reset(): void`（保留既有语义并清空 publications）

- [ ] **Step 1: 写失败测试**

新建 `app/web/src/stores/screenShareStore.test.ts`，断言：
1. `replacePublications` 只影响 publications，不改动 `remote`
2. `upsertPublication` 对 `available:false` 删除对应 identity+source 条目
3. `upsertRemote` 保持原有去重/追加语义（identity 唯一）
4. `reset` 同时清空 `remote`、`publications`、`localSharing`、`localTrack`

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/web test -- screenShareStore.test.ts`
Expected: FAIL

- [ ] **Step 3: 实现 store 改造**

在 `screenShareStore.ts` 增加上述状态与 actions；保留 `remote` 现有接口，避免影响其他调用方。

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/web test -- screenShareStore.test.ts`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add app/web/src/stores/screenShareStore.ts app/web/src/stores/screenShareStore.test.ts
git commit -m "feat(web): track subscribable screen share publications"
```

### Task 4: VoiceSessionProvider 桥接 publication 事件与订阅入口

**Files:**
- Modify: `app/web/src/components/room/VoiceSessionProvider.tsx`
- Create: `app/web/src/components/room/useWatchScreenShare.ts`
- Test: `app/web/src/components/room/VoiceSessionProvider.videoSubscription.test.tsx`

**Interfaces:**
- Consumes: Task 1 `SFUClient.setRemoteVideoSubscribed` / `onRemoteVideoPublicationChanged` / `getExistingRemoteVideoPublications`；Task 3 store actions。
- Produces:
  - `useWatchScreenShare(): { watchScreenShare: (identity: string) => Promise<void>; stopWatchingScreenShare: () => Promise<void> }`
  - 内部维护 `activeSubscription: { identity: string; source: "screen" } | null`

- [ ] **Step 1: 先写失败测试（桥接层）**

创建 `VoiceSessionProvider.videoSubscription.test.tsx`，mock `useVoiceSession` 与 sfuClient，断言：
1. 挂载时 `getExistingRemoteVideoPublications` 被消费并写入 store 的 publications
2. `onRemoteVideoPublicationChanged` 事件写入/删除 publications
3. `watchScreenShare("bob")` 调用 `setRemoteVideoSubscribed("bob", "screen", true)`，并退订上一个 active
4. `stopWatchingScreenShare()` 调用 `setRemoteVideoSubscribed` 退订当前 active 并清空
5. 订阅抛错时 active 状态不更新、不进入共享视图（Review Focus 失败路径）
6. 卸载时 `reset` 被调用

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/web test -- VoiceSessionProvider.videoSubscription.test.tsx`
Expected: FAIL

- [ ] **Step 3: 实现桥接与 hook**

在 `VoiceSessionProvider.tsx` 的 sfuClient effect 中：
- 初始化 `store.replacePublications(sfuClient.getExistingRemoteVideoPublications?.() ?? [])`
- 注册 `onRemoteVideoPublicationChanged` → `store.upsertPublication(info)`
- 保留现有 `onRemoteVideoTrack` / `onRemoteVideoTrackRemoved` / `onLocalScreenShareChanged` 行为
- cleanup：退订当前 active、`store.reset()`

新建 `useWatchScreenShare.ts`：用 zustand 之外的最小模块级状态或 React context 暴露稳定函数；实现"退订旧 → 订阅新 → 更新 active"的串行流程，失败时回滚 active。

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/web test -- VoiceSessionProvider.videoSubscription.test.tsx`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add app/web/src/components/room/VoiceSessionProvider.tsx app/web/src/components/room/useWatchScreenShare.ts app/web/src/components/room/VoiceSessionProvider.videoSubscription.test.tsx
git commit -m "feat(web): bridge video publications to screen share store"
```

### Task 5: RoomDetail 成员卡片点击观看与共享视图切换/退出

**Files:**
- Modify: `app/web/src/components/room/roomDetail.tsx`
- Modify: `app/web/src/components/room/roomDetail.test.tsx`

**Interfaces:**
- Consumes: Task 3 `remotePublications`；Task 4 `useWatchScreenShare()` 暴露的函数。
- Produces: 无新接口。

- [ ] **Step 1: 写失败测试**

扩展 `roomDetail.test.tsx`，mock `useWatchScreenShare`，断言：
1. Bob 在 `remotePublications` 中但 `remote` 为空时：成员卡片渲染"正在共享屏幕"角标，主区域仍是成员网格（无共享画面）
2. 点击 Bob 的共享角标：`watchScreenShare("bob")` 被调用一次
3. `activeKey` 指向 Bob 且 `remote` 中有 Bob 的 track 时：渲染共享画面与标签 `Bob 正在共享屏幕`，并显示"退出共享"按钮
4. 点击"退出共享"：`stopWatchingScreenShare()` 被调用，回到成员网格

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/web test -- roomDetail.test.tsx`
Expected: FAIL

- [ ] **Step 3: 实现 UI 改造**

在 `roomDetail.tsx`：
- 从 store 读取 `remotePublications`；成员卡片新增 `sharing` prop，在卡片内渲染共享角标与点击入口（`onClick={() => onWatchShare(member.identity)}`）
- `shareEntries` 来源改为 `remote` 中已订阅的屏幕共享 track（不再从 publications 直接构造播放条目），local 自共享保持原逻辑
- `sharedTrack` 仅来自当前 active entry；`activeKey` 初始 `null`，当 active entry 消失时回落到 `null`
- 共享视图底部切换条保留；新增"退出共享"按钮调用 `stopWatchingScreenShare()`
- 成员网格在有人共享但尚未观看时仍显示所有成员卡片，未被共享的成员卡片无角标

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/web test -- roomDetail.test.tsx`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add app/web/src/components/room/roomDetail.tsx app/web/src/components/room/roomDetail.test.tsx
git commit -m "feat(web): watch screen share only after member card click"
```

### Task 6: 全量验证与收尾

**Files:** 无（仅验证）

- [ ] **Step 1: 运行门禁**

```bash
pnpm --filter @gospeak/sfu-client test
pnpm --filter @gospeak/web test
pnpm typecheck
pnpm lint
```

Expected: 全部 PASS；如有 hook deps 棘轮告警新增则按项目约定修掉，不得新增豁免。

- [ ] **Step 2: 构建验证（含桌面目标，条件编译受影响文件）**

```bash
pnpm build
pnpm --filter @gospeak/web build:desktop
```

Expected: PASS

- [ ] **Step 3: 写 agent 测试记录**

按 `agent_test_logs/AGENTS.md` 命名规范写 `agent_test_logs/屏幕共享按需订阅-<时间>.md`，记录命令与结果。

- [ ] **Step 4: 提交记录**

```bash
git add agent_test_logs/
git commit -m "docs: add screen share on-demand subscribe test log"
```
