# 域文件夹与个人布局持久化设计（2026-11-21）

## 背景与目标

左侧 64px 图标侧栏（`app/web/src/layouts/common/sidebar.tsx`）目前按 `myDomainUUIDs` 顺序平铺所有已加入的域。用户希望像手机桌面文件夹一样，由**每个用户自己**把域分组、命名文件夹、拖动排序，并且状态由**后端持久化**（跨设备/重登生效），而不是只存 localStorage。

已确认需求：
- 数据模型：文件夹（一个域只属于一个文件夹，另有“未分组”）。
- 交互：拖拽为主 + 右键/长按菜单兜底。
- 侧栏呈现：64px 侧栏显示文件夹缩略图标，点击弹浮层展示组内域，不撑宽侧栏。

## 方案选择

采用「整份布局替换」API：`GET /domain/layout` 返回全量，`POST /domain/layout/save` 提交全量 `{groups, items}`，后端在单事务内校验并覆盖。相比细粒度 CRUD 接口更少、拖拽后一次提交、易测试；个人 UI 状态下多标签页“最后写入生效”可接受。

## 数据模型（新增，注册进 `db_migrations.go` 的 AutoMigrate）

### `user_domain_groups`
| 字段 | 类型 | 说明 |
|---|---|---|
| id | uint 主键 | 内部用，不对外 |
| uuid | string(36) uniqueIndex | 对外标识，由客户端新建时生成（crypto.randomUUID） |
| user_uuid | uuid, index | 属主，强制隔离 |
| name | string(32) not null | 1–16 个字符（trim 后） |
| color | string(16) not null | 固定 token 枚举：lime/sky/rose/amber/violet/slate |
| sort_order | int not null | 分组在侧栏中的顺序，0..n-1 |
| created_at / updated_at | time | 标准时间戳 |

约束：同一用户显示名唯一（service 校验，超长/空白拒绝），每用户最多 20 个分组。

### `user_domain_layout_items`
| 字段 | 类型 | 说明 |
|---|---|---|
| id | uint 主键 | 内部用 |
| user_uuid | uuid, index (user_uuid, domain_uuid) uniqueIndex | 属主 |
| domain_uuid | string(32) | 域 UUID |
| group_uuid | string(36) nullable | 空表示“未分组”；必须属于同一用户 |
| sort_order | int not null | 所属容器内顺序，0..n-1 |

说明：该表是**个人 UI 状态**，与 `domain_members`（成员关系）解耦。读取时用成员关系补齐缺失域（自动出现在“未分组”末尾），保存时剔除已退出的域，避免脏数据堆积。组删除时其下 items 置为未分组。

## 后端分层与接口

- `model/user_domain_layout.go`：两个 GORM 实体 + TableName。
- `repository/user_domain_layout_repo.go`：按 user_uuid 读取 groups/items、整份替换（事务）、清理孤儿行。
- `service/domain_layout_service.go`：领域规则（返回 `*pkg.AppError`）
  - 分组名 trim、长度 1–16、同用户内唯一、数量上限 20；
  - color 必须是白名单枚举；
  - item 的 domain_uuid 必须属于该用户已加入的域；
  - item 的 group_uuid 必须为空或属于该用户；
  - 每个容器（每个组 + 未分组）内 sort_order 归一化为 0..n-1；
  - 整份保存单事务完成：删除旧 items → 写入新 items；groups 按 uuid upsert，删除本次未出现的组（其成员转未分组）。
- `handler/domain_layout_handler.go`：`/domain/layout/get` 与 `/domain/layout/save`，`middleware.CurrentUserUUID(c)` 取用户，空则 `INVALID_PARAMS`；不提供任何指定他人的参数。成功 `pkg.Success`，失败 `pkg.HandleError`。
- `routes/domain/routes.go`：在 protected 组新增两条 POST；`server/gin.go`：装配 repo → service → handler。
- Swagger：若项目使用注解则补充，否则同步 `app/server/docs/swagger.yaml` 条目。

### 响应形状

`GET /domain/layout/get` → `{ groups: DomainGroup[], items: DomainLayoutItem[] }`
- groups 按 sort_order 升序；items 含该用户所有已加入域（后端补齐）。

`POST /domain/layout/save` body 同上 → 返回规范化后的完整布局。

## 前端

- `api/domainLayout.ts`：types + `getDomainLayout()` / `saveDomainLayout(layout)`。
- `stores/domainLayoutStore.ts`：zustand `xxxStore` 门面 + `useXxxStore` hook 双入口。state：groups、items、loaded、saving、error。actions：`load()`、`createGroup/renameGroup/recolorGroup/deleteGroup`、`moveDomain(domainUUID, groupUUID|null, index)`、`reorderGroup(from,to)`、`reorderDomain(groupUUID, from, to)`。每次变更先更新本地再 400ms 防抖 `save()`；失败 toast 并保留本地改动。
- `utils/domainLayout.ts`：纯函数排序/移动辅助（便于单测）。
- 侧栏改造：顶层序列 = 各文件夹 + 未分组的域图标（沿用 `DomainIcon`，有头像显示头像）。文件夹图标为手机文件夹式**四宫格**：固定 2×2 四格，前 4 个域的微缩头像/首字填格，空位留占位；右上角数量角标；空文件夹回退为文件夹图标。双击域沿用 `requiresDoubleClick` 进入域。
- 交互分工：
  - **左键**文件夹 = 查看内容（Radix Popover，可排序域列表 + “拖到这里移出分组”投放区）。
  - **右键**文件夹 = 编辑面板（富面板而非文字菜单）：名称输入框、颜色圆点、拖放移出区、上移/下移、删除分组；面板锚定在鼠标右下角（`side="right" align="start"` + 偏移）。
  - **移动端**文件夹长按 300ms 打开同一编辑面板；域图标长按拖拽即可排序/入组/移出。
  - 域（频道）**不提供删除**，仅“移到未分组 / 移到某分组 / 上移 / 下移”，同样右键在鼠标位置弹出。
- DnD：新增 `@dnd-kit/core` + `@dnd-kit/sortable`。侧栏顶层可拖文件夹与未分组域排序，把域拖到文件夹图标即入组；面板内可拖域排序、拖到移出区即出组。不做侧栏↔面板跨容器拖拽（面板为瞬时 UI）。
- 视觉：沿用现有 token 类名与 lucide 图标；文件夹颜色仅用枚举 token；组件下沉 `components/domain/`，不在页面堆逻辑。
- 错误：布局加载失败回退为全部未分组且不阻塞侧栏；保存失败 toast 提示可重试；`domainStore.myDomainUUIDs` 变化时重新 load 以补齐新加入/退出的域。

## 测试

- Go 单测（同包 `*_test.go`）：service 的校验（空名/重名/超量/非法颜色/非本人组/未加入域）、排序归一化、组删除转未分组、补齐与清理、跨用户隔离；handler 用户隔离。
- Web 单测：`utils/domainLayout` 纯函数、store 的 move/reorder/deleteGroup、API mock。
- `test/` 集成：GET（空）→ SAVE → GET 往返；用户 A 保存后用户 B 看不到。
- 验证：`pnpm typecheck`、`pnpm lint`、`pnpm test`、`pnpm test:server:go`、`pnpm build`。

## 范围外

- 不共享/公开分组；分组完全个人化。
- 不改成员关系、权限、域本身字段。
- 不做文件夹嵌套（手机文件夹式单层）。
- 不做跨设备实时同步推送（刷新/重登后拉取即可）。
