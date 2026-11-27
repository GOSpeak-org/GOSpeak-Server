# Casbin 权威化 + 域策略失效改造 + 角色权限页域作用域

> 【状态】2026-11-27 核验：该设计已落地（Casbin 权威化与策略失效改造已交付），仅供历史参考。

日期：2026-11-18
分支：dev-1118（工作区已有其他未提交改动，本变更不触碰那些文件）

## 背景与目标

后端已用 Casbin 做 domain RBAC，但存在三个问题：

1. **性能**：`DomainService.HasDomainPermission` 每次判定都调用 `LoadPolicy()`，adapter 全表扫 users / domains / domain_members / domain_role_permissions，房间/消息高频路径开销大。
2. **双路径**：`useCasbin` 布尔 + DB/cache 回落分支仍在；生产路径虽恒为 true，但构造签名允许“无 Casbin 实例”，测试也大量依赖回落。
3. **前端缺口**：`/manage/roles`（角色权限页）只有平台角色与 `domain:*` 平台开关，无法配置“某个域内”的角色权限；真正的配置藏在 `/manage/domains/$domainUUID`。

目标：① 去掉每次判定的全量 reload，改为事件驱动失效；② 让 Casbin 成为唯一权威鉴权源；③ 角色权限页加“平台 / 指定域”作用域切换，复用现有 `DomainRolePanel` 编辑域内角色权限。

## 非目标

- 不改 `internal/policy` 的封禁判决（域级 policy 规则仍未实现，本次不碰）。
- 不做多 SFU、访客、集群等能力。
- 不删除域成员缓存 `memberCache`（与权限策略无关）。

## 设计

### A. 启动顺序（为权威化铺路）

- 拆分 `bootstrapControlPlane`：
  - `seedControlPlane(db, roleRepo, userRepo, permRepo, sfuConfigSvc) error`：只做 seedRoles / seedAdminUser / EnsureDefaultDomain / BackfillDomainRoleDefaults / seedPermissions / sfuConfigSvc.SyncFromEnv。
  - 从 `gin.go` 在构造 `permSvc` / `domainSvc` **之前**调用；`sfuConfigSvc` 构造相应提前。
- 删除 `bootstrap.go` 中 seed 后的二次 `UseCasbin`，删除 `gin.go` 早期那次“加载空表”的 `UseCasbin`。
- 改为构造器一次注入 adapter：`NewPermissionService(permRepo, adapter)` / `NewDomainService(domainRepo, roleRepo, inviteCreator, adapter)`，内部建 enforcer 并 LoadPolicy；失败返回 error。

### B. 事件驱动失效（性能修复）

- 新增内部事件 `EventDomainPolicyChanged`（`DirInternal`，payload `{domain_uuid}`），在 `protocol/events.go` 的 Catalog 与 payloadStructs / eventConsts 登记；`pnpm gen:protocol`（内部事件预期无 TS diff）。
- `DomainService` 新增 `ReloadPolicy() error`：调 `enforcer.LoadPolicy()`。失败时置 `policyStale=true` 并记录日志；`HasDomainPermission` 在 stale 状态下**一律拒绝**（不用旧策略放行未知变更），直到某次 reload 成功后清除标记。写操作本身已落库，不因 reload 失败回滚。
- `HasDomainPermission` 删除 `reloadDomainCasbin` 调用，只读内存策略。
- 本地失效：所有改策略的写操作成功后调 `ReloadPolicy()`：`Create`、`AddMember`、`Kick`、`Leave`、`TransferOwnership`、`CreateDomainRole`、`UpdateDomainRolePermissions`、`DeleteDomainRole`、`SetMemberRole`。
- 远端失效：上述操作同时 publish `EventDomainPolicyChanged`；`gin.go` dispatcher 注册 `domainSvc.ReloadPolicy`（无参数时全量重载）。
- `PermissionService.OnRemoteInvalidate` 补 `reloadCasbin()`（当前只 LoadCache，Casbin 权威后是漏洞）。
- `SyncRolePermissions` / `RenameRole` 已有 reload，保持不变。

### C. Casbin 权威化（删双路径）

