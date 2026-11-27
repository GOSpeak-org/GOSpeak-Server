## LiveKit Provider 实现完整度

**评估时间**: 2026-11-24（对照当前 `server/internal/sfu/` 与 `client/packages/sfu-client/` 源码核实）
**接口**: `sfu.Provider`（`server/internal/sfu/provider.go`）

> 图例：✅ 完整 / ⚠️ 降级 / ❌ 返回 `ErrSFUNotSupported` 或静默空操作
>
> 范围说明：当前唯一 SFU provider 为 LiveKit；历史多 provider（SRS / Cloudflare / mediasoup / Agora）已移除，`SFU_PROVIDER` 仅接受 `livekit`。

### 1. 整体成熟度概览

| Provider | 成熟度 | 接口方法覆盖 | 关键缺口 |
|----------|--------|--------------|----------|
| LiveKit | 完整 | 15/15 ✅ | 无 |

### 2. 方法覆盖矩阵

| 方法 | LiveKit |
|------|---------|
| `ProviderName` | ✅ |
| `Capabilities` | ✅ |
| `GenerateToken` | ✅ |
| `GenerateTokenWithPublish` | ✅ |
| `ListRooms` | ✅ |
| `ListParticipants` | ✅ |
| `MuteParticipant` | ✅ |
| `MuteParticipantTimed` | ✅ |
| `RemoveParticipant` | ✅ |
| `DeleteRoom` | ✅ |
| `GetHost` | ✅ |
| `SupportsStream` | ✅ |
| `StreamName` | ✅ |
| `StreamInfo` | ✅ |
| `ClientInfo` | ✅ |

> 历史 B05 回归已修复：`handler/signal_handler.go` 的 `/signal/participants` 现在通过 `pkg.HandleError` 返回非 0 错误码，监控与前端可区分"房间无人"与"上游故障"；回归测试见 `signal_handler_test.go:TestListParticipants_SFUError`。

### 3. 实现说明

| Provider | 路径 | 关键事实 |
|----------|------|----------|
| LiveKit | `internal/sfu/providers/livekit/` | 全部 ✅；`MuteParticipant` 支持按 trackSid 精确静音或按 identity 批量静音；`ProviderName()` = `livekit` |

### 4. 前端 SFU 客户端（`client/packages/sfu-client`）

LiveKit 客户端已完整实现，无已知标记。通过 `livekit-client.ts` 接入，按房间/参与者订阅媒体与事件。
包内保留 `speaking-meter` / `speaking-detector` / `speaking-meter-worklet`（本地麦克风 AudioWorklet 事件检测），但当前生产链路未调用：LiveKit 使用 SFU 原生 `onActiveSpeakers`。该底座可作为未来"本地发言检测"选项，但不是当前高亮来源。

### 5. 发言检测（Active Speakers）

| Provider | 检测机制 | 链路 | 能力 |
|----------|----------|------|------|
| LiveKit | SFU 原生 `RoomEvent.ActiveSpeakersChanged` | 客户端 `onActiveSpeakers` → 前端高亮 | ✅ 原生 |

**信令聚合链路要点**：
- 成员 `member:speaking` 上报；服务端同值去重后聚合 `room:active-speakers`，仅在状态翻转时广播
- 新成员 `room:join:sfu` 成功后回放当前 active speakers，加入者立即看到正在发言的人（当前仅本实例成员；单实例架构下完整）
- 成员离开 / 断连 / 被踢 / 切房时清发言态并广播最新列表重置高亮
- 前端离开 / 断连 / 切房时清空 `speakingIdentities`，避免陈旧高亮残留

> 历史"跨实例 join 回放合并 KV Speaking""SRS/Cloudflare 本地 meter 事件驱动化""mediasoup 事件驱动化"等计划已随多 provider 移除而作废；相关计划文档已删除（见 `docs/superpowers/plans/README.md`）。

### 6. 关于 Mute 语义的说明

SFU `MuteParticipant` 是 **服务端轨道级 SFU mute**，而非用户禁言。

用户禁言由独立层实现：
- `MuteService` + `MuteHandler` + `Hub.BroadcastMute` / `Hub.BroadcastUnmute`
- WebSocket 事件：`user:muted` / `user:unmuted`（被禁言者本人）；`member:muted` / `member:unmuted`（全员订阅端静音）
- 数据库中持久化禁言记录（以 `user_uuid` 标识用户），含过期自动清理；临时禁言优先调 `MuteParticipantTimed`，到期经内部事件 `mute:expired` 解锁并恢复 SFU 媒体
- 媒体强制经 Hub 尽力执行，事件 `enforcement` 取 `hard` / `degraded` / `soft`（见下节）
- 该层 **已完整实现且功能齐全**

本地播放静音（静音远端音轨音量）是纯客户端行为，与以上两层无关。

### 7. 信令层踢人分发（按 `ErrSFUNotSupported` 优雅降级）

`internal/signal/hub.go` 的 `removeParticipantSafe` 调用 `sfuProvider.RemoveParticipant(room, identity)`，仅当返回 `pkg.ErrSFUNotSupported` 时静默跳过。LiveKit 已实现并被调用。

> 注意 `ErrSFUNotSupported` 契约漂移：LiveKit 从不返回该 sentinel，Hub 中按此 sentinel 静默跳过的分支实际为死代码。当前 LiveKit 已实现 `RemoveParticipant`，正常路径不会触发该分支。

### Enforcement 事件语义

`room:kicked` / `user:muted` / `user:unmuted` 携带 `enforcement`:
- `hard`：原生媒体强制成功
- `degraded`：替代 API 强制成功
- `soft`：仅信令/策略 + 客户端配合
