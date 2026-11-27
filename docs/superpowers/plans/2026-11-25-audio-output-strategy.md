# 音频输出策略抽象与 Web Audio 优先 fallback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在纯 Web 目标引入 Web Audio 优先、HTMLMediaElement fallback 的 per-identity 播放会话；桌面与移动目标维持现有媒体元素/原生路由语义，同时保留现有音量、静音和尽力而为的输出设备切换行为。

**Architecture:** 把当前 `handler_audio/index.ts` 中直接 `track.attach()` 的逻辑抽成 per-identity 播放会话。会话优先尝试 Web Audio（`MediaStreamAudioSourceNode -> GainNode -> destination`）；任一 track 的 Web Audio 初始化失败、底层 `MediaStreamTrack` 缺失/ended、或运行目标不是纯 Web 时，立即 fallback 到现有 `track.attach()` 媒体元素路径。`sfu-client` 只新增可选 `getMediaStreamTrack()`，由 LiveKit 适配器返回 `track.mediaStreamTrack`，不泄漏 provider 类型。用户选择非默认输出设备或 WebView/移动目标下强制/保持 media element，避免 Web Audio 路线承诺无法兑现的 `setSinkId()`。

**Tech Stack:** React 19 · TypeScript · Vitest 5 · Web Audio API (`AudioContext`, `MediaStreamAudioSourceNode`, `GainNode`) · LiveKit client 2.24.1 · pnpm/Turbo · Biome · `inDesktop()` / `inMobile()` 条件编译判定

**Spec:** 用户确认的需求：Web Audio 优先，失败 fallback 到 HTMLMediaElement；允许不改 Go 后端、不新增 SFU provider；不改变移动端 AudioSession/AudioRoute 策略。多 agent review 结论：非颠覆性，重大修订后实施。

## Global Constraints

- 全程遵守根 `AGENTS.md`、`app/web/AGENTS.md`、`app/web/src/AGENTS.md`、`packages/sfu-client/AGENTS.md`。
- 不改 Go 后端；不新增 SFU provider；不新增第三方依赖。
- 只改前端 workspace：`packages/sfu-client` 与 `app/web`。
- 用户标识继续使用 identity 字符串，不引入数字 user id；协议事件名仍只从 `@/socket/events` 取。
- 纯 Web 目标（`__GOSPEAK_TARGET__ === "web"` 且非 `inDesktop()` / `inMobile()`）才允许 Web Audio 优先；桌面 WebView 与移动目标保持 media element 路径，移动端继续走现有 `useMobileAudioRoute()` / AudioSession 语义。
- 当 `preferredSinkId` 非默认（非空字符串）时，强制 media element 路径，保证 `setSinkId()` 承诺有效。
- Web Audio 会话必须按 identity 管理；每路 track 独立决定实际后端，禁止全局单例静态选择导致某路失败后音量/静音打到错误后端。
- Web Audio 路线不调用 `setSinkId()`；媒体元素路径在 `start()` 与 `setAudioOutputDevice()` 时都要应用当前 `preferredSinkId`。
- 音量统一在 handler 层归一化到 0–1 有限值：`masterMuted ? 0 : clampVolume(masterVolume) * clampVolume(memberVolume)`；backend 只接受 0–1 有限值。
- GainNode 音量变化使用 `setTargetAtTime(target, ctx.currentTime, 0.01)` 平滑，不直接写 `value`，避免爆音；Web Audio 路径不调用 LiveKit `track.setVolume()`，避免双重缩放。
- 除复杂逻辑外不新增注释；新增注释必须为中文并说明 why，不写 emoji。
- Git：从最新 `origin/release` 签出短期分支 `feature/audio-output-strategy`；每个 Task incremental commit；PR 合入目标默认 `release`；push 前 lint/typecheck/build/hook-deps 全过。

## Review Focus

- **非 Web 目标不得进入 Web Audio**：desktop/mobile 必须创建 media element 会话，且不新建 AudioContext。测试见 Task 3 Step 1–4、Task 4 Step 5–8。
- **Web Audio 初始化/连接失败要逐 track fallback**：`createMediaStreamSource` 或 `connect` 抛错时必须启动 media element、断开半连接节点，后续音量/静音作用于实际 backend。测试见 Task 2 Step 1–5、Task 4 Step 5–8。
- **getMediaStreamTrack 缺失/track ended/同 identity 新 track**：必须 fallback 或 stop/start 重建，不留下旧 source 静默。测试见 Task 2 Step 1–5、Task 4 Step 5–8。
- **输出设备切换语义**：选择非默认设备或切换设备时，活跃媒体元素必须重新 `setSinkId()`；Web Audio 路径下 UI 语义不承诺设备切换。测试见 Task 3 Step 1–4、Task 4 Step 5–8。
- **resume 必须在用户手势同步栈调用**：加入房间 click handler 中调用 `resumeAudioBackend()`；effect 只作兜底，不作为唯一恢复机制。测试见 Task 5 Step 1–6。
- **音量/静音语义一致**：master/per-member/server mute 对实际 backend 调用 `setVolume`，调用次数与最终值都断言；非有限/越界输入归一化。测试见 Task 2 Step 1–5、Task 4 Step 5–8。
- **清理不泄漏**：成员移除、切房、重复 subscribe、同一 identity 新 track 都断开节点/移除元素；`cleanupAudioHandler()` 对所有会话调用 stop/dispose。测试见 Task 2–4。

