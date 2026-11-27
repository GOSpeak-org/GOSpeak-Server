# 2026-11-24 插件 Node 运行时回退检测设计

> 【状态】2026-11-27 核验：该设计已落地（运行时回退探测已交付），仅供历史参考。

## 背景
当前插件管理器硬编码要求 `<plugin_root>/runtime/bin/node` 与 `<plugin_root>/bin/gospeak.runtime` 存在，缺失即 panic。单二进制部署没有这两个文件，导致主程序无法启动。

## 目标
让 Node 运行时探测具备回退能力：管理员在页面选装的插件根 node 优先；没有时复用系统 PATH 的 node；两者都没有时主程序仍可启动，仅跳过插件并告警。保持「零自动下载」约束：任何下载仍由管理员显式触发。

## 范围
- 修改 `app/server/internal/plugin` 的探测与启动逻辑。
- 不改动 proto、插件 SDK、管理 API 形状。
- 不实现静默自动下载；不处理 gospeak.runtime 二进制的分发。

## 架构
新增 `runtime_probe.go`：
- `resolveNode(root string) (path string, source string, ok bool)`：先检查 `<root>/runtime/bin/node` 且可执行，source=managed；否则 `exec.LookPath("node")` 且 `node --version` 成功，source=system；否则 ok=false。

修改 `manager.go`：
- `Start()` 探测失败时不再 panic，置 `m.noRuntime=true` 并返回 nil，主进程继续启动。
- `m.noRuntime` 时 Install 系列跳过插件并记录告警。

修改 `lifecycle.go`：
- `launch()` 调 `resolveNode(m.rootDir)` 取路径，去掉写死的路径。

修改 `test/plugin/testdata/gospeak.runtime/main.go`：
- `List()` 在插件根无软链且 PATH 有 node 时，报告 version=system、Active=true 的记录。

## 错误处理
- node 不可执行（权限/损坏）视为不可用，继续回退。
- PATH node 执行失败视为无运行时，主程序继续启动。

## 测试
- resolveNode 三种分支单测。
- Start 在无 node 时不 panic、返回 nil、m.noRuntime=true。
- launch 使用 PATH node。
- runtime List 上报 system active。