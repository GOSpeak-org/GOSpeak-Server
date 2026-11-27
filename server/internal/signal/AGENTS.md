# signal 模块

WebSocket 信令中心：`Hub` 维护房间/成员状态，事件 handler 全部走 `internal/protocol` 定义的事件名。

## 文件组织

`hub.go` 是 Hub 主体；其余按职责拆成 `hub_*.go`：

| 文件 | 职责 |
|------|------|
| `hub_lifecycle` `hub_disconnect` `hub_disconnect_grace` | 连接生命周期、断线宽限 |
| `hub_room` `hub_room_join` `hub_room_list` `hub_room_events` `hub_room_meta` | 房间进出与元数据 |
| `hub_mute` `hub_mute_cache` | 禁言状态与缓存 |
| `hub_sfu` `hub_reconcile` | SFU 调用与状态对账 |
| `hub_kickban` `hub_removal` | 踢出/封禁与成员移除 |
| `hub_bus` `state_sync` `state_sync_room` | 跨实例事件与状态同步 |
| `hub_typing` `message_bridge` `private_bridge` | 打字、消息与私聊桥接 |
| `permission` `events` `types` | 权限判定、事件别名、类型 |

## 语义约束

- **信令层确实调用 SFU**：禁言优先调 `MuteParticipantTimed`（临时禁言，`ErrSFUNotSupported` 时降级为普通 `MuteParticipant`），踢出/房间查询会调 `ListRooms`/`ListParticipants`（见 `hub_mute.go`、`hub_sfu.go`）。不要写成「信令层不触发 SFU」。
- 禁言链路还包含媒体强制等级（`hard` / `degraded` / `soft`，见 `sfu.EnforcementFromLevel`）、成员级 `member:muted` / `member:unmuted` 定向广播，以及通过跨实例 KV 扫描做离线解禁自愈；期限由 `PermanentMuteTTLSeconds`（365 天）兜底。
- 对 `ErrSFUNotSupported` 必须降级而非报错；其他 SFU 错误按错误契约上抛。
- 发言检测由 `packages/sfu-client` 的 meter 采集 → `member:speaking` → 服务端聚合 → `room:active-speakers`。
- **事件名不写字符串字面量**，统一用 `protocol.EventXxx`；`events.go` 仅为存量别名文件。
- 房间/成员状态变更要同时处理本地状态与 bus 同步，避免多端不一致。
- 域隔离：房间标识用 `pkg.RoomKey(domainUUID, room)`，不要拼接裸字符串。

## 依赖

只依赖 `protocol` / `ws` / `pkg` / `bus` / `sfu` 抽象与 model 快照，不直接依赖 HTTP 层。