---

### Git 前置（所有 Task 共享，不单独提交）

- [ ] 在执行 Task 1 前运行：

```bash
git fetch origin
git switch -c feature/audio-output-strategy origin/release
```

确认工作区干净；后续所有 Task 在该短期分支上 incremental commit，最终 PR 合入 `release`。

### Task 1: sfu-client 暴露远端音频 MediaStreamTrack

**Files:**
- Modify: `packages/sfu-client/src/types.ts:13-18`
- Modify: `packages/sfu-client/src/livekit-client.ts:182-186`
- Modify: `packages/sfu-client/src/livekit-client.ts:481-495`
- Test: `packages/sfu-client/src/livekit-client.video-subscription.test.ts`

**Interfaces:**
- Consumes: LiveKit 2.24.1 `Track.mediaStreamTrack`（`Track.d.ts:53`）。
- Produces: `RemoteAudioTrackLike.getMediaStreamTrack?(): MediaStreamTrack | null`；返回当前时间点轨道快照，调用方不得跨订阅生命周期缓存。

- [ ] **Step 1: 在已有测试文件中写失败测试**

扩展 `livekit-client.video-subscription.test.ts` 现有 fake RoomEvent mock：audio track 的 `mediaStreamTrack` 为指定 fake `MediaStreamTrack`；触发 `room.emit("trackSubscribed", ...)` 后断言 `info.track.getMediaStreamTrack?.()` 返回该对象。不创建新测试文件。

- [ ] **Step 2: 运行失败测试**

Run: `pnpm --filter @gospeak/sfu-client exec vitest run src/livekit-client.video-subscription.test.ts`
Expected: FAIL，提示 `getMediaStreamTrack` 不存在或返回不符。

- [ ] **Step 3: 实现契约与适配**

在 `RemoteAudioTrackLike` 增加可选方法。TrackSubscribed audio 分支与 `getExistingRemoteAudioTracks()` audio push 两处传入对象实现该方法，从 `track.mediaStreamTrack` 读取，异常或缺失返回 `null`。不导出 LiveKit 类型，不改 video 分支。

- [ ] **Step 4: 运行测试通过**

Run: `pnpm --filter @gospeak/sfu-client exec vitest run src/livekit-client.video-subscription.test.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add packages/sfu-client/src/types.ts packages/sfu-client/src/livekit-client.ts packages/sfu-client/src/livekit-client.video-subscription.test.ts
git commit -m "feat(sfu-client): expose remote audio media stream track"
```

### Task 2: 新建播放会话接口与 Web Audio 后端

**Files:**
- Create: `app/web/src/handler_audio/backends/types.ts`
- Create: `app/web/src/handler_audio/backends/webaudio.ts`
- Test: `app/web/src/handler_audio/backends/webaudio.test.ts`

**Interfaces:**
- Consumes: `RemoteAudioTrackLike.getMediaStreamTrack?()`（Task 1）。
- Produces:
  - `PlaybackSession`：`{ kind: "webaudio" | "media-element"; identity: string; setVolume(volume: number): void; stop(): void }`。
  - `AudioPlaybackBackend`：`canUse(): boolean`, `start(identity: string, track: RemoteAudioTrackLike, volume: number): Promise<PlaybackSession | null>`, `resume(): Promise<void>`, `dispose(): void`。
  - `createWebAudioBackend(): AudioPlaybackBackend`：内部懒构造独立 AudioContext；构造失败时 `canUse()` 返回 false，`start()` 返回 null。

- [ ] **Step 1: 写失败测试，覆盖 fake 环境与全部失败路径**

