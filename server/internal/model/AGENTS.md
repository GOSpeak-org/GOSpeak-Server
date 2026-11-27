# model 模块

GORM 实体定义与表结构。迁移在 `internal/repository/db_migrations.go` 的 `AutoMigrate` 中注册，启动时自动建表。

## 文件说明

| 领域 | 文件 |
|------|------|
| 用户与权限 | `user.go` `role.go` `permission.go` `user_group.go` `audit_log.go` |
| 域（语音服务器） | `domain.go` `domain_role.go` `domain_invite.go` `domain_join_request.go` |
| 房间与消息 | `room.go` `message.go` `message_mention.go` `message_reaction.go` `conversation_participant.go` |
| 禁言 | `mute.go` |
| 邮箱 | `email_config.go` `email_verification_code.go` |
| OAuth | `oauth_account.go` `oauth_provider.go` |
| 配置 | `sfu_config.go` `storage_config.go` |
| 公告 | `announcement.go` |

## 当前事实（易误解处）

- `Domain` 只有 `UUID / Name / IconURL / Description / OwnerUUID / IsPublic / RequireJoinApproval`。**没有 `InviteCode`，也没有 `AllowGuest` / `Guest*` 系列字段**（访客体系已删除）。
- `DomainRole` 的系统角色只有 `owner` / `admin` / `member` 三个，**没有 `guest`**。
- 邀请是独立实体：`DomainInvite` + `DomainInviteRedemption`（自包含签名串，不存明文码）。
- 入域审核是独立实体：`DomainJoinRequest`（`status=pending/approved/rejected`）。
- `User` 无 `IsGuest`（列已通过迁移删除）；有 `TokenVersion`（改密后旧 token 失效）。
- `Room.DomainUUID` 非空，房间必属某域。

## 约定

- 新增模型必须同步在 `db_migrations.go` 注册，否则不会建表。
- 需要删列/删表时写幂等迁移并加测试（参考既有 `db_migrations_legacy_test.go`）。
- JSON tag 默认 snake_case；模型文件命名为 `snake_case.go`。
- 角色与权限的种子数据在 `model/permission.go` 的 `DefaultPermissions` / `DefaultRolePermissions`。
