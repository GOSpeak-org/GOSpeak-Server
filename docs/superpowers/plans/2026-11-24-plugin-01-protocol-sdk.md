# 插件平台 01：基础协议与 JS SDK Implementation Plan

> 【状态】2026-11-27 核验：该计划已交付（插件平台已从 feature-plugin-platform 迁移），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 定义插件系统全部 gRPC 契约并提交生成物；提供 JS SDK 让插件作者零接触 grpc-js/proto/端口。

**Architecture:** proto 是跨语言单一事实源（Registration/PluginAPI/Hook/通用 RuntimeService，python kind 预留）；JS SDK 内置 proto + grpc-js，封装 createPlugin() API；示例插件作为模板。

**Tech Stack:** Go 1.27.4 · protoc · protoc-gen-go/grpc · TypeScript · grpc-js · @grpc/proto-loader · Vitest

**Spec:** docs/superpowers/specs/2026-11-24-plugin-subprocess-design.md
**依赖本计划的下游:** 02（运行时与进程管理）、03（插件能力与管理面）

## Global Constraints

- Go 代码 snake_case/PascalCase/import 三组；无注释（复杂逻辑除外）、无 emoji；proto 生成物提交，CI proto-check。
- 用户标识一律 *_uuid；RuntimeService 为通用 Install/List/Use(kind,version)，kind=python 返回 unsupported。
- JS SDK 全封装：插件作者只接触 createPlugin()，零 grpc-js/proto/端口/凭据文件。
- 从 origin/release 签 feature/plugin-protocol 分支；Conventional Commits；PR 前 lint/typecheck/build。

## Review Focus

- proto 字段全 *_uuid、写请求含 reason/request_id（Task 1 测试）。
- SDK 能完成 Register + 调 GetUser（Task 2 测试）；凭据文件读取正确（Task 2 测试）。
- CI proto-check 能拦截生成物漂移（Task 1 Step 3）。

## File Structure

- `app/server/plugin/proto/plugin.proto` + `generate.go` + `app/server/internal/plugin/gen/*.pb.go`
- `app/server/go.mod`（grpc/protobuf direct）、`.github/workflows/ci.yml`（proto-check job）
- `packages/plugin-sdk/{package.json,src/index.ts,proto/,test/}`
- `test/plugin/testdata/com.example.audit/{metadata.yaml,package.json,index.js}`

---

### Task 1: proto 契约与生成物

**Files:** Create `plugin/proto/plugin.proto`、`plugin/proto/generate.go`、`internal/plugin/gen/*.pb.go`；Modify `go.mod`、`ci.yml`

**Interfaces:** Produces `RegistrationService.Register({plugin_id,name,version,hook_types,pages})→{api_endpoint,confirmed_pages}` + Heartbeat；`PluginAPIService{GetUser,ListRooms,MuteUser,CreateRoom,GetConfig,KVSave,KVLoad,ReportError}`；`PluginHookService.HandleEvent`；`RuntimeService{Install(kind,version,mirror?),List(kind?),Use(kind,version)}`

- [ ] Step 1: 写 proto（字段 *_uuid，写请求 reason/request_id；RuntimeService kind 字段含 python 预留）
- [ ] Step 2: protoc 确定性生成（paths=source_relative, go_package=GOSpeak/internal/plugin/gen）
- [ ] Step 3: CI proto-check job（重新生成 + git diff --exit-code；Go 单测不依赖 protoc）
- [ ] Step 4: Commit `feat(plugin): proto contract with runtime and plugin services`

### Task 2: JS SDK 与示例插件模板

**Files:** Create `packages/plugin-sdk/{package.json,src/index.ts,proto/}`、`test/plugin/testdata/com.example.audit/{metadata.yaml,package.json,index.js}`

**Interfaces:** Produces `createPlugin({onEvent}) → {api.getUser/listRooms/muteUser/createRoom, on(event,handler), config, kv, reportError}`；SDK 内部读 GOSPEAK_PLUGIN_CREDENTIAL 拿 127.0.0.1:<port>，内置 proto，零 grpc-js/proto 暴露

- [ ] Step 1: 失败测试（SDK 连本地 gRPC server 完成 Register + 调 GetUser；凭据读取正确）
- [ ] Step 2: 实现 SDK（grpc-js + proto 内置 + 端口凭据读取 + API/Hook/Config/KV/Error 封装）
- [ ] Step 3: 实现示例插件模板（用 SDK 写最小审核插件，metadata.yaml 含 runtime: node 预留字段）
- [ ] Step 4: `pnpm --filter @gospeak/plugin-sdk test` PASS
- [ ] Step 5: Commit `feat(plugin-sdk): JS SDK and example plugin template`