在 `webaudio.test.ts` 中用 `vi.stubGlobal("AudioContext", FakeAudioContext)`、`vi.stubGlobal("MediaStreamTrack", class {})` 构造环境；每个 `afterEach` 调用 `vi.unstubAllGlobals()` 并通过 `vi.resetModules()` 重新导入模块，重置模块状态（不在生产模块导出测试重置函数）。断言：
1. `new AudioContext()` 抛错时 `canUse()` 为 false，`start()` 返回 null。
2. `getMediaStreamTrack` 返回有效 track 时，`start()` 返回 `kind === "webaudio"` 的 session；`createMediaStreamSource` 收到该 track；`gain.gain.setTargetAtTime(0.4, currentTime, 0.01)` 被调用；source/gain 连接顺序为 source -> gain -> destination。
3. `createMediaStreamSource` 抛错、`source.connect` 抛错、`gain.connect` 抛错时，`start()` 返回 null，并调用已创建节点的 `disconnect()`；无半连接残留。
4. `session.setVolume(0.75)` 调用 `setTargetAtTime(0.75, ...)`；非有限或越界输入在调用前由 handler 归一化，backend 只收到 0–1。
5. `ctx.state === "suspended"` 时 `resume()` 调用 `ctx.resume()`；reject 时 `resume()` 不抛。
6. `session.stop()` 调用 source/gain `disconnect()`；同 identity 再次 start 可创建全新 session。
7. `dispose()` 关闭该 backend 自己持有的 AudioContext，不影响其他模块 context。

- [ ] **Step 2: 运行失败测试**

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/backends/webaudio.test.ts`
Expected: FAIL，提示模块不存在或接口未实现。

- [ ] **Step 3: 实现 Web Audio backend**

在 `types.ts` 定义上述接口。在 `webaudio.ts` 实现：懒构造独立 `AudioContext`；`start` 捕获所有创建/连接异常，失败时清理已建节点并返回 null；成功时保存 track 引用并注册 `ended` 监听，`ended` 时调用 session.stop()；`setVolume` 用 `setTargetAtTime(..., 0.01)`；`stop` 断开节点并移除监听；`resume` 吞掉 reject；`dispose` 停止所有 session 并关闭自身 context。

- [ ] **Step 4: 运行测试通过**

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/backends/webaudio.test.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add app/web/src/handler_audio/backends/types.ts app/web/src/handler_audio/backends/webaudio.ts app/web/src/handler_audio/backends/webaudio.test.ts
git commit -m "feat(web): add web audio playback session backend"
```

### Task 3: 新建媒体元素 fallback 后端

**Files:**
- Create: `app/web/src/handler_audio/backends/media-element.ts`
- Test: `app/web/src/handler_audio/backends/media-element.test.ts`

**Interfaces:**
- Consumes: `PlaybackSession` / `AudioPlaybackBackend`（Task 2）。
- Produces: `createMediaElementBackend(options: { applySinkId: (el: HTMLMediaElement) => void; shouldUse: () => boolean }): AudioPlaybackBackend`；`shouldUse()` 由 handler 传入，包含目标设备与 preferredSinkId 判定。

- [ ] **Step 1: 写失败测试，覆盖 attach/applySinkId/detach/动态设备切换**

在 `media-element.test.ts` 断言：
1. `shouldUse()` 为 true 时 `canUse()` 为 true；false 时 `start()` 返回 null。
2. `start("alice", track, 0.4)` 调用 `track.attach()`，元素 `autoplay === true`、`style.display === "none"`、追加到 `document.body`，`options.applySinkId(el)` 被调用，`track.setVolume(0.4)` 被调用，`el.play?.()` 被调用且 reject 被吞掉。
3. `session.setVolume(0.6)` 调用 `track.setVolume(0.6)`。
4. `stop()` 从 DOM 移除元素、调用 `track.detach()`；后续 setVolume 不抛。
5. handler 调用 backend 的 `reapplySinkId()`（本 Task 暴露）后，对所有活跃 session 的 element 再次调用 `applySinkId`。
6. `resume()` 空实现不抛；`dispose()` 清理所有 session。

- [ ] **Step 2: 运行失败测试**

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/backends/media-element.test.ts`
Expected: FAIL，提示模块不存在或接口未实现。

- [ ] **Step 3: 实现媒体元素 backend**

按 identity 保存 `{ track, el }`；`canUse()` 调 `shouldUse()`；`start` 先 stop 同 identity，再 attach/append/applySinkId/setVolume/play；`reapplySinkId()` 遍历 sessions 调 `applySinkId`；`setVolume` 调 `track.setVolume`；`stop` detach 并移除 DOM；`dispose` 清理全部。

- [ ] **Step 4: 运行测试通过**

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/backends/media-element.test.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add app/web/src/handler_audio/backends/media-element.ts app/web/src/handler_audio/backends/media-element.test.ts
git commit -m "feat(web): add media element fallback backend"
```

