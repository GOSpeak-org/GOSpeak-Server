# authstate 模块

共享 JWT 认证状态：token 黑名单、refresh family 重放防护、签名密钥轮换。**不依赖 Redis**：多实例状态由可注入的 `Backend`（通常是 `bus.AuthStore`，NATS KV）提供，未注入时退化为进程内内存。

## 文件

| 文件 | 职责 |
|------|------|
| `backend.go` | `Backend` 接口与 `SetBackend` / `BackendName` 注入点 |
| `blacklist.go` | 黑名单、refresh family、内存降级 |
| `jwt_key.go` | 签名密钥：dev 静态键、prod 随机键 + 定期轮换、历史键保留 |

## Backend 接口要点

`BlacklistToken` / `IsBlacklisted`（接口层 fail-open）/ `IsBlacklistedErr`（返回错误，供安全敏感调用方自行选择 fail-open / fail-closed）/ `GetSigningKey` / `SetSigningKey`（create-only）/ `UpdateSigningKey` / `GetCreatedAt` / `AddHistoryKey` / `HistoryKeys` / refresh family 三方法 / `Backend()`。

注意：接口层 `Backend.IsBlacklisted` 是 fail-open（存储报错视为未黑名单），但包级 `authstate.IsBlacklisted` 是 **fail-closed**（后端报错时返回 true，视为已黑名单），安全敏感判定直接用 `IsBlacklistedErr`。

## 签名密钥策略

- **开发**用静态 `JWT_KEY`：内嵌 NATS 重启不换键，避免所有人被踢下线。
- **生产**用随机键，首启写入 Backend；并发首启输掉的一方重新读取，不覆盖。
- 轮换由 `StartKeyRotationLoop` 驱动；`ShouldRotateKey` 在后端故障时**跳过本轮**（重复轮换会挤掉历史键、踢掉在线会话）。
- 历史键保留 `histTTL`（7 天），供 `GetAllSigningKeys` 验签过渡期 token。

## 约束

- 改密 / 角色变更通过 `User.TokenVersion` 失效旧 token，不要另造机制。
- 安全敏感的判定用 `IsBlacklistedErr`，不要默默 fail-open。
- `Configure(cfg)` 必须在启动早期调用；`SetProductionMode()` 决定走静态键还是随机键。
