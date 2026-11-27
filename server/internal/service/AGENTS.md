# service 模块

业务逻辑层：协调 repository 与外部抽象（SFU / AuthState / Bus / 存储 / 邮件），是唯一编写领域规则的地方。

## 分组

| 领域 | 文件 |
|------|------|
| 认证与用户 | `auth_service` `user_service` `user_group_service` `email_service` `email_verification_service` `oauth_service` |
| 权限 | `casbin` `domain_casbin` `permission_service` `role_service` |
| 域与邀请 | `domain_service` `invite_service` `invite_join_service` `join_request_service` `invite_code` |
| 房间与信令 | `room_service` `sfu_service` `sfu_config_service` `mute_service` |
| 消息 | `message_service` `message_history` `message_mutation` `message_jobs` `conversation_service` |
| 其他 | `announcement_service` `storage_service` `system_update_service` `events` |

## 约定

- **返回 `*pkg.AppError`**（`pkg.NewAppError(code, msg)`）。历史代码中仍有少量 `fmt.Errorf` 返回，属遗留；新增代码必须用 AppError，不要继续扩散。
- **不感知 HTTP**：禁止 import `gin`、禁止出现 `*gin.Context`。需要请求上下文时传 `context.Context`。
- **跨表写入用事务**：邀请码兑换 + 入域等必须原子（见 repository `tx.go`）。
- **领域规则写在这里**，不要固化进 repository 的 SQL，也不要在 handler 里判断。
- **事件常量**：`events.go` 只是 `internal/protocol` 的别名，新代码直接引用 `protocol.EventXxx`，不要新增字符串字面量。
- **异步任务**走 `internal/jobs`（聊天持久化 `ChatPersister`、禁言到期扫描），service 只暴露接口。

## 易错点

- 禁言是**用户级**语义：允许收听、禁止发布本地音轨；不存在房间级禁言。
- SFU 只支持 LiveKit，`sfu_config_service` 对该约束是硬编码的；不要写「切换 provider」的新逻辑。
- 改密/角色变更要让 `TokenVersion` 失效（旧 token 立即不可用）。
