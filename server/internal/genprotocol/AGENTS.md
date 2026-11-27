# genprotocol 模块

从 `internal/protocol` 静态生成前端 TypeScript（事件名、payload 类型、envelope）。

## 入口

`Generate(pkgDir string) (map[string]string, error)` —— 返回 `文件名 → 文件内容`，**不落盘**。

调用方：
- `server/cmd/gen-protocol/main.go`：`-out DIR` 写指定目录，`-update-golden` 写 `testdata/golden/`（二者互斥，缺 `-out` 即报错）。根 `pnpm gen:protocol` 只是冒烟（写 `/tmp/gen`）。
- 本包 `generate_test.go` 的 `TestGolden`：比对 `testdata/golden/` 是否新鲜，是唯一的新鲜度契约测试。

## 输出

`testdata/golden/` 下三个文件（提交入库；Client 仓的 `web/src/protocol/generated/` 由该仓 `scripts/check-protocol-sync.mjs` 从 golden 同步，本仓不写客户端产物）：
`events.ts`（事件名常量与联合类型）、`payloads.ts`（payload 接口）、`envelope.ts`（线路信封）。

文件头固定 `GENERATED — 手改无效`，**不要手改产物**。

## 实现要点

- 纯 `go/ast` 静态解析，**不执行任何代码**；分两阶段：先收集全部常量/struct（Catalog 会跨文件引用常量），再解析 `Catalog`。
- 字段名取 `json` tag（缺省转 camelCase），`omitempty` → TS 可选。
- 不支持的 Go 类型 → `errUnsupported`，生成失败而非静默降级。
- `goNameByWire` 把 wire 事件名映射回 Go 常量名，用于生成类型名。

## 测试

`generate_test.go` + `testdata/golden/`（`envelope.ts` / `events.ts` / `payloads.ts`）。改生成逻辑时同步更新 golden。

## 易错点

- 新增 payload 字段用了生成器不认识的类型（如 `map[string]any` 嵌套泛型）会直接报错，需先在 `tsType` 补映射。
- 生成器依赖 `Catalog` 的复合字面量写法；若改成运行时构造（函数返回切片），生成器找不到 `Catalog` var 会失败。
