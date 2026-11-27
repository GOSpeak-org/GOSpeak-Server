# sfu 模块

SFU 抽象层：provider 无关的接口、能力矩阵、类型。**当前只有 LiveKit 一个实现**。

## 文件

| 文件 | 职责 |
|------|------|
| `provider.go` | `Provider` 接口、`BaseProvider`（不支持方法的零实现）、`DefaultProviderName` |
| `types.go` | `RoomSummary`/`ParticipantSummary`、enforcement 等级、`Capabilities`、等级工具函数 |
| `capabilities.go` | `CapabilitiesFor(provider)` / `AllProviderCapabilities()` |
| `factory/` | provider 注册表与动态解析 |
| `providers/livekit/` | 唯一实现 |

## 能力契约（硬约定）

- 支持的操作返回真实结果；**不支持的操作必须返回 `pkg.NewErrSFUNotSupported()`**，调用方用 `errors.Is` 降级。
- **配置缺失返回 `SFU_NOT_CONFIGURED`(6001)**，不要和「不支持」混用。
- `Capabilities()` 是声明式契约，**必须与真实方法行为一致**。
- 能力方法（`GenerateTokenWithPublish`/`MuteParticipantTimed`/`SupportsStream`/`StreamName`/`StreamInfo`/`ClientInfo`）统一收敛在主接口，调用方直接调用并按 sentinel 降级，**禁止散落类型断言探测**。

## Enforcement 等级

`hard`（原生媒体强制）> `degraded`（替代手段强制）> `soft`（仅信令+客户端配合）> `none`（无能力）。

- `NormalizeLevel` 把未知值归一到 `soft`；`LevelEnabled` 判断是否具备媒体强制力；`EnforcementFromLevel` 给出成功调用后的事件语义。
- 调用失败/不支持时，现场一律降级为 `soft`。
- `PermanentMuteTTLSeconds`（365 天）用于需要 TTL 的媒体规则实现永久禁言。

## 当前能力矩阵

`CapabilitiesFor(DefaultProviderName)` 全项 `hard`；未注册的 provider 名返回全 `none`。矩阵要与 livekit 实现同步维护。

## 约束

- 新增接口方法必须同时给 `BaseProvider` 加零实现，否则所有桩实现都要改。
- 不要在 handler / service 里判断具体 provider 名；统一走接口 + 降级。
- `RoomSummary`/`ParticipantSummary` 走 camelCase JSON，与 HTTP 响应的 snake_case 约定不同（它们是 SFU 侧形状），改字段会波及前端。