### Task 4: 改造 handler_audio 为 per-identity 会话策略

**Files:**
- Modify: `app/web/src/handler_audio/index.ts`
- Modify: `app/web/src/handler_audio/index.test.ts`
- Create: `app/web/src/handler_audio/backends/select.ts`
- Test: `app/web/src/handler_audio/backends/select.test.ts`

**Interfaces:**
- Consumes: `createWebAudioBackend()`、`createMediaElementBackend(options)`、`PlaybackSession`（Task 2–3）。
- Produces: `selectBackendForTrack(deps, track): Promise<PlaybackSession | null>`：优先 Web Audio `start()`，失败则 media element `start()`；非 Web 目标或 `preferredSinkId` 非空时直接 media element。handler 现有导出 API 名称签名保持不变，新增 `resumeAudioBackend()`。

- [ ] **Step 1: 写失败测试：后端选择顺序与逐 track fallback**

在 `select.test.ts` 断言：
1. 目标为 web 且无 preferredSinkId、Web Audio `canUse()` 与 `start()` 成功时返回 webaudio session，media element start 未被调用。
2. Web Audio `start()` 返回 null 时调用 media element start 并返回 media-element session。
3. `inDesktop()`/`inMobile()` 为真或 preferredSinkId 非空时，不调用 Web Audio start，直接返回 media-element session。
4. `getMediaStreamTrack` 缺失时 Web Audio start 返回 null，走 media element。

- [ ] **Step 2: 运行失败测试**

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/backends/select.test.ts`
Expected: FAIL，提示模块不存在或函数未导出。

- [ ] **Step 3: 实现 select 函数**

在 `select.ts` 实现按条件选择；所有 backend 调用异常都吞掉并尝试 fallback；不调用音量方法。

- [ ] **Step 4: 运行测试通过**

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/backends/select.test.ts`
Expected: PASS。

- [ ] **Step 5: 改造 handler 测试并先运行失败**

修改 `index.test.ts`：通过 `vi.mock` 注入 fake backends 与 fake `inDesktop/inMobile`，断言：
1. `setupAudioHandler(client)` 后每个 `onRemoteAudioTrack` 都调用 `selectBackendForTrack`，同 identity 同 track 重复事件不重复建 session。
2. 同 identity 新 track 到达时先 stop 旧 session 再 start 新 session。
3. `setVolumeByIdentity(0.5)`、`setMasterMuted(true)`、`setServerMutedByIdentity(true/false)`、`setMutedByIdentity(true/false)` 均对当前 identity 的实际 session 调用 `setVolume`，断言调用次数与最终值；公式遵循 Global Constraints。
4. `setAudioOutputDevice("speaker-a")` 保存 preferredSinkId，并对所有 media-element session 调用 `reapplySinkId()`；Web Audio session 不调用 setSinkId。
5. `onRemoteAudioTrackRemoved` 调 session.stop() 并清理 volume/mute/serverMute 状态。
6. `getExistingRemoteAudioTracks()` 补齐路径走同一 session 创建与音量应用。
7. `cleanupAudioHandler()` 停止所有 session，重置 master/volume/mute 状态，`resumeAudioBackend()` 转发 backend.resume() 并吞错。

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/index.test.ts`
Expected: FAIL，指出尚未接入 per-identity session。

- [ ] **Step 6: 改造 handler 主逻辑**

用 `Map<identity, PlaybackSession>` 替代 `audioElements`。`onTrackSubscribed` 先 stop 同 identity 旧 session，再 `await selectBackendForTrack()`，成功后保存 session 并 `setVolume(effectiveVolume(identity))`；失败则 Toast 一次并跳过该 track。所有音量/静音 setter 改为对 sessions 调用 `setVolume`。`setAudioOutputDevice` 保存 preferredSinkId 并调用 media-element backend 的 `reapplySinkId()`。`setupAudioHandler` 绑定 client 回调并用 `getExistingRemoteAudioTracks()` 补齐。`cleanupAudioHandler` 停止全部 session 并重置状态。

- [ ] **Step 7: 运行测试通过**

Run: `pnpm --filter @gospeak/web exec vitest run src/handler_audio/index.test.ts src/handler_audio/backends/select.test.ts`
Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add app/web/src/handler_audio/index.ts app/web/src/handler_audio/index.test.ts app/web/src/handler_audio/backends/select.ts app/web/src/handler_audio/backends/select.test.ts
git commit -m "feat(web): use per-identity audio playback sessions"
```

### Task 5: 用户手势 resume、门禁与三端构建验证

