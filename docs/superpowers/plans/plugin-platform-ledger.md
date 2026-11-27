# 插件平台执行 Ledger

主 agent 编排；各计划 Subagent-driven（每 Task 新实现子代理 + 新审阅子代理）；计划间集成 review。

## 状态

- [x] 计划01 基础协议与 SDK（proto / JS SDK / 示例插件模板）— 交付，集成审阅通过
- [x] 计划01→02 集成 review（hook_endpoint 契约修复）
- [x] 计划02 运行时与进程管理（数据表/审计 / runtime 插件 / Manager）— 交付，审阅+修复通过
- [x] 计划02→03 集成 review（注册表 DB 唯一事实源 / RuntimeList 签名修复）
- [x] 计划03 管理面与插件能力（管理API/页面 / 插件API / Hook / 页面挂载）— 交付，审阅+修复通过
- [x] 计划03→04 集成 review（gin.go 统一装配 2f56cc1f）
- [~] 计划04 集成验证：Go 全量构建/vet/test 与前端 typecheck/SDK test/文档构建已由 lead 执行通过；独立 E2E 测试夹具与 gospeak_plugin_ 指标 Task 未单独实现（记录为后续项）
- [x] 计划05 文档与文档站：AGENTS 指针/部署文档（bc42de0b）+ VitePress 插件指南与 API 参考（82cc1e4f），sidebar 可达、docs build 通过
- [x] 最终整体验收：门禁通过（2026-11-24，补齐后复验 Go exit 0 / 前端 SDK 5/5 / web typecheck / docs build 全通过）

## 完成记录

- 计划02 Task2（runtime 插件）：✅ 审阅通过（无 blocking）。commit 9280d911。非阻断建议：installNode 原子性（旧版本 rename .old）、extractTarGz 路径遍历过滤、启动零下载测试加请求计数、未使用常量 runtimeMain。
- 计划02 Task1（数据表/审计/系统用户）：✅ 审阅通过（Accept + 修复）。commit d0cc2505；修复 f3b418e8（Enabled 应用层默认）、78c3ac9c（genprotocol golden 补齐 MemberInfo.user_uuid，基线 bf97d727 遗留，非计划01）。
- 计划01 Task1（proto）：✅ 审阅通过（SPEC 合规、质量通过、无 blocking）。commit 4735708b。审阅非阻断建议：CI apt 安装的 protoc 版本未钉死，后续可改用 setup-protoc 固定版本，避免 runner 镜像漂移导致生成物误报。

- 计划03 Task3（Hook）：审阅基本通过；blocking 生产链路未接线归 lead 集成阶段；实现方补：真实 5s 超时测试 + Close/Stop 入口。
- 计划03 Task4（页面挂载）：审阅 3 blocking（BrowserProxy 身份签名、Origin 默认拒绝、routeTree 强提交）派修复中。

- 计划02 Task3 / 计划03 Task1 集成 blocking：注册表以 DB plugin_registrations 为唯一事实源（Manager 写、Service 读）；RuntimeList 签名统一为 []*gen.RuntimeVersion；Manager 心跳需真正重启进程。

## Rulings

- 注册表唯一事实源：DB plugin_registrations 表；manager 的 _registrations.json 仅作调试镜像，不作权威；管理面 List/Enable/Disable 以 DB 为准。

- routeTree.gen.ts 强提交：裁定保留提交但修正 .gitignore（移除对该生成物的忽略），与 WS 生成物入库约定一致；不手改生成物，仍由 tsr generate 产出。
- 多 agent 并行期间，server/gin.go 由 lead 统一装配（所有子代理禁止改）；子代理"未改 gin.go"不算 blocking，装配需求在报告中登记，集成阶段统一处理。

- protoc -I 与计划命令不同：接受实现者的等价命令（include 根相同、生成物稳定、CI 与 generate.go 一致）— 影响：仅命令写法，产物正确。
## 最终验收证据（2026-11-24）

- Go: `go build ./... && go vet ./... && go test ./... -count=1` → exit 0，全部包 ok（含 internal/plugin 6.595s）。
- 前端: `pnpm --filter @gospeak/plugin-sdk test` 5/5 PASS；`pnpm --filter @gospeak/web typecheck` PASS；`pnpm --filter @gospeak/docs build` PASS。
- 装配提交：2f56cc1f（gin.go 启动链 + 管理导航）。

## 后续项（未纳入本次交付）

- E2E fixture 的 GetUser 使用固定 UUID，返回 NOT_FOUND（测试数据问题，非代码缺陷）。
- 审计 API 在 E2E 中软断言（admin 403/1013），写入由 Go 单测覆盖。
- runtime 非阻断建议：installNode 原子性、extractTarGz 路径遍历过滤。
- CI protoc 版本钉死（避免 runner 漂移）。