# jobs 模块

后台任务：JetStream 队列消费者 + 周期扫描器。

## 文件

| 文件 | 职责 |
|------|------|
| `handlers.go` | `Handle(job bus.JobEnvelope, deps Deps) error` 按 `job.Type` 分发；`Deps{Cleaner, Chat, Private}` |
| `chat.go` | `ChatPersister` 接口（`PersistFromJob` / `MutateFromJob`），由 `MessageService` 实现 |
| `mute_expiry.go` | `StartMuteExpiryScanner(ctx, scan, interval)` 周期调用过期禁言清理，返回 `stop` |

## job.Type 取值

`livekit`（= `sfu.DefaultProviderName`，LiveKit webhook 事件）、`sfu_cleanup`、`chat.persist`、`chat.mutate`、`chat.private.persist`。未知类型只记日志并忽略。

## 约束

- handler 必须**幂等**：队列可能重投。
- `deps` 里的能力为 nil 时应安全跳过（现有实现即如此），不要 panic。
- 通过接口依赖 service（`ChatPersister`/`SFUCleaner`），不要直接 import service 包，避免循环依赖。
- 禁言到期扫描只周期性调用 `scan()` 回调；到期广播由 `MuteService` 发布内部事件 `EventMuteExpired`，signal Hub 订阅后执行完整 unmute（广播用户级与成员级解禁事件 + SFU 恢复），不要在扫描器里写业务判断。
