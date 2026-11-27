# 域文件夹与个人布局持久化 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让每个用户把已加入的域组织成手机式的命名文件夹（含未分组），支持拖拽与菜单两种操作排序/移动，布局由后端按用户持久化。

**架构：** 新增 personal-layout 两张表（folders + items），与 domain_members 解耦；后端用「整份替换」API 在单事务内校验并保存，读取时按成员关系补齐缺失域；前端 zustand 门面本地即时更新 + 防抖保存，侧栏用文件夹图标 + Popover 浮层 + dnd-kit，菜单作为可访问性兜底。

**技术栈：** Go 1.27 / Gin / GORM / SQLite · React 19 / TypeScript / zustand / react-query / Radix / @dnd-kit/core + sortable / Biome / Vitest

参考规格：`docs/superpowers/specs/2026-11-21-domain-folder-layout-design.md`

---

## 文件结构

**后端（新建）**
- `app/server/internal/model/user_domain_layout.go` — 两实体 + TableName
- `app/server/internal/model/user_domain_layout_test.go` — 表名/字段约束
- `app/server/internal/repository/user_domain_layout_repo.go` — user 维度 CRUD、事务替换、补齐/清理
- `app/server/internal/repository/user_domain_layout_repo_test.go` — 仓储层测试
- `app/server/internal/service/domain_layout_service.go` — 校验 + 规范化 + 补齐 + 事务
- `app/server/internal/service/domain_layout_service_test.go` — 规则测试（含跨用户隔离）
- `app/server/internal/handler/domain_layout_handler.go` — get/save 两个入口
- `app/server/internal/handler/domain_layout_handler_test.go` — 用户隔离/参数校验

**后端（修改）**
- `app/server/internal/repository/db_migrations.go` — 注册 AutoMigrate
- `app/server/internal/router/routes/domain/routes.go` — 新增两条 POST（Register 需接收 layout handler）
- `app/server/internal/router/router.go` — 传递新 handler（若签名变化）
- `app/server/server/gin.go` — repo → service → handler 装配

**前端（新建）**
- `app/web/src/api/domainLayout.ts` — types + API 函数
- `app/web/src/utils/domainLayout.ts` — 纯函数：move/reorder/normalize/deleteGroup
- `app/web/src/utils/domainLayout.test.ts` — 纯函数单测
- `app/web/src/stores/domainLayoutStore.ts` — 双入口 store + 防抖保存
- `app/web/src/stores/domainLayoutStore.test.ts` — store 单测
- `app/web/src/components/domain/DomainGroupIcon.tsx` — 文件夹缩略图标
- `app/web/src/components/domain/DomainGroupPopover.tsx` — 浮层：组内域排序 + 重命名/改色/删除 + 移出
- `app/web/src/components/domain/DomainLayoutContextMenu.tsx` — 右键/长按菜单兜底
- `app/web/src/components/domain/DomainGroupColor.ts` — 颜色枚举 token

**前端（修改）**
- `app/web/src/layouts/common/sidebar.tsx` — 用文件夹/未分组结构替换平铺列表，接入 DndContext
- `app/web/package.json` / lockfile — 新增 @dnd-kit 依赖（用 pnpm add）

**集成测试与文档**
- `test/domain_layout/aggregate.test.ts` — GET→SAVE→GET 往返 + 跨用户隔离
- `app/server/docs/swagger.yaml` — 补充两条接口（若生成方式允许）
- `agent_test_logs/domain-folder-layout-2026-11-21.md` — 测试记录

---

## 任务 1：后端数据模型与迁移

**文件：** 创建 `model/user_domain_layout.go`、`model/user_domain_layout_test.go`；修改 `repository/db_migrations.go`
- [ ] 定义 `UserDomainGroup`（ID/UUID/UserUUID/Name/Color/SortOrder/时间戳，TableName=user_domain_groups）与 `UserDomainLayoutItem`（ID/UserUUID/DomainUUID/GroupUUID sql.NullString/SortOrder，TableName=user_domain_layout_items，唯一键 user+domain）。
- [ ] 写 model 单测断言 TableName 与字段；跑 `pnpm test:server:go -- --run TestUserDomainLayout` 通过。
- [ ] 在 `db_migrations.go` 的 AutoMigrate 列表加入两个模型；跑 `go build ./app/server/...` 通过。
- [ ] Commit：`feat(server): add user domain layout models`

## 任务 2：后端仓储

**文件：** 创建 `repository/user_domain_layout_repo.go` 与测试
- [ ] 实现 `ListGroups(userUUID) ([]model.UserDomainGroup, error)`、`ListItems(userUUID) ([]model.UserDomainLayoutItem, error)`、`Replace(userUUID string, groups []model.UserDomainGroup, items []model.UserDomainLayoutItem) error`（单事务：按 group uuid upsert、删除缺失组、删除并重建 items）。
- [ ] 实现 `JoinedDomainUUIDs(userUUID) ([]string, error)`（从 domain_members 取）。
- [ ] 测试：repo 单测（用现有测试 DB helper）验证 upsert 保留字段、删除缺失组、items 覆盖、user 隔离。
- [ ] Commit：`feat(server): add domain layout repository`

