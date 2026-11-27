# protocol 模块

WS 事件协议的**唯一事实源**。事件名、方向、是否需要 ACK、payload 结构全部在这里定义，前端 TS 由 `genprotocol` 生成。

## 文件

| 文件 | 内容 |
|------|------|
| `events.go` | `Direction` 常量、`EventXxx` 事件名常量、`Catalog`、`payloadStructs` / `eventConsts` 登记表 |
| `payloads.go` | 房间/成员/禁言等 payload struct |
| `message_payloads.go` | 消息与 `MessageDTO` |
| `internal_events.go` | `DirInternal` 事件（不产出前端 TS） |
| `catalog_test.go` | Catalog 完整性校验 |

## Direction

`DirClientToServer`(c2s) / `DirServerToClient`(s2c) / `DirBoth` / `DirInternal`（仅服务端内部，不生成 TS）。

## 新增/修改事件的强制步骤

1. 在 `events.go` 增加 `EventXxx` 常量。
2. 在 `Catalog` 增加条目（Name 必须引用常量，不要写字面量）。
3. 有 payload 时在对应文件定义 struct，并登记进 `payloadStructs`。
4. 常量登记进 `eventConsts`（漏登记 → 契约测试红）。
5. 跑 `cd server && go run ./cmd/gen-protocol -update-golden` 更新 `server/internal/genprotocol/testdata/golden/` 三个产物并提交；客户端仓的 `web/src/protocol/generated/` 由 Client 仓 `scripts/check-protocol-sync.mjs --fix` 从 golden 同步（本仓不写客户端产物）。
6. 更新使用方（signal / ws / service / 前端）。

## 约束

- **禁止在其他包或前端手写 wire 事件名字符串**，一律用 `protocol.EventXxx`。
- `Catalog` 条目的事件名必须**等于**常量值（`catalog_test.go` 会校验，防手抄漂移）。
- `Payload` 引用的 struct 必须真实存在于本包，否则测试失败。
- `DirInternal` 事件不进前端，也不要求 payload 挂到 Catalog（按需）。
- 改完不重新生成 → 本地 `go test ./internal/genprotocol/` 就会红（`genprotocol.TestGolden` 对比 golden），CI 的 protocol gen-diff 还会再拦一次。