**Files:**
- Modify: `app/web/src/handler_audio/index.ts`
- Modify: `app/web/src/components/room/hooks/useRoomAudioBridge.ts`
- Test: `app/web/src/components/room/hooks/useRoomAudioBridge.test.tsx`（Create）

**Interfaces:**
- Consumes: `resumeAudioBackend(): Promise<void>`（Task 4）。
- Produces: 在加入房间 click handler 中同步调用 `resumeAudioBackend()`；`useEffect` 中保留一次兜底调用。

- [ ] **Step 1: 写失败测试：click 同步 resume，effect 兜底，失败不中断**

新建 `useRoomAudioBridge.test.tsx`，mock `resumeAudioBackend`。断言：加入房间 click 触发时同步调用一次；joined effect 再调用一次兜底；mock reject 后不抛、不产生未处理 rejection。

- [ ] **Step 2: 运行失败测试**

Run: `pnpm --filter @gospeak/web exec vitest run src/components/room/hooks/useRoomAudioBridge.test.tsx src/handler_audio/index.test.ts`
Expected: FAIL，提示 resume 未被点击链路调用或测试文件缺失。

- [ ] **Step 3: 实现 resume 接入**

在 `useRoomAudioBridge.ts` 暴露或在现有入房 click handler 中同步调用 `void resumeAudioBackend()`；joined effect 中保留兜底 `void resumeAudioBackend()`。不改变现有麦克风 setMicEnabled 逻辑。

- [ ] **Step 4: 运行测试通过**

Run: `pnpm --filter @gospeak/web exec vitest run src/components/room/hooks/useRoomAudioBridge.test.tsx src/handler_audio/index.test.ts`
Expected: PASS。

- [ ] **Step 5: 全量门禁**

Run:
```bash
pnpm --filter @gospeak/sfu-client typecheck && pnpm --filter @gospeak/web typecheck
pnpm --filter @gospeak/sfu-client test && pnpm --filter @gospeak/web test
pnpm check && pnpm lint
node scripts/check-hook-deps.mjs
```
Expected: PASS。hook deps 基线不得新增告警；如新增必须删除依赖或写带理由的 `biome-ignore`。

- [ ] **Step 6: 三端构建验证**

Run: `pnpm --filter @gospeak/web build && pnpm --filter @gospeak/web build:desktop && pnpm --filter @gospeak/web build:mobile`
Expected: PASS。桌面/移动构建只验证编译与裁剪，不替代真机/WebView 音频设备验证。

- [ ] **Step 7: 提交**

```bash
git add app/web/src/handler_audio/index.ts app/web/src/components/room/hooks/useRoomAudioBridge.ts app/web/src/components/room/hooks/useRoomAudioBridge.test.tsx app/web/src/handler_audio/index.test.ts
git commit -m "feat(web): resume audio backend from join gesture"
```

### Task 6: 计划索引状态校正与交付记录

**Files:**
- Modify: `docs/superpowers/plans/README.md:20`

**Interfaces:**
- Consumes: 本计划文件。
- Produces: 索引条目保持存在且恰好匹配一次；交付后状态改为「已交付」或按索引规则移除。

- [ ] **Step 1: 校正索引**

README 已包含本计划行。执行时只核对状态与剩余描述，不新增重复行。全部 Task 完成后，将状态改为「已交付」并按索引规则保留或移除。

- [ ] **Step 2: 校验唯一性与链接**

Run: `test -f docs/superpowers/plans/2026-11-25-audio-output-strategy.md && test "$(grep -c 'audio-output-strategy' docs/superpowers/plans/README.md)" -eq 1`
Expected: 退出码 0。

- [ ] **Step 3: 提交索引变更（如有）**

```bash
git add docs/superpowers/plans/README.md
git commit -m "docs: update audio output strategy plan status"
```

## Self-Review

- 覆盖用户确认的方向：Web Audio 优先、失败 per-identity fallback 到 media element；保留不改 Go、不新增 provider 约束。
- 吸收四份 review 的重大问题：平台隔离、逐 track fallback、`setSinkId` 策略、用户手势 resume、测试文件/命令、Git 前置、hook deps、索引状态。
- 类型一致性：`PlaybackSession`、`AudioPlaybackBackend`、`selectBackendForTrack`、`resumeAudioBackend` 在 Task 2–5 同名同签名；`getMediaStreamTrack` 在 Task 1–4 一致。
- Review Focus 每项都有对应测试任务；真机/WebView 输出设备效果明确列为未覆盖验收项，不伪装成单测能证明。
- 未覆盖事项：仅验证构建与单测，不承诺真机声卡/蓝牙/WebView 实际输出；移动端系统路由语义由现有 `useMobileAudioRoute()` 继续负责。