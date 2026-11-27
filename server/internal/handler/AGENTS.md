# handler 模块

HTTP 请求处理层：参数绑定与校验、调用 service、按统一格式返回。

## 分组

| 领域 | 文件 |
|------|------|
| 认证 | `auth_handler` `authcookie` `oauth_handler` |
| 用户与角色 | `user_handler` `user_group_handler` `role_handler` `permission_handler` |
| 域 | `domain_handler` `domain_access` `invite_handler` `invite_join_handler` `join_request_handler` |
| 房间与信令 | `room_handler` `signal_handler` `mute_handler` `sfu_config_handler` |
| 消息 | `message_handler` `conversation_handler` |
| 其他 | `announcement_handler` `audit_handler` + `audit_helper` `storage_handler` + `storage_object_handler` + `storage_helpers` `storage_config_handler` `email_config_handler` `email_verification_handler` `monitor_handler` `system_update_handler` |

## 约定

- 方法签名固定 `func (h *XxxHandler) Name(c *gin.Context)`，**无返回值**。
- 成功用 `pkg.Success(c, data)`；业务错误 `pkg.HandleError(c, err)`；参数错误 `pkg.Fail(c, pkg.INVALID_PARAMS, ...)`。
- **不写业务逻辑**：校验入参格式后即调用 service，领域规则留在 service。
- 不在 handler 里分支 SFU provider——统一走 `sfu.Provider` 抽象，按 `ErrSFUNotSupported` 降级。
- 敏感字段（密码等）JSON tag 用 `json:"-"`，不要出现在响应里。

## signal_handler 要点

- `LivekitWebhook`：先验签（失败 → `TOKEN_WRONG`；未配置 → `SFU_NOT_CONFIGURED`），再投递到 jobs；签名校验不可省略。
- `ListParticipants` 与 `ListRooms` 一样：上游 SFU 出错时走 `pkg.HandleError(c, err)`，不得把故障伪装成「房间无人」的空数组。回归测试见 `signal_handler_test.go` 的 `TestListParticipants_SFUError`。

## 测试

handler 单测与源码同包（`*_test.go`），用 `rt`/`gin.TestContext` 构造请求；跨层行为放 `test/` 集成测试。
