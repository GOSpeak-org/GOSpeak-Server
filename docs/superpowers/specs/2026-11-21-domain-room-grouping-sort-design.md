# 域内房间分组与排序：可折叠分组 + 两处配置入口

> 【状态】2026-11-27 核验：该设计主体已落地（分组与排序已交付），仅供历史参考。

日期：2026-11-21
分支：dev-1121

## 背景与目标

当前域内房间在两处展示，都没有分组与自定义排序：

- 左侧房间列表 `app/web/src/components/room/roomList.tsx` 从 WS `room:list:result` 取数，`visibleRoomsForDomain` 仅按 `domain_uuid` 过滤；顺序由服务端 `app/server/internal/signal/hub_room_list.go` 按 `CreatedAt` 升序 + `ID` 兜底决定，用户无法调整。
- 域管理页 `app/web/src/pages/(app)/manage/domains/$domainUUID/index.tsx` 的「房间管理」用 HTTP `/api/v1/room/list` 做分页表格，同样没有分组概念。

目标：允许平台管理员、域 owner/admin 以及被授予专用权限的自定义域角色，为某个域自定义房间的**分组**与**排序**；分组在侧边栏以可折叠标题展示，配置在管理页和侧边栏两处都能进行，结果对所有成员一致生效。

## 已确认的决策

| 议题 | 决策 |
| --- | --- |
| 分组呈现形态 | Discord 风格可折叠分组标题，组内房间可排序 |
| 生效范围 | 域级全局：所有成员看到同样的分组与顺序，由有权限者统一维护 |
| 配置入口 | 两处都做：管理页维护分组结构，侧边栏提供就地快捷拖拽排序 |
| 权限模型 | 新增域内权限 `domain:room-layout:manage`；平台 admin、域 owner/admin 默认拥有，可分配给自定义域角色 |
| 归属关系 | 单归属；未分配房间平铺展示（不加分组标题），但作为整体区块可拖到任意分组之前或之后 |
| 排序粒度 | 两级拖拽：分组之间换序，组内房间换序，未分组区块内部也可排序且区块位置可调 |
| 技术方案 | 规范化表 + 全量布局保存 + WS `RoomInfo` 携带分组信息 |

## 非目标

- 不做个人级排序偏好：折叠状态是本地 UI 状态，但分组结构和顺序全局唯一。
- 不引入房间多归属（一个房间同时出现在多个分组）。
- 不在首版做乐观锁；并发保存采用服务端全量替换、后写覆盖。
- 不改变平台级角色权限体系（`role:*`），新权限只进域内可分配白名单。
- 不在房间创建/编辑弹窗新增「所属分组」字段；分组归属只在两处配置入口维护。

## 设计

### A. 数据模型与迁移

三处存储变更，均需在 `app/server/internal/repository/db_migrations.go` 的 `AutoMigrate` 注册：

1. 新表 `domain_room_groups`（model `DomainRoomGroup`）：`ID uint` 主键、`UUID string`（`type:uuid;uniqueIndex`）、`DomainUUID string`（`size:32`）、`Name string`（`size:64`）、`SortOrder int`（默认 0）、`CreatedAt` / `UpdatedAt`。唯一索引 `(domain_uuid, name)`，普通索引 `(domain_uuid)`，UUID 作为对外稳定标识。
2. `room` 表加两列：`GroupUUID string`（`size:36;index`，空串表示未分组）与 `SortOrder int`（`not null;default:0`，组内顺序）。
3. 新表 `domain_room_layouts`（model `DomainRoomLayout`）：`DomainUUID string` 主键、`UngroupedPosition int`（默认 0，未分组区块在分组序列中的位置，0 表示最前，等于分组数表示最后）、`UpdatedAt`。

迁移默认行为：存量房间 `group_uuid=''`、`sort_order=0`，全部落入未分组区块；`ungrouped_position` 初始化为分组数（即最后），保证升级后展示顺序与现状一致。

### B. 权限

