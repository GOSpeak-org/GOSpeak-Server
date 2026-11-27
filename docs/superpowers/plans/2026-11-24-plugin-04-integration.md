# 插件平台 04：集成验证 Implementation Plan

> 【状态】2026-11-27 核验：该计划已交付（插件指标与集成验证已落地），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 补齐插件可观测性指标，建立端到端集成测试（runtime 无 node panic → 页面选版本 → 市场拉取插件 → 注册 → 读写/配置/KV → 页面 → 升级/重载/卸载 → 关闭 → 审计），跑通全量门禁。

**Architecture:** 在 metrics Snapshot → monitor_handler → server_stats 链路加 gospeak_plugin_ 指标；改造 test/globalSetup 预置插件目录与 Node 缓存（避免测试联网）；用本地示例仓库模拟市场/GitHub 来源。

**Tech Stack:** Go 1.27.4 · Prometheus client_golang · Vitest · test/globalSetup.ts

**Spec:** docs/superpowers/specs/2026-11-24-plugin-subprocess-design.md
**依赖:** 计划 01（proto/SDK）、02（runtime/Manager）、03（管理面/能力）全部完成

## Global Constraints

- 指标名 gospeak_plugin_ 前缀，plugin_id 为低基数标签；不改现有指标语义。
- 集成测试不得依赖外网（预置 node 缓存与本地 git 仓库 fixture）。
- 全量门禁必须通过：pnpm lint / typecheck / test:server:go / test / build。
- 从 origin/release 签 feature/plugin-e2e 分支；Conventional Commits。

## Review Focus

- 端到端覆盖 runtime panic → 人工选版本 → 安装 → 运行 → 升级/重载/卸载 → 关闭 → 审计全链路。
- globalSetup 预置环境后测试可离线跑通；指标链路三处（metrics/handler/stats）齐全。

## File Structure

- `internal/metrics/{metrics.go,monitor_handler.go}`、`server/server_stats.go`
- `test/globalSetup.ts`、`test/plugin/`（E2E 用例 + 本地示例插件仓库 fixture）
- `docs/superpowers/plans/README.md`（更新四份计划状态）

---

### Task 1: 指标链路

**Files:** Modify `internal/metrics/metrics.go`（Snapshot 增插件字段 + gauge + MustRegister）、`internal/handler/monitor_handler.go`（HealthSnapshot 采集）、`server/server_stats.go`（toMetricsSnapshot 转换）

**Interfaces:** Produces gospeak_plugin_installed/active/restarts/api_calls/api_errors 指标；plugin_id 低基数标签；Manager 统计注入入口

- [ ] Step 1: 失败测试（Snapshot 含插件字段；指标名与标签符合约束；server_stats 转换传递）
- [ ] Step 2: 实现三处链路（metrics → monitor_handler → server_stats）+ Manager 统计注入
- [ ] Step 3: `go test ./internal/metrics/... ./server/...` PASS
- [ ] Step 4: Commit `feat(plugin): plugin metrics pipeline`

### Task 2: 端到端集成测试与离线夹具

**Files:** Modify `test/globalSetup.ts`；Create `test/plugin/`（E2E 用例 + 本地示例插件仓库 fixture）

- [ ] Step 1: 写 E2E 测试（runtime 无 node panic → 页面选版本下载 → 市场拉取本地仓库示例插件 → npm ci → 注册 → 读写/配置/KV → 页面访问 → 升级/重载/卸载 → 优雅关闭 → 审计断言）
- [ ] Step 2: 改造 globalSetup（准备 /app/plugins 的 bin/runtime 目录、预置 node 缓存与本地 git 仓库 fixture、注入端口凭据；避免联网）
- [ ] Step 3: `pnpm test:server` PASS（E2E 全链路）
- [ ] Step 4: Commit `test(plugin): end-to-end integration and offline fixtures`

### Task 3: 全量门禁与索引收尾

**Files:** Modify `docs/superpowers/plans/README.md`（四份计划状态）

- [ ] Step 1: 运行全量门禁 `pnpm lint && pnpm typecheck && pnpm test:server:go && pnpm test && pnpm build` PASS
- [ ] Step 2: 更新 plans README（01/02/03/04 状态与剩余项；删除已交付的旧 2026-11-24-plugin-subprocess.md 条目）
- [ ] Step 3: Commit `docs(plugin): update plan index after delivery`