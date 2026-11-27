# Signal 信令模块

> 标签: `Signal` | 基础路径: `/api/v1/signal`

## 接口列表

| 方法 | 路径 | 说明 | 需鉴权 |
|------|------|------|--------|
| POST | `/api/v1/signal/webhook` | LiveKit Webhook 回调 | ❌ |
| GET | `/api/v1/signal/rooms` | SFU 房间列表（LiveKit） | ✅ |
| GET | `/api/v1/signal/participants` | 房间参与者列表 | ✅ |
| POST | `/api/v1/signal/token` | 获取 SFU 加入 Token（LiveKit） | ✅ |
| GET | `/api/v1/signal/ws-endpoint` | 获取 WebSocket 信令端点 | ✅ |

> WebRTC offer/answer/ICE 信令已改由 `/ws` 原生 WebSocket 承载，不再有 HTTP `/signal/signal` 中转端点。

---

## POST /api/v1/signal/token

生成 SFU 房间加入 Token（LiveKit）

### Request Body

```json
{
  "room": "my-room",
  "domain_uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "password": ""
}
```

**字段说明:**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| room | string | ✅ | 房间名称 |
| domain_uuid | string | ❌ | 域 UUID |
| password | string | ❌ | 房间密码 |

> `identity` 不接收客户端输入，由已认证用户的 access token 推导。

### Response (200)

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "room": "my-room",
    "identity": "user-123"
  }
}
```

### Response (400)

```json
{
  "code": 2001,
  "msg": "参数错误详情",
  "data": null
}
```

---

## GET /api/v1/signal/rooms

获取 LiveKit 中所有活跃房间列表（需 admin 角色，`middleware.RequireRole("admin")`）

### Response (200)

```json
{
  "code": 0,
  "msg": "success",
  "data": [
    {
      "name": "my-room",
      "memberCount": 2
    }
  ]
}
```

### Response (500)

```json
{
  "code": 5001,
  "msg": "服务器内部错误",
  "data": null
}
```

---

## GET /api/v1/signal/participants

获取指定房间的所有参与者（需 admin 角色，`middleware.RequireRole("admin")`）

### Query Parameters

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| room | string | ✅ | 房间名称 |

### 请求示例

```
GET /api/v1/signal/participants?room=my-room
```

### Response (200)

```json
{
  "code": 0,
  "msg": "success",
  "data": [
    {
      "identity": "user-123",
      "joinedAt": 1763966400
    }
  ]
}
```

### Response (400)

```json
{
  "code": 2001,
  "msg": "room is required",
  "data": null
}
```