- `app/server/internal/permcode/domain_permcode.go` 新增 `PermDomainRoomLayoutManage = "domain:room-layout:manage"`。
- `app/server/internal/model/permission.go`：`DefaultPermissions` 增加 `{Code: PermDomainRoomLayoutManage, Name: "管理房间分组与排序", ...}`；`DefaultRolePermissions["admin"]`（平台 admin）增加该码。
- `app/server/internal/model/domain_role.go`：`AssignableDomainPermissions` 增加该码（自定义域角色可授予）；`DefaultDomainRolePermissions[DomainRoleAdmin]` 增加该码。
- 判定口径与域管理页一致：`isOwner || 平台 hasPermission(code) || hasDomainPermission(uuid, code)`；域 `member` 默认无此权限，只读。
- 前端 `docs/permissions.md` 补充说明；权限名称来自 `listPermissions`，种子同步后在 `DomainRolePanel` 的 `domain` 分组自动可见。

### C. API 契约

挂在 `/api/v1/domain` 下，路由用 `middleware.RequireDomainMember()`，再由 handler 按 B 节口径判定配置权限。风格沿用 `{code,msg,data}` 与 `pkg.NewAppError`。

- `POST /domain/room-layout/get`
  - 请求：`{ "domain_uuid": "..." }`
  - 响应 data：`{ "groups": [ { "uuid","name","sort_order","rooms":[RoomBrief] } ], "ungrouped": [RoomBrief], "ungrouped_position": number, "can_manage": boolean }`
  - `RoomBrief` 至少含 `uuid / name / type / count / has_password / sort_order`，供管理页展示。
- `POST /domain/room-layout/save`
  - 请求：`{ "domain_uuid": "...", "groups": [ { "uuid"?: string, "name": string, "sort_order": number, "room_uuids": string[] } ], "ungrouped_position": number, "ungrouped_room_uuids": string[] }`
  - 语义：一次提交整份布局，事务内原子替换。
  - 校验：房间必须属于该域；分组名 trim 后非空且域内唯一；`uuid` 为空视为新建分组，非空必须存在且属于该域；`room_uuids` 内房间不得重复；`ungrouped_position` 裁剪到 `[0, len(groups)]`。
  - payload 中缺失的分组删除，其下房间转为未分组；不在任何组且不在 `ungrouped_room_uuids` 中的房间保持未分组。
  - 响应 data：保存后的完整布局（同 get），便于前端直接对齐。

### D. 后端分层与 WS 协议

- 分层：新增 `RoomLayoutService`（`GetLayout` / `SaveLayout`，事务内完成分组 CRUD、房间 `group_uuid`/`sort_order` 批量更新、`ungrouped_position` 写入；返回 `*AppError`，不 import gin）；新增 `domain_room_group_repo` / `domain_room_layout_repo`；`room_repo` 扩展按 uuid 批量更新归属与顺序。
- Handler：新增 `RoomLayoutHandler`，只做绑定、权限判定、调用 service、`pkg.Success` / `pkg.HandleError`；依赖在 `app/server/server/gin.go` 装配。
- 路由：在 `app/server/internal/router/routes/domain/routes.go` 注册上述两个端点。
- WS 协议（单一事实源四步）：
  1. `app/server/internal/protocol/payloads.go` 的 `RoomInfo` 增加 `GroupUUID string`（json `group_uuid,omitempty`）与 `SortOrder int`（json `sort_order`）；
  2. `RoomListResultPayload` 增加 `Layout` 字段（`{ groups: [{uuid,name,sort_order}], ungrouped_position }`），让侧边栏一次拿到分组标题与区块顺序，无需额外 HTTP 请求；
  3. `hub_room_list.go` 注入只读 layout store，DB 房间构造 `RoomInfo` 时复制两字段；内存活跃房间按未分组处理；
  4. 排序从 `CreatedAt` 升序改为：按 `ungrouped_position` 与分组 `sort_order` 确定区块顺序 → 区块内按 `sort_order` 升序 → `CreatedAt` → `ID` 兜底；
  5. `pnpm gen:protocol` 重新生成 `app/web/src/protocol/generated/` 三个产物并提交。
