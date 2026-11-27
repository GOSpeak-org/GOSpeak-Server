# ws 模块

WebSocket 基础设施：HTTP→WS 升级、鉴权、连接生命周期、扇出广播、ACK。**业务事件不在这里定义**（见 `internal/protocol`）。

## 文件

| 文件 | 职责 |
|------|------|
| `types.go` | 线路格式：`Message{id,event,data}`、`ACK{id,event,data,error}`、`ACKError` |
| `upgrader.go` | 升级、Origin 白名单、cookie 鉴权、`ConnectPolicy` 钩子、`OnConnect`/`OnDisconnect` |
| `client.go` | `Client`：状态机、读/写/心跳循环、`Send`、`SendACK`、`SendErrorACK(AndClose)` |
| `handler.go` | `HandlerRegistry`：`Handle`/`HandleAck`/`Dispatch`、`SetPolicyGuard` |
| `fanout.go` | `Fanout`：房间/命名空间广播、`DroppedCount` |
| `broadcaster.go` | `Broadcaster` 接口（Hub 依赖的最小面，便于 mock） |
| `messenger.go` | `ClientMessenger` 接口 |
| `upgrader_policy_test.go` / `handler_policy_test.go` | 策略回归测试 |

## 生命周期与状态

连接经历 `ConnState` 状态机，非法迁移会被拒绝（`validConnStateTransition`）。读写分离：单读循环 + 单写循环 + 心跳，写操作序列化在 `sendRaw`。

## ACK 语义

- 客户端请求带 `id` → 必须应答（`SendACK` 成功 / `SendErrorACK` 业务错误 / `SendErrorACKAndClose` 致命错误随后断开）。
- 服务端推送不带 `id`，不应答。

## 策略

`/ws` 绕过 Gin 中间件链，因此升级前必须走 `ConnectPolicy`（与 HTTP 的 `BanCheck` 共用 `policy.Engine`），每条上行消息再经 `HandlerRegistry.PolicyGuard` 实时判决。**不要在这里另写鉴权逻辑**。

## 约束

- Origin：`AllowedOrigins` 为空时只允许与请求 Host 同源；含 `*` 才放开。生产不要留 `*`。
- 广播房间名必须是复合键（`pkg.RoomKey`），不要拼裸字符串。
- 上下行业务事件名一律来自 `protocol` 常量。
- 心跳/超时/队列容量改动会影响所有客户端，改前先看 `client_test.go` 的行为断言。
