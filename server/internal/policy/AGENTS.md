# policy 模块

传输无关的访问策略引擎。**HTTP 与 WS 两个通道共用同一份判决**，避免绕过中间件链形成策略洼地。

## 类型

- `Subject{UserUUID, Username, Role}`，由 `SubjectFromClaims` 从 JWT claims 构造。
- `Action`：当前只有 `ActionWSConnect`。
- `Decision{Allowed, Code, Message, Terminate}`：`Code`/`Message` 可直接用于任意通道响应；`Terminate=true` 表示命中不可逆状态（封禁），WS 应主动断连以停止下行广播。

## 当前规则（重要）

只有一条：**身份级封禁** —— `role == ban` → `USER_BANNED`(1015)，`Terminate=true`。

- `CheckBan(sub)`：给 `middleware.BanCheck` 用。
- `Engine.Evaluate(sub, action, domainUUID)`：HTTP/WS 共用调用点；**`action` 与 `domainUUID` 目前是预留参数，没有任何域级规则参与判决**。
- `hasBanRole` 在 `model.HasBanRole`（依赖启动期 roleCache）之外补了字面量 `ban` 兜底，避免未走 bootstrap 的路径 fail-open。

## 约束

- 调用方**不得假设域级强制已生效**，也不要把 payload 里的 `domain_uuid` 当作已校验的域上下文（必须用服务端登记过的可信值）。
- 不要在中间件或 WS handler 里另写一份封禁判断；新增规则加在本 Engine，两边自动一致。
- 保留 `Engine` 结构体（当前无状态）是为了将来引入域级策略时不必改调用方签名。