- 广播：`SaveLayout` 成功后由 handler 调 `Hub.BroadcastRoomList(domainUUID)`，与房间增删改一致，所有在线成员立即收到新顺序。

### E. 管理页配置面板

- 在域详情页「房间管理」Section 内（或紧邻新增 Section）加「分组与排序」面板，API 在 `app/web/src/api/domain.ts` 新增 `getRoomLayout` / `saveRoomLayout`。
- 使用仓库已有的 `@dnd-kit/core` + `@dnd-kit/sortable` 实现：分组标题拖拽换序、房间跨分组移动与组内排序、未分组区块拖到任意位置；分组新增/改名（内联 Input + Enter 提交）/删除；底部整体「保存布局」按钮。
- `can_manage=false` 时只渲染只读布局，不出现拖拽手柄与编辑控件。
- 保存走乐观更新：先更新本地 state，失败回滚并 toast；成功后无需手动刷新 WS 房间列表（服务端会广播）。
- UI 遵循 token 类名，图标用 `lucide-react`；Hooks 依赖门禁只许减少不许新增告警。

### F. 侧边栏渲染与快捷拖拽

- `roomList.tsx` 从 WS `rooms` 按 `group_uuid` 分块渲染；每个分组一个可折叠标题，未分组区块没有标题、平铺展示但可整体移动位置。折叠状态存本地 zustand 或组件 state，不写服务端。
- 有配置权限的用户在 `RoomListHeader` 出现「编辑布局」开关：开启后侧边栏内可直接拖拽分组与房间，拖拽结束调用同一个 `/domain/room-layout/save` 提交，保证与管理页一致。
- 虚拟列表适配：把分组标题行与房间行统一拍平成虚拟项计入 `useVirtualizer` 的 `count`，继续用 `measureElement` 动态测量；行 key 用 `group:<uuid>` / `room:<domain_uuid>:<name>` 区分。
- 既有 `visibleRoomsForDomain` 仍负责按域过滤，新增 `groupRoomsForSidebar` 纯函数负责分块，便于单测。

### G. 边界情况

| 情况 | 行为 |
| --- | --- |
| 删除房间 | 从布局与 WS 列表同时移除，无需手动清理组配置 |
| 删除分组 | 其房间转为未分组，未分组区块位置不变 |
| 房间归属其他域 | save 校验拒绝，返回 `INVALID_PARAMS` |
| 分组重名 | save 校验拒绝 |
| 无权限用户拖拽 | UI 不出现编辑控件；服务端二次校验，越权返回 `FORBIDDEN` |
| 内存活跃但无 DB 记录的房间 | 视为未分组，`sort_order=0` |
| 并发保存 | 全量替换，后写覆盖（首版不做乐观锁） |
| 折叠状态 | 仅本地，不影响其他成员 |

## 测试

- Go 单测：`RoomLayoutService` 的 get/save 事务与校验（重名、跨域房间、空分组名、位置裁剪）、WS `hub_room_list` 的分组排序（区块顺序、组内顺序、未分组位置、兜底）。
- API 集成测试 `test/domain/`：读取/保存布局、权限（owner / 域 admin / 自定义角色 / 普通成员）、非法归属拒绝、删除分组后房间转未分组。
- 前端单测：`roomListUtils` 的分块纯函数、管理页面板的拖拽与保存交互、侧边栏折叠与编辑开关的权限显隐。
- 门禁：`pnpm gen:protocol`（含 `gen_contract_test.go` 新鲜度校验）、`pnpm typecheck`、`pnpm test`、`pnpm lint`、Swagger 重新生成。

## 工作区注意

当前 `dev-1121` 工作区已有未提交改动，且其中 `app/web/src/components/room/components/roomItem.tsx`、`app/web/src/pages/(app)/manage/domains/$domainUUID/index.tsx`、`DomainRolePanel.tsx` 等文件与本功能重叠。实现时必须保留这些既有改动，只在其上叠加本功能的修改，禁止用整体覆盖写入。
