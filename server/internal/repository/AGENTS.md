# repository 模块

数据访问层：封装数据库操作，向 service 暴露 CRUD 与查询方法。

## 基础设施

| 文件 | 职责 |
|------|------|
| `db.go` | 数据库初始化与连接池 |
| `db_migrations.go` | `AutoMigrate` 注册、幂等删列/删表迁移 |
| `tx.go` | 事务辅助 |
| `timeout.go` | 查询超时包装 |
| `casbin_adapter.go` / `domain_casbin_adapter.go` | Casbin 存储适配 |

## 仓储

`user_repo` `user_group_repo` `role_repo` `permission_repo` `audit`（在 handler 侧）`domain_repo` `domain_role_repo` `domain_invite_repo` `domain_join_request_repo` `room_repo` `message_repo` `conversation_repo` `mute_repo` `email_config_repo` `email_verification_code_repo` `oauth_provider_repo` `oauth_account_repo` `sfu_config_repo` `storage_config_repo` `announcement_repo` `push_token_repo` `room_layout_repo` `user_domain_layout_repo`

## 约定

- 只做数据访问，**不承载业务策略**（角色升降级、权限计算等应在 `internal/policy` / service）。
- 单条查询未命中返回 `gorm.ErrRecordNotFound`，由 service 转成 `*pkg.AppError`。
- 事务边界：跨表写入用 `tx.go` 辅助，保证原子性（如邀请码兑换 + 入域）。
- 新增仓储需在 `server/gin.go` 装配并注入对应 service。
- 注意 `db_migrations.go` 中的幂等逻辑：重复执行必须安全。
