# audit 模块

审计日志的异步写入与查询。

## 文件

`audit.go`（单文件）：常量、`Entry`/`Query`、`Service`、`IsValidAction`/`IsValidTargetType`、`AuditIP`、`List`。

## 结构

- **异步队列**：`queueSize = 1024`，`Log(e)` 入队立即返回，独立 `worker` 落库；队列满则丢弃并累加 `Dropped()`。
- **分页上限**：`AuditMaxLimit = 200`、`AuditDefaultLimit = 50`。

## 动作与目标白名单

动作：`kick_member` `mute_user` `unmute_user` `delete_room` `delete_user`。
目标类型：`member` `room` `user` `mute`。

**新增动作/目标类型必须同时改常量与 `validActions`/`validTargetTypes`**，否则写入被拒。

## 约束

- 审计是旁路：**不允许因为审计失败而让主业务失败**。
- `AuditIP(c)` 取真实客户端 IP（尊重可信代理配置），不要在 handler 里手写 IP 解析。
- 记录的是「谁对什么做了什么」，不要塞敏感内容（密码、token）。
- 测试环境用 `Stop()` 收尾，避免 goroutine 泄漏。
