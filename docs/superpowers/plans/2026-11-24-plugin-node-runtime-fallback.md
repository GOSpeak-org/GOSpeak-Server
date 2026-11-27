# 插件 Node 运行时回退检测 实现计划

> 【状态】2026-11-27 核验：该计划已交付（运行时回退探测已落地），仅供历史参考。

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让 Node 运行时探测具备回退能力：插件根选装优先，PATH 兜底，都无则主进程正常启动并告警，零自动下载保持不变。

**架构：** 新增独立探测文件 runtime_probe.go；Manager.Start 不再 panic；launch 使用探测结果；runtime 的 List 上报 system 版本。

**技术栈：** Go 1.27、os/exec、testing、现有 plugin 包 fake 依赖注入。

---

### 任务 1：运行时探测单元

**文件：**
- 创建：`app/server/internal/plugin/runtime_probe.go`
- 测试：`app/server/internal/plugin/runtime_probe_test.go`

- [ ] **步骤 1：编写失败测试**

在 runtime_probe_test.go 写 TestResolveNodePrefersManaged / TestResolveNodeFallsBackToPATH / TestResolveNodeNotFound，断言 path/source/ok 三元组（详见设计文档）。

- [ ] **步骤 2：运行测试确认失败**

运行：`cd app/server && go test ./internal/plugin -run TestResolveNode -count=1`
预期：FAIL，undefined: resolveNode

- [ ] **步骤 3：实现 resolveNode**

创建 runtime_probe.go，函数逻辑：managed 路径存在且 --version 成功 → (managed,"managed",true)；exec.LookPath("node") 且 --version 成功 → (system,"system",true)；否则 ("","",false)。

- [ ] **步骤 4：运行测试确认通过**

运行：`cd app/server && go test ./internal/plugin -run TestResolveNode -count=1`
预期：PASS

- [ ] **步骤 5：Commit**

提交信息：`feat(plugin): add node runtime probe with PATH fallback`

### 任务 2：Manager 启动不再因缺 node panic

**文件：**
- 修改：`app/server/internal/plugin/manager.go` 的 Manager 结构与 Start（约 139-185、296-312）
- 测试：`app/server/internal/plugin/manager_test.go`

- [ ] **步骤 1：编写失败测试**

写 TestManagerStartWithoutNodeDoesNotPanics：空 TempDir、NewManager、NotPanics 包裹 Start，断言 err=nil 且 m.noRuntime=true。

- [ ] **步骤 2：运行测试确认失败**

运行：`cd app/server && go test ./internal/plugin -run TestManagerStartWithoutNode -count=1`
预期：FAIL，m.noRuntime undefined

- [ ] **步骤 3：修改 Manager.Start**

Manager 加 noRuntime bool。Start 先 resolveNode；ok=false → Warn 日志 + m.noRuntime=true + return nil；ok=true → 原有 runtime 二进制校验与 startRuntime 流程。

- [ ] **步骤 4：运行测试确认通过**

运行：`cd app/server && go test ./internal/plugin -run TestManagerStartWithoutNode -count=1`
预期：PASS

- [ ] **步骤 5：Commit**

提交信息：`fix(plugin): allow server start without node runtime`

### 任务 3：插件启动使用探测结果

**文件：**
- 修改：`app/server/internal/plugin/lifecycle.go:243`
- 测试：`app/server/internal/plugin/manager_test.go`

- [ ] **步骤 1：编写失败测试**

写 TestLaunchUsesSystemNodeFallback：makeManager 后删掉根 node，PATH 注入临时 node，替换 deps.command 记录 name，调 launch，断言记录的 name 等于 PATH node 路径。

- [ ] **步骤 2：运行测试确认失败**

运行：`cd app/server && go test ./internal/plugin -run TestLaunchUsesSystemNodeFallback -count=1`
预期：FAIL，实际 name 是插件根路径

- [ ] **步骤 3：修改 launch**

把写死的 node 路径替换为 `node, _, ok := resolveNode(m.rootDir)`；!ok 时 return errors.New("no node runtime available")。

- [ ] **步骤 4：运行测试确认通过**

运行：`cd app/server && go test ./internal/plugin -run TestLaunchUsesSystemNodeFallback -count=1`
预期：PASS

- [ ] **步骤 5：Commit**

提交信息：`fix(plugin): launch plugins with probed node binary`

### 任务 4：runtime List 报告 system 运行时

**文件：**
- 修改：`test/plugin/testdata/gospeak.runtime/main.go` 的 List（约 71-107）
- 测试：`app/server/internal/plugin/runtime_test.go`

- [ ] **步骤 1：编写失败测试**

写 TestRuntimeListReportsSystemNode：root 空、PATH 注入假 node，startRuntime 后 List(kind=node)，断言存在 version=system 且 Active=true 的记录。

- [ ] **步骤 2：运行测试确认失败**

运行：`cd app/server && go test ./internal/plugin -run TestRuntimeListReportsSystemNode -count=1`
预期：FAIL，找不到 system 记录

- [ ] **步骤 3：修改 runtime List**

在 active 判定处追加 PATH 探测：LookPath("node") 且 --version 成功 → active="system"；遍历 entries 为空且 kind=node 且 active=system 时追加一条 RuntimeVersion{Kind:"node",Version:"system",Active:true}。import os/exec。

- [ ] **步骤 4：运行测试确认通过**

运行：`cd app/server && go test ./internal/plugin -run TestRuntimeListReportsSystemNode -count=1`
预期：PASS

- [ ] **步骤 5：运行整包测试**

运行：`cd app/server && go test ./internal/plugin -count=1`
预期：PASS

- [ ] **步骤 6：Commit**

提交信息：`feat(plugin): report system node runtime in List`

### 任务 5：全量验证与文档

- [ ] **步骤 1：Go 全量验证**

运行：`cd app/server && go build ./... && go vet ./... && go test ./... -count=1`
预期：全部通过

- [ ] **步骤 2：文档同步**

修改 `app/docs/guide/plugin.md` 的「运行时版本选择」节与 `deploy/DEPLOY.md` 的插件节，说明：插件根选装优先，未安装时复用系统 PATH 的 node；两者都没有时服务正常启动但插件跳过；仍不自动下载。

- [ ] **步骤 3：Commit**

提交信息：`docs(plugin): document node runtime PATH fallback`