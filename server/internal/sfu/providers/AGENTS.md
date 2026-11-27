# sfu/providers 模块

SFU provider 实现目录。**当前只有 `livekit/`**（见 `livekit/AGENTS.md`）。

## 约定

- 每个 provider 提供 `NewService(cfg *config.Config) *Service`（返回具体类型，可选实现 `Close() error`）；由 `sfu/factory` 注册时包装成 `sfu.Provider`。
- 无法实现的方法**嵌入 `sfu.BaseProvider`** 自动获得「返回 `ErrSFUNotSupported`」的零实现，只覆写真正支持的方法。
- `Capabilities()` 的声明必须与真实支持度逐项对齐（`server/internal/sfu/capabilities.go`）。
- provider 内部不得依赖 handler / service，只能依赖 `config`、`sfu` 类型与自身 SDK。
- 外部调用失败要归一成 `pkg.NewAppError` 体系（配置问题 → `SFU_NOT_CONFIGURED`，其余 → `SFU_ERROR`）。

## 新增 provider 的完整清单

1. 本目录新建 `<name>/` 实现 `sfu.Provider`。
2. `server/internal/sfu/capabilities.go` 登记能力矩阵。
3. `server/internal/sfu/factory/factory.go` 注册构造器，并把配置字段加入 `fingerprint`。
4. `server/internal/config/config_validate.go` 放开白名单。
5. `server/internal/service/sfu_config_service.go` 去掉「仅 livekit」硬约束。
6. 前端 `packages/sfu-client/src/provider.ts` 的 `SFU_PROVIDERS` 与 `PROVIDER_LABELS` 加项。
7. 补 e2e 与能力降级测试。

> 只改 1–3 而不改 4–6，会导致「代码里有实现但运行时永远启用不了」。
