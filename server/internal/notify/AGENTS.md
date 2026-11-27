# notify 模块

把事件定向投递给用户或广播给全体，供公告等服务复用。

## API

- `NotifyUser(ctx, bus, userUUID, event, payload)`：发到用户个人房间 `__user:<uuid>`。
- `NotifyAll(ctx, bus, event, payload)`：发到广播命名空间。
- `UserRoomPrefix = "__user:"` 是个人房间前缀常量。

## 语义

- 公告类：平台公告走 namespace 广播，域公告定向投递到 `__user` 房间。
- 依赖 `bus` 接口（`EventBus`），不直接依赖 Hub，便于测试。

## 约束

- 事件名用 `protocol.EventXxx`，不要写字面量。
- 用户房间前缀只认 `UserRoomPrefix`，不要在别处拼 `"__user:"`。
- 通知是「尽力而为」：失败要记录并让上层决定是否重试，不要静默吞掉。
