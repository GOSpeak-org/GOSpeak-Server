# sfu/factory 模块

SFU provider 的注册表与运行时解析。

## 文件

| 文件 | 职责 |
|------|------|
| `factory.go` | `Register` / `RegisteredProviders` / `NewProvider`、内置 `init()` 注册 |
| `dynamic_provider.go` | `DynamicProvider`：`ConfigResolver` 注入、TTL 缓存、热切换 |

## 注册机制

- `Register(name, build)`：空名或重复注册**直接 panic**（编程错误，启动期暴露）。
- `init()` 只注册 `sfu.DefaultProviderName`（即 livekit）。
- `NewProvider(cfg)`：按 `cfg.SFUProvider` 查表；未指定回退默认；未知名字返回错误并列出已注册名单。

## DynamicProvider

- 每 2 秒（`configCacheTTL`）内 `fingerprint` 不变则复用已解析配置，避免热路径反复重建 provider。
- `fingerprint` 覆盖 `SFUProvider` + `LiveKit{Host,PublicURL,Key,Secret}`；**新增 provider 配置字段必须加进这里**，否则改了配置不生效。
- `resolve` 失败时：TTL 内有旧缓存则继续用旧配置（热路径不全挂），否则透传错误。
- 持有旧 provider 时通过 `providerCloser` 关闭，防止资源泄漏。

## 新增 provider 的前置条件（缺一不可）

1. 在 `internal/sfu/capabilities.go` 加能力矩阵。
2. 在本包 `init()` 或显式注册构造器。
3. `config/config_validate.go` 放开 `SFU_PROVIDER` 白名单。
4. `service/sfu_config_service.go` 的硬编码约束同步放开。
5. `fingerprint` 纳入新 provider 的配置字段。

**当前四项均只允许 livekit**；不要只改注册表就宣称支持了第二个后端。
