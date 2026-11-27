# GOSpeak API 总览

> 版本: 1.0 | 基础路径: `/api/v1`

## 服务信息

| 项目 | 内容 |
|------|------|
| 标题 | GOSpeak API |
| 描述 | GOSpeak - WebRTC Server API |
| 版本 | 1.0 |
| 服务条款 | https://github.com/GOSpeak-org/GOSpeak-Server |
| 许可证 | MIT (https://opensource.org/licenses/MIT) |
| 联系人 | GOSpeak-org (https://github.com/GOSpeak-org) |

## 基础 URL

**开发环境:** `http://localhost:8998`
**基础路径:** `/api/v1`

## 鉴权方式

所有需要登录的接口使用 **Bearer Token** 鉴权：

```
Authorization: Bearer <your-jwt-token>
```

### 获取 Token

1. 调用 `POST /api/v1/auth/login` 获取 `access_token` 和 `refresh_token`
2. 在 Swagger UI 中点击 **Authorize** 按钮，输入 `Bearer <token>`
3. Token 过期后使用 `POST /api/v1/auth/refresh_token` 刷新

## 通用响应格式

所有接口返回统一的 JSON 结构：

```json
{
  "code": 0,
  "msg": "success",
  "data": {}
}
```

### 错误码

> 以下为 `internal/pkg/errors.go` 中定义的完整错误码（字段名统一为 `msg`，非 `message`）。

| code | 名称 | 说明 |
|------|------|------|
| 0 | SUCCESS | 请求成功 |
| 1001 | TOKEN_NOT_EXIST | 未提供 Token |
| 1002 | TOKEN_WRONG | Token 无效 |
| 1003 | TOKEN_EXPIRED | Token 过期 |
| 1010 | INVALID_PASSWORD | 密码错误 |
| 1011 | USER_NOT_FOUND | 用户不存在 |
| 1012 | USERNAME_EXISTS | 用户名已存在 |
| 1013 | FORBIDDEN | 禁止访问（权限不足） |
| 1014 | TOKEN_REVOKED | Token 已吊销（黑名单 / 版本失效） |
| 1015 | USER_BANNED | 用户被封禁 |
| 1016 | USER_MUTED | 用户被禁言 |
| 1017 | RATE_LIMITED | 触发限流 |
| 2001 | INVALID_PARAMS | 请求参数错误 |
| 3001 | NOT_FOUND | 资源不存在 |
| 3002 | ALREADY_EXISTS | 资源已存在 |
| 5001 | INTERNAL_ERROR | 服务器内部错误 |
| 6001 | SFU_NOT_CONFIGURED | SFU 未配置 |
| 6002 | SFU_ERROR | SFU 错误 |
| 7001 | OAUTH_PROVIDER_NOT_FOUND | OAuth 提供商不存在 |
| 7002 | OAUTH_PROVIDER_DISABLED | OAuth 提供商已禁用 |
| 7003 | OAUTH_TOKEN_EXCHANGE_FAILED | OAuth Token 交换失败 |
| 7004 | OAUTH_GET_USER_FAILED | OAuth 获取用户信息失败 |
| 8001 | EMAIL_ALREADY_EXISTS | 邮箱已存在 |
| 8002 | EMAIL_CODE_INVALID | 邮箱验证码无效 |
| 8003 | EMAIL_CODE_EXPIRED | 邮箱验证码过期 |
| 8004 | EMAIL_CODE_ALREADY_USED | 邮箱验证码已使用 |
| 8005 | EMAIL_SEND_TOO_FREQUENT | 邮箱发送过于频繁 |
| 8006 | EMAIL_SEND_FAILED | 邮箱发送失败 |
| 8007 | EMAIL_NOT_VERIFIED | 邮箱未验证 |
| 8008 | EMAIL_CODE_MAX_ATTEMPTS | 邮箱验证码尝试超限 |
| 8009 | EMAIL_NOT_CONFIGURED | 邮箱服务未配置 |
| 8010 | PASSWORD_RESET_DISABLED | 密码重置已禁用 |
| 8101 | STORAGE_NOT_CONFIGURED | 存储未配置 |
| 8102 | STORAGE_ERROR | 存储错误 |
| 8103 | STORAGE_FILE_TOO_LARGE | 文件过大 |
| 8104 | STORAGE_FILE_TYPE_NOT_ALLOWED | 文件类型不允许 |

## 接口列表

> 完整路由以 `internal/router/router.go` 与各 `routes/*/routes.go` 为准；下表列出主要端点（基础路径 `/api/v1`），新增端点以代码为准。✅ = 需 JWT，❌ = 公开。

| 模块 | 路径 | 方法 | 需鉴权 |
|------|------|------|--------|
| Auth | `/auth/login` | POST | ❌ |
| Auth | `/auth/register` | POST | ❌ |
| Auth | `/auth/refresh_token` | POST | ❌ |
| Auth | `/auth/reset_password` | POST | ❌ |
| Auth | `/auth/logout` | POST | ✅ |
| Auth | `/auth/refresh` | POST | ✅ |
| Auth | `/auth/change_password` | POST | ✅ |
| Auth | `/auth/first_change_password` | POST | ✅ |
| Email | `/email/send_code` | POST | ❌ |
| Email | `/email/verify_code` | POST | ❌ |
| Email | `/email/config` | POST | ✅ |
| Email | `/email/update-config` | POST | ✅ |
| OAuth | `/oauth/login/:provider` | GET | ❌ |
| OAuth | `/oauth/callback/:provider` | GET | ❌ |
| OAuth | `/oauth/providers` | GET | ❌ |
| OAuth | `/oauth/admin/providers` | GET/POST/PUT | ✅ |
| OAuth | `/oauth/admin/providers/:id` | DELETE | ✅ |
| Signal | `/signal/webhook` | POST | ❌ |
| Signal | `/signal/token` | POST | ✅ |
| Signal | `/signal/rooms` | GET | ✅ |
| Signal | `/signal/participants` | GET | ✅ |
| Signal | `/signal/ws-endpoint` | GET | ✅ |
| Invite | `/invite/preview` | POST | ❌ |
| Invite | `/invite/register` | POST | ❌ |
| User | `/user/profile` | POST | ✅ |
| User | `/user/info` | POST | ✅ |
| User | `/user/preset-avatars` | GET/POST | ✅ |
| User | `/user/update-profile` | POST | ✅ |
| User | `/user/list` | POST | ✅ |
| User | `/user/get` | POST | ✅ |
| User | `/user/delete` | POST | ✅ |
| User | `/user/update-role` | POST | ✅ |
| User Group | `/user-group/list` | POST | ✅ |
| User Group | `/user-group/create` | POST | ✅ |
| User Group | `/user-group/update` | POST | ✅ |
| User Group | `/user-group/delete` | POST | ✅ |
| Role | `/role/list` | POST | ✅ |
| Role | `/role/create` | POST | ✅ |
| Role | `/role/update` | POST | ✅ |
| Role | `/role/delete` | POST | ✅ |
| Permission | `/permission/list` | POST | ✅ |
| Permission | `/permission/role` | POST | ✅ |
| Permission | `/permission/sync` | POST | ✅ |
| Mute | `/mute/create` | POST | ✅ |
| Mute | `/mute/cancel` | POST | ✅ |
| Mute | `/mute/status` | POST | ✅ |
| Mute | `/mute/list` | POST | ✅ |
| Room | `/room/create` | POST | ✅ |
| Room | `/room/list` | POST | ✅ |
| Room | `/room/get` | POST | ✅ |
| Room | `/room/update` | POST | ✅ |
| Room | `/room/delete` | POST | ✅ |
| Message | `/room/messages/list` | POST | ✅ |
| Message | `/room/messages/search` | POST | ✅ |
| Message | `/room/messages/react` | POST | ✅ |
| SFU | `/sfu/config` | POST | ✅ |
| SFU | `/sfu/config/:provider` | POST | ✅ |
| SFU | `/sfu/update-config` | POST | ✅ |
| SFU | `/sfu/providers` | POST | ✅ |
| Storage | `/storage/presign` | POST | ✅ |
| Storage | `/storage/confirm` | POST | ✅ |
| Storage | `/storage/upload` | POST | ✅ |
| Storage | `/storage/delete` | POST | ✅ |
| Storage | `/storage/config` | POST | ✅ |
| Storage | `/storage/update-config` | POST | ✅ |
| Storage | `/storage/test-config` | POST | ✅ |
| Conversation | `/conversation/list` | POST | ✅ |
| Conversation | `/conversation/messages` | POST | ✅ |
| Conversation | `/conversation/mark-read` | POST | ✅ |
| Conversation | `/conversation/send` | POST | ✅ |
| Domain | `/domain/create` | POST | ✅ |
| Domain | `/domain/get` | POST | ✅ |
| Domain | `/domain/list` | POST | ✅ |
| Domain | `/domain/list-public` | POST | ✅ |
| Domain | `/domain/my-domains` | POST | ✅ |
| Domain | `/domain/update` | POST | ✅ |
| Domain | `/domain/delete` | POST | ✅ |
| Domain | `/domain/leave` | POST | ✅ |
| Domain | `/domain/kick` | POST | ✅ |
| Domain | `/domain/members` | POST | ✅ |
| Domain | `/domain/roles/list` | POST | ✅ |
| Domain | `/domain/roles/create` | POST | ✅ |
| Domain | `/domain/roles/update` | POST | ✅ |
| Domain | `/domain/roles/delete` | POST | ✅ |
| Domain | `/domain/members/update-role` | POST | ✅ |
| Domain | `/domain/my-permissions` | POST | ✅ |
| Invite | `/domain/invites/list` | POST | ✅ |
| Invite | `/domain/invites/create` | POST | ✅ |
| Invite | `/domain/invites/disable` | POST | ✅ |
| Invite | `/domain/invites/redemptions` | POST | ✅ |
| Invite | `/invite/join` | POST | ✅ |
| Invite | `/invite/my-request` | POST | ✅ |
| Invite | `/invite/cancel-request` | POST | ✅ |
| Invite | `/domain/join-requests/list` | POST | ✅ |
| Invite | `/domain/join-requests/review` | POST | ✅ |
| Invite | `/domain/join-approval/config` | POST | ✅ |
| Announcement | `/announcement/list` | POST | ✅ |
| Announcement | `/announcement/unread-count` | POST | ✅ |
| Announcement | `/announcement/mark-read` | POST | ✅ |
| Announcement | `/announcement/admin/list` | POST | ✅ |
| Announcement | `/announcement/admin/create` | POST | ✅ |
| Announcement | `/announcement/admin/update` | POST | ✅ |
| Announcement | `/announcement/admin/delete` | POST | ✅ |
| Audit | `/audit/list` | POST | ✅ |
| System | `/system/stream` | GET | ✅ |
| System | `/system/update/current` | GET | ✅ |
| System | `/system/update/check` | GET | ✅ |
---

## 模块与路由前缀

下表列出服务端实际存在的业务模块（对照 `internal/handler/` 与 `internal/router/`）。基础路径统一为 `/api/v1`，`鉴权` 指是否要求 Bearer Token（受 `protected` 组约束）。

| 模块 | 路由前缀 | 鉴权 |
|------|----------|------|
| Auth | `/api/v1/auth` | 公开 + 受保护 |
| User | `/api/v1/user` | 受保护 |
| User Group | `/api/v1/user-group` | 受保护 |
| OAuth | `/api/v1/oauth`（`/oauth/admin` 受保护） | 公开 + 管理员 |
| Signal | `/api/v1/signal` | 公开 + 受保护 |
| Role | `/api/v1/role` | 受保护 |
| Permission | `/api/v1/permission` | 受保护 |
| Mute | `/api/v1/mute` | 受保护 |
| Room | `/api/v1/room` | 受保护 |
| Message（房间消息） | `/api/v1/room/messages` | 受保护 |
| Storage | `/api/v1/storage` | 受保护 |
| Email（验证） | `/api/v1/email` | 公开 + 受保护 |
| Email Config | `/api/v1/email`（`/config`、`/update-config`） | 受保护 |
| SFU Config | `/api/v1/sfu`（`sfu:manage`） | 受保护 |
| Conversation | `/api/v1/conversation` | 受保护 |
| Domain | `/api/v1/domain` | 受保护 |
| Audit | `/api/v1/audit` | 受保护 |
| System（健康检查流） | `/api/v1/system` | 受保护 |
| Announcement | `/api/v1/announcement` | 受保护 |
| Invite | `/api/v1/invite`（`/invite/preview` 公开） | 公开 + 受保护 |
| Invite Join（入域审核） | `/api/v1/domain/invites`、`/domain/join-requests` | 受保护 |
| Swagger | `/swagger/*` | 公开 |

> SFU 媒体层基于 **LiveKit**（由 `SFU_PROVIDER` 选择，唯一支持 `livekit`）。