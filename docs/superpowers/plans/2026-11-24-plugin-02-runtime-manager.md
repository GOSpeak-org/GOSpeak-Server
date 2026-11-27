# 插件平台 02：运行时与进程管理 Implementation Plan

> 【状态】2026-11-27 核验：该计划已交付（数据表 / runtime / Manager 已落地），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 建好插件数据层、runtime 插件（人工选版本、Node 实装/Python 预留）和 PluginManager（市场拉取 + 生命周期），产出能拉起插件并执行 Register 的可运行系统。

**Architecture:** 数据表 plugin_registrations/config/kv/request_log + 审计 ActorKind；runtime 插件实现通用 RuntimeService（无自动下载，管理员页面选版本）；Manager 负责市场/GitHub 拉取 → npm ci → exec → TCP 握手 → 心跳 → 优雅关闭。

**Tech Stack:** Go 1.27.4 · GORM · gRPC-Go · git clone/tarball · npm · 内嵌 NATS

**Spec:** docs/superpowers/specs/2026-11-24-plugin-subprocess-design.md
**依赖:** 计划 01（proto + JS SDK 已就绪）；**下游:** 计划 03（管理面与插件能力）

## Global Constraints

- Go 代码规范同项目根 AGENTS.md；用户标识 *_uuid；无插件 env；根卷硬编码 /app/plugins；127.0.0.1 动态端口。
- 拉取即信任：无白名单/验签/沙箱；runtime 零自动下载、无已装 Node → panic；kind=python 返回 unsupported。
- 写操作委托现有 service（系统用户 UUID）；审计旁路；业务插件失败只跳过。
- 从 origin/release 签 feature/plugin-runtime 分支；Conventional Commits；PR 前 lint/build。

## Review Focus

- 审计白名单接受插件动作、actor_kind 迁移、系统用户 UUID 非真实用户（Task 1）。
- runtime 启动不下载、校验失败不建软链、Install{node}/Use{node} 生效、Install{python} unsupported（Task 2）。
- 市场拉取 → npm ci → exec → Register 顺序；升级保留 state、卸载保留 state；业务失败跳过；无 node panic（Task 3）。
- 优雅关闭：Hook 停 → Manager.Stop → bus → audit（Task 3）。

## File Structure

- `internal/model/{plugin_registration,plugin_config,plugin_kv,plugin_request_log}.go`、`audit_log.go`、`seeds.go`
- `internal/repository/db_migrations.go`、`internal/audit/audit.go`、`deploy/docker-compose.yml`、镜像 Dockerfile（ENV PATH）
- `test/plugin/testdata/gospeak.runtime/main.go`（runtime 示例）、`internal/plugin/runtime_test.go`
- `internal/plugin/{manager,metadata,lifecycle}.go`、`manager_test.go`、`server/{gin.go,server_shutdown.go}`

---

### Task 1: 数据表、审计与系统用户

**Files:** Create `internal/model/{plugin_registration,plugin_config,plugin_kv,plugin_request_log}.go`；Modify `model/audit_log.go`（actor_kind）、`model/seeds.go`、`repository/db_migrations.go`、`audit/audit.go`、`deploy/docker-compose.yml`

**Interfaces:** Produces `PluginRegistration{ID,PluginID,Source,Version,Enabled,AllowedRoutePrefix}`；`PluginConfig/PluginKV/PluginRequestLog`；`audit.Entry.ActorKind`；`model.PluginSystemUserUUID`；compose 加 gospeak-plugins 卷 /app/plugins

- [ ] Step 1: 失败测试（审计白名单；actor_kind 列；系统用户 UUID 非真实用户）
- [ ] Step 2: 实现 model / 迁移 / 审计 / seeds / compose 卷
- [ ] Step 3: `go test ./internal/audit/... ./internal/repository/...` PASS
- [ ] Step 4: Commit `feat(plugin): tables, audit and system user`

### Task 2: runtime 插件（人工选版本 + Python 预留）

**Files:** Create `test/plugin/testdata/gospeak.runtime/main.go`、`internal/plugin/runtime_test.go`；Modify 镜像 Dockerfile（ENV PATH=/app/plugins/runtime/bin:$PATH）

**Interfaces:** runtime 启动不下载只 Register；Install({kind,version,mirror?})：node 下载 tarball→校验 SHASUMS256→解压 download/node/v<version>→软链 runtime/bin/node；python 返回 unsupported；List 列出；Use 切换软链；无已装 node 时主程序 panic

- [ ] Step 1: 失败测试（启动不下载；校验失败不建软链；镜像不可达报错；Install{node} 后 node --version 可执行；Use{node} 切换；Install{python} 返回 unsupported）
- [ ] Step 2: 实现 runtime 示例插件（下载/校验/解压/软链/版本列表；python unsupported）；镜像加 ENV PATH
- [ ] Step 3: `go test ./internal/plugin/... -run TestRuntime` PASS
- [ ] Step 4: Commit `feat(plugin): runtime plugin with manual version management`

### Task 3: PluginManager（市场拉取 + 生命周期）

**Files:** Create `internal/plugin/{manager,metadata,lifecycle}.go`、`manager_test.go`；Modify `server/{gin.go,server_shutdown.go}`

**Interfaces:** Produces `NewManager(rootDir string, register RegisterFunc) *Manager`；`Start(ctx) error`（runtime 握手失败/无 node → error → panic）；`Stop(ctx) error`；`metadata.go` 解析 metadata.yaml；`lifecycle.go` 实现 Install/Upgrade/Reload/Uninstall/Enable/Disable

- [ ] Step 1: 失败测试（无 node panic；runtime 握手失败 panic；市场拉取→npm ci→exec→Register 顺序；升级覆盖代码保留 state；卸载删代码保留 state；npm ci 失败跳过；心跳/退避/失败上限）
- [ ] Step 2: 实现 Manager（git clone/tarball + metadata + npm ci + exec + TCP server + 凭据文件 + 心跳 + 退避 + 生命周期接口）
- [ ] Step 3: gin.go 装配（bus.Init 后建 server、路由后 serveHTTP 前 Start；Start error → panic）；shutdown 挂接（Hook 停 → Manager.Stop grpc.GracefulStop 独立 10s ctx → bus 关 → audit.Stop）
- [ ] Step 4: `go test ./internal/plugin/... ./server/...` PASS
- [ ] Step 5: Commit `feat(plugin): manager with marketplace install and lifecycle`