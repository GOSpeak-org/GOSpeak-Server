# User 用户管理模块

> 标签: `User` | 基础路径: `/api/v1/user`
>
> 精确路由、分组与中间件以 `internal/router/routes/user/routes.go` 为准。本模块端点均为 **POST + JSON body**（预设头像另有 GET），鉴权走 protected 组（JWT + BanCheck）。

## 接口列表

| 方法 | 路径 | 说明 | 权限码 |
|------|------|------|--------|
| POST | `/api/v1/user/profile` | 获取当前登录用户资料 | 仅登录 |
| POST | `/api/v1/user/info` | 按用户名（identity）查询用户 | 仅登录 |
| GET / POST | `/api/v1/user/preset-avatars` | 预设头像列表 | 仅登录 |
| POST | `/api/v1/user/update-profile` | 更新个人资料 | 仅登录 |
| POST | `/api/v1/user/list` | 用户列表（分页） | `user:read` |
| POST | `/api/v1/user/get` | 按 ID 获取用户详情 | `user:read` |
| POST | `/api/v1/user/delete` | 删除用户 | `user:delete` |
| POST | `/api/v1/user/update-role` | 更新用户角色 | `user:update` |
| POST | `/api/v1/user/push-token` | 上报推送 token | 仅登录 |

---

## POST /api/v1/user/profile

获取当前登录用户的资料。身份由 access token 推导，不需要请求体。

### Response (200)

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "id": 1,
    "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "name": "john",
    "role": "user",
    "permissions": ["..."],
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-01-01T00:00:00Z"
  }
}
```

> 用户名字段为 `name`（JWT 中同名 claim 为 `username`）。

### Response (401)

```json
{ "code": 1001, "msg": "token 不存在", "data": null }
```

### Response (404)

```json
{ "code": 3001, "msg": "用户不存在", "data": null }
```

---

## POST /api/v1/user/list

分页获取用户列表。参数为 **JSON body**，不是 query string。

### Request Body

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| page | int | ❌ | 1 | 页码 |
| page_size | int | ❌ | 20 | 每页数量 |
| exclude_bots | bool | ❌ | false | 是否排除机器人账号 |
| keyword | string | ❌ | 空 | 关键词过滤 |

```json
{ "page": 1, "page_size": 20, "exclude_bots": false, "keyword": "" }
```

### Response (200)

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "list": [
      { "id": 1, "uuid": "xxx", "name": "john", "created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-01-01T00:00:00Z" }
    ],
    "total": 1,
    "page": 1
  }
}
```

### Response (500)

```json
{ "code": 5001, "msg": "服务器内部错误", "data": null }
```

---

## POST /api/v1/user/get

按 ID 获取用户详情。

### Request Body

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | uint | ✅ | 用户 ID |

```json
{ "id": 1 }
```

### Response (200)

```json
{
  "code": 0,
  "msg": "success",
  "data": { "id": 1, "uuid": "xxx", "name": "john" }
}
```

### Response (404)

```json
{ "code": 3001, "msg": "用户不存在", "data": null }
```

---

## POST /api/v1/user/delete

按 ID 删除用户。

### Request Body

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | uint | ✅ | 用户 ID |

```json
{ "id": 1 }
```

### Response (200)

```json
{ "code": 0, "msg": "success", "data": null }
```

### Response (400)

```json
{ "code": 2001, "msg": "参数错误", "data": null }
```
