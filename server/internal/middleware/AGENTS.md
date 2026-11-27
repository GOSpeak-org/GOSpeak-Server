# middleware 模块

Gin 中间件：请求标识、鉴权、域成员校验、限流、CORS。

## 文件说明

| 文件 | 内容 |
|------|------|
| `auth.go` | `Configure` / `JWTAuth` / `RequireRole` / `RequirePermission` / `RequireOwnerOrPermission` / `BanCheck` / `CORS` / `VerifyToken` / `InvalidateTokenVersion` / `Current*` 取值器 |
| `domain.go` | `SetDomainChecker` / `IsDomainMember` / `IsPlatformAdmin` / `RequireDomainMember(IfProvided)` / `RequirePlatformAdminOrDomainMember(IfProvided)` |
| `ratelimit.go` | `RateLimit(max, window)` |
| `request_id.go` | `RequestID()` |
| `password_change.go` | `RequirePasswordChanged()`：默认口令管理员门禁；`ClearDefaultPasswordCache()` 与 `DefaultPasswordNeedsChange()` |
| `security_headers.go` | `SecurityHeaders()`：全局 CSP + nosniff + Referrer-Policy（Swagger 单独放宽） |

## JWTAuth 检查顺序

1. 取 `Authorization` 头；为空时回退 HttpOnly cookie（`ACCESS_COOKIE_NAME`，默认 `gospeak_token`，供 EventSource 使用）。
2. 都为空 → `TOKEN_NOT_EXIST` (1001)。
3. 去掉可选 `Bearer ` 前缀后 `VerifyToken`：签名 → 过期 → TokenType 须为 access → TokenVersion 一致 → JTI 不在登出黑名单。失败分别返回 `TOKEN_WRONG` / `TOKEN_EXPIRED` / `TOKEN_REVOKED`。
4. `setClaimsContext` 注入 `username` / `display_name` / `user_uuid` / `role` / `claims` / `auth_type`。

## 错误码约定

| 中间件 | 失败码 |
|--------|--------|
| `JWTAuth` | `TOKEN_NOT_EXIST` (1001) / `TOKEN_WRONG` (1002) / `TOKEN_EXPIRED` (1003) / `TOKEN_REVOKED` (1014) |
| `RequireRole` | `FORBIDDEN` (1013) |
| `RequirePermission` | 无 role → `TOKEN_NOT_EXIST` (1001)；无权限 → `FORBIDDEN` (1013) |
| `BanCheck` | **`USER_BANNED` (1015)**，经 `policy.CheckBan` |

## 策略单点

`BanCheck` 不自己判角色，而是调用 `internal/policy` 的 `CheckBan`；`/ws` 的握手与逐事件策略用同一个 `policy.Engine`（`server/ws_policy.go`）。**不要在中间件里另写一份封禁逻辑**，否则 HTTP 与 WS 会分叉。

## RBAC

- 权限码集中在 `internal/permcode/`（`permcode.go` 平台码 + `domain_permcode.go` 域内码）。
- 用户权限 = `role` 经 `role_permissions` 解析，由启动期注入的 `permissionChecker` 提供（`SetPermissionChecker` / `SetTokenVersionChecker`）。
- 域成员校验经 `SetDomainChecker` 注入，避免中间件反向依赖 repository。
