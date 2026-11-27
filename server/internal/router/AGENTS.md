# router 模块

路由注册层：`router.go` 负责装配，`routes/<module>/routes.go` 负责各模块定义。

## 分组与中间件

```
/api/v1
├── 公开组            无中间件（auth/email/oauth/signal/invite 的一部分）
├── protected 组      JWTAuth() + BanCheck() + RequirePasswordChanged()，再按模块叠加 RequirePermission / RequireDomainMember
└── 根路径            /ping /readyz /uploads/* /swagger/*（及 SPA 回退）
```

**基线三件套 `JWTAuth() + BanCheck() + RequirePasswordChanged()`**（auth 子组走 `protectedNoPwdGate` 豁免第三件），没有访客守卫（GuestGuard 已随访客体系删除）。

## 模块清单（`routes/`）

`announcement` · `audit` · `auth` · `conversation` · `domain` · `email` · `email_config` · `invite` · `invite_join` · `message` · `mute` · `oauth` · `permission` · `role` · `room` · `sfu_config` · `signal` · `storage` · `swagger` · `system` · `user` · `user_group`

## 挂载要点（易错）

- `system` 在 **protected 组内**，`/system/stream` 与 `/system/update/*` 额外要求 `RequireRole("admin")`，不是公开路由。
- `message` 挂在 **`/room`** 前缀下（`/room/messages/*`），与 `room` 模块共用前缀。
- `user_group` 前缀是 `/user-group`。
- `invite` 同时注册公开（`/invite/preview`）与受保护（`/domain/invites/*`）两组。
- `invite_join` 的加入审核端点挂在 `/domain` 前缀下（`/domain/join-requests/*`、`/domain/join-approval/config`）。
- `conversation` / `announcement` / `invite` / `invite_join` 在 handler 为 nil 时跳过注册。

## 约定

- 精确路由表**以各 `routes.go` 代码为准**，本文件不再维护清单（避免腐化）。
- 新增模块：建 `routes/<module>/routes.go`，在其中区分 `Register`（公开）/ `RegisterProtected`，再到 `router.go` 挂组。
- 权限用 `middleware.RequirePermission(permcode.X)`；域内资源用 `middleware.RequireDomainMember()`。
- Swagger 使用 `locale: "zh-CN"`。
