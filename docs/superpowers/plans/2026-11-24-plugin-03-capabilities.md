# 插件平台 03：管理面与插件能力 Implementation Plan

> 【状态】2026-11-27 核验：该计划已交付（管理 API / 插件 API / Hook / 页面挂载已落地），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 交付插件管理 API + WebUI（装/升/卸/重载/开关 + 运行时选择）、插件 API 服务（读写/配置/KV/审计）、Hook 事件推送、页面挂载——插件系统对用户完全可用。

**Architecture:** 管理接口走 protected + 现有管理员角色；APIService 写操作以系统用户委托 service + 审计 ActorKind=plugin；HookDispatcher 订阅 bus 内部事件白名单裁剪；页面通过 spa 反代 + iframe 动态路由挂载。

**Tech Stack:** Go 1.27.4 · Gin · gRPC-Go · 内嵌 NATS · TanStack Router · Vitest

**Spec:** docs/superpowers/specs/2026-11-24-plugin-subprocess-design.md
**依赖:** 计划 01（proto/SDK）、计划 02（数据表/runtime/Manager）；**下游:** 计划 04（集成验证）

## Global Constraints

- Go 代码规范同项目；*_uuid 标识；审计旁路；无新增权限码（复用管理员角色）。
- 管理 API遵循 pkg.Success/Fail 约定；Hook 5s 超时 + 连续 10 次失败熔断 disable。
- 页面路由强制 /plugins/<plugin_id>/ 前缀；iframe sandbox + origin/source/type 校验 + CSP。
- 从 origin/release 签 feature/plugin-capabilities 分支；Conventional Commits；PR 前 lint/typecheck/build。

## Review Focus

- 管理 API 无权限 403、runtime install{python} unsupported、enable/disable 持久化（Task 1）。
- 写失败不留虚假审计、request_id 幂等、配置/KV 按 plugin_id 隔离、ReportError 入日志（Task 2）。
- Hook 白名单敏感事件不推、超时/错误计数触发熔断（Task 3）。
- 页面非法前缀拒绝、未确认页面不挂载、iframe 伪造 origin 拒绝（Task 4）。

## File Structure

- `internal/router/routes/plugin/routes.go`、`internal/handler/plugin_handler.go`、`internal/service/plugin_service.go`
- `app/web/src/pages/(app)/manage/plugins/index.tsx`、`routeTree.gen.ts`（tsr generate）
- `internal/plugin/{api,hook,pageproxy,browser_proxy}.go` + 对应 *_test.go
- `app/web/src/pages/(app)/plugins/$pluginId/$.tsx`、`app/web/vite.config.ts`（dev proxy）

---

### Task 1: 管理 HTTP 接口与管理页面

**Files:** Create `internal/router/routes/plugin/routes.go`、`internal/handler/plugin_handler.go`、`internal/service/plugin_service.go`、`app/web/src/pages/(app)/manage/plugins/index.tsx`；Modify `internal/router/router.go`、`server/gin.go`、`routeTree.gen.ts`（tsr generate）

**Interfaces:** 管理 API：POST install/upgrade/reload/uninstall/enable/disable/config/runtime/install/runtime/use；GET list/runtime/list（protected + 管理员角色）；页面：插件列表 + 运行时选择（Node 可用/Python tab 预留提示）+ 安装入口

- [ ] Step 1: 失败测试（install 拉取+启动；runtime install{node}/use；runtime install{python} unsupported；enable/disable 持久化；无权限 403）
- [ ] Step 2: 实现 handler/service/routes（pkg.Success/Fail）
- [ ] Step 3: 实现管理页面 + tsr generate 提交 routeTree.gen.ts
- [ ] Step 4: `go test ./internal/handler/... && pnpm --filter @gospeak/web typecheck` PASS
- [ ] Step 5: Commit `feat(plugin): management API and admin page`

### Task 2: 插件 API 服务（配置/KV/错误上报 + 审计）

**Files:** Create `internal/plugin/api.go`、`api_test.go`；Modify `server/gin.go`

**Interfaces:** Produces `NewAPIService(userSvc,roomSvc,msgSvc,muteSvc,auditSvc,configRepo,kvRepo,dedup)`；写操作以 PluginSystemUserUUID 委托 service + 审计 ActorKind=plugin

- [ ] Step 1: 失败测试（GetUser 读；MuteUser 委托+审计；写失败不留审计；request_id 幂等；GetConfig/KV 按 plugin_id 隔离；ReportError 入日志）
- [ ] Step 2: 实现 APIService（幂等 → service → 成功审计；配置/KV/错误上报）
- [ ] Step 3: `go test ./internal/plugin/... -run TestAPI` PASS
- [ ] Step 4: Commit `feat(plugin): read/write API with config, KV and audit`

### Task 3: Hook 事件推送

**Files:** Create `internal/plugin/hook.go`、`hook_test.go`；Modify `server/gin.go`

**Interfaces:** Produces `HookDispatcher{allowedEvents map[string]bool}`，订阅 bus → gRPC HandleEvent；5s 超时 + 连续 10 次失败 disable

- [ ] Step 1: 失败测试（白名单推送；敏感事件不推；未订阅不推；超时/错误计数触发熔断）
- [ ] Step 2: 实现 HookDispatcher（消息创建/用户加入离开白名单 + payload 裁剪 + 超时 + 熔断）
- [ ] Step 3: `go test ./internal/plugin/... -run TestHook` PASS
- [ ] Step 4: Commit `feat(plugin): hook dispatch with allowlist and circuit breaker`

### Task 4: 插件页面挂载

**Files:** Create `internal/plugin/pageproxy.go`、`browser_proxy.go`、`pageproxy_test.go`；Modify `internal/router/router.go`、`app/web/src/pages/(app)/plugins/$pluginId/$.tsx`、`routeTree.gen.ts`、`vite.config.ts`

- [ ] Step 1: 失败测试（spa 代理优先于 SPA fallback；非法前缀拒绝；未确认页面不挂载；iframe 伪造 origin 拒绝）
- [ ] Step 2: 实现 spa 反代（复用 proxyrules）+ 浏览器代理端点（用户 JWT 校验）
- [ ] Step 3: 前端通用动态路由 + iframe + tsr generate + vite dev proxy
- [ ] Step 4: `go test ./internal/plugin/... && pnpm --filter @gospeak/web typecheck` PASS
- [ ] Step 5: Commit `feat(plugin): mount plugin pages`