- 删除 `useCasbin` 字段；`HasDomainPermission` / `HasPermission` 只走 enforcer，Enforce 出错 fail-closed。
- `PermissionService.cache` 保留给 `GetRolePermissions` 等读接口，不再作为鉴权 fallback；`LoadCache` / `OnRemoteInvalidate` 保留。
- `DomainService.GetDomainRolePermissions` / `MyDomainPermissions` 仍查 repo（Casbin 无“列出权限”查询），只作读模型。
- 迁移全部构造点：`NewDomainService` 16 处（14 测试）、`NewPermissionService` 8 处（6 测试）。测试统一 helper：用现有 sqlite 测试 DB + 真实 GORM adapter，或用 in-memory adapter 后 `AddPolicy`。
- 失败语义：数据库未初始化 / adapter 为 nil 时构造即失败，不再静默降级。

### D. 角色权限页域作用域（前端）

- 将 `pages/(app)/manage/domains/$domainUUID/components/DomainRolePanel.tsx` 移到 `components/manage/DomainRolePanel.tsx`（共享组件，不参与路由生成），更新域详情页 import。
- `pages/(app)/manage/roles/index.tsx` 顶部加作用域选择器：「平台」/ 用户已加入的域（复用 `myDomains` 查询）。
  - 平台：保留现有角色列表 + 权限勾选 + `syncRolePermissions`。
  - 指定域：拉 `listDomainRoles(uuid)`（返回含 `assignable`）、`getDomainRolePermissions`，渲染 `DomainRolePanel`，onCreate/onUpdate/onDelete 接 `domainApi` 的 roles/create、roles/update、members/update-role、roles/delete。
- 权限守卫：域作用域下用 `hasDomainPermission(uuid, "domain:role:manage")` 或平台 `domain:manage` 决定是否可编辑；只读时禁用操作按钮。
- 权限项展示：平台模式给 `domain:*` 权限码加“平台级”分组标签和说明，避免与域内权限混淆；域模式沿用 `DomainRolePanel` 的域/房间/消息分组。
- 路由不需要新增（仍是 `/manage/roles`）。

## 验证

```bash
cd app/server && go test ./...
pnpm gen:protocol && git diff --stat app/web/src/protocol/generated/   # 与改前基线比对；工作区已有 payloads.ts 改动，禁止直接 --exit-code
pnpm test:server
pnpm --filter @gospeak/web typecheck
pnpm --filter @gospeak/web test
```

新增/补充测试：
- `domain_casbin_test.go`：跨域隔离（已有）+ 写操作后策略立即生效、无全量 reload。
- `permission_service` 测试：Enforce 失败 fail-closed；remote invalidate 触发 Casbin reload。
- 前端：角色权限页作用域切换、域模式角色 CRUD 的交互测试。

## 风险与缓解

- 启动顺序变更：seed 必须在任何 service 使用前完成；用启动期冒烟测试覆盖（seed 后 admin 权限立即可用）。
- 失效遗漏：每个改策略的写路径都要 reload + publish；用表格逐条核对并加测试。
- 工作区已有其他未提交改动：只改本设计列出的文件，不 commit、不 stash、不 rebase。

## 实施补充记录（2026-11-18）

- **前端范围升级**：用户确认“权限页面需要重构”后，Task 5 从“加作用域切换”升级为“重新设计整页结构”：/manage/roles 改为「平台角色 / 域内角色」两个页签，平台权限按 domain:* / 其他分组展示并标注“平台级”，域内页签含域选择器、DomainRolePanel 与成员角色分配（DomainMemberTable）。DomainRolePanel 已移至 components/manage/ 供两处复用。
- **本地事件回环陷阱**：NATSBus.PublishInternal 的 RemoteHook 不含自身回环。原先 HasDomainPermission 每次判定 reload 掩盖了“成员写入后未本地失效”的问题；改为纯事件驱动后，邀请入域与入域审核两条路径（事务内直接写 domain_members）不会触发本地策略重载，导致消息读写 403。已为 InviteJoinService / JoinRequestService 增加 SetMemberAddedHook，在 gin.go 接到 domainSvc.OnMemberAdded，本地重载 + NATS 广播远端两步都保留。
- **验证**：go test ./...、pnpm test:server（17 文件 / 57 用例）、pnpm lint、web typecheck、web test（91 文件 / 489 用例）全部通过。