## 任务 3：后端服务层

**文件：** 创建 `service/domain_layout_service.go` 与测试
- [ ] 定义对外 DTO：`LayoutGroup{UUID,Name,Color,SortOrder}`、`LayoutItem{DomainUUID,GroupUUID,SortOrder}`、`Layout{Groups,Items}`。
- [ ] `GetLayout(userUUID) (*Layout, *pkg.AppError)`：读 groups/items + 已加入域，补齐缺失域为未分组（sort_order 接末尾），过滤已退出域；groups 按 sort_order 升序。
- [ ] `SaveLayout(userUUID string, layout *Layout) (*Layout, *pkg.AppError)`：校验（名称 trim 1–16、唯一、≤20、颜色白名单、domain 已加入、group 属于本人）→ 重排 sort_order → 调 repo.Replace → 返回 GetLayout 规范化结果；任一步失败返回 AppError。
- [ ] 测试：空布局补齐、非法名称/颜色/重名/超限/他人 group/未加入 domain 拒绝、sort_order 归一化、删除组后成员转未分组、用户 A/B 隔离。
- [ ] Commit：`feat(server): add domain layout service`

## 任务 4：后端 handler、路由与 DI

**文件：** 创建 `handler/domain_layout_handler.go` 与测试；修改 routes/domain/routes.go、router/router.go、server/gin.go
- [ ] Handler 两方法：`Get(c *gin.Context)`、`Save(c *gin.Context)`；`middleware.CurrentUserUUID(c)` 为空 → `pkg.Fail(c, pkg.INVALID_PARAMS, ...)`；binding JSON 失败同；调用 service；`pkg.Success` / `pkg.HandleError`。
- [ ] routes：在 protected 组注册 `POST /domain/layout/get`、`POST /domain/layout/save`（无 RequireDomainMember，因为是个人布局）。
- [ ] gin.go：新建 repo/service/handler 并按顺序装配，传入 router.Register。
- [ ] Handler 测试：未登录拒绝、get 返回当前用户布局、save 只影响当前用户。
- [ ] 跑 `pnpm test:server:go` 与 `go vet`；Commit：`feat(server): expose domain layout endpoints`

## 任务 5：前端 API、工具与 store

**文件：** 创建 api/domainLayout.ts、utils/domainLayout.ts + test、stores/domainLayoutStore.ts + test
- [ ] types 对齐后端 DTO；apiClient.post 调 get/save。
- [ ] 纯函数：`normalizeLayout`、`moveDomain(layout, domainUUID, groupUUID, index)`、`reorderGroup`、`reorderDomain`、`deleteGroup`（成员转未分组）、`colorOptions`。
- [ ] store：state + actions；`load()` 失败回退空布局、不抛错；变更立即更新本地 + 400ms 防抖 `save(layout)`；保存失败 toast（沿用现有错误通道）且不清本地。
- [ ] 单测：纯函数各分支 + store 的 move/reorder/deleteGroup/失败回退（mock api）。
- [ ] 跑 `pnpm typecheck` 与 `pnpm test:web`；Commit：`feat(web): add domain layout api and store`

## 任务 6：前端侧栏 UI（文件夹图标 + 浮层 + DnD + 菜单）

**文件：** 创建 components/domain/DomainGroupIcon.tsx、DomainGroupPopover.tsx、DomainLayoutContextMenu.tsx、DomainGroupColor.ts；修改 layouts/common/sidebar.tsx；`pnpm add @dnd-kit/core @dnd-kit/sortable` 于 app/web
- [ ] 文件夹图标：48px 圆角、枚举色、数量角标，tooltip 显示名称；复用 token 与 lucide Folder 图标。
- [ ] Popover：组内域用 SortableContext 排序；头部重命名（input）+ 颜色选择（枚举）+ 删除；底部“移出到未分组”投放区。
- [ ] ContextMenu：域上“移动到…/移出分组/上移/下移”；文件夹上“重命名/改色/删除/上移/下移”。
- [ ] Sidebar：顶层 DndContext 排序文件夹与未分组域；`myDomainUUIDs` 变化时 `load()`；保留双击进入域行为。
- [ ] 跑 `pnpm typecheck`、`pnpm test:web`、`pnpm lint`；Commit：`feat(web): render domain folders in sidebar`

## 任务 7：集成验证与记录

**文件：** 创建 test/domain_layout/aggregate.test.ts、agent_test_logs/domain-folder-layout-2026-11-21.md；更新文档若有注解要求
- [ ] 集成测试：GET 空 → SAVE → GET 往返一致；用户 A 保存后用户 B GET 不到。
- [ ] 全量验证：`pnpm typecheck`、`pnpm lint`、`pnpm test`、`pnpm test:server:go`、`pnpm test:server`（若环境允许）、`pnpm build`。
- [ ] 写 agent 测试记录；Commit：`test: cover domain folder layout`
