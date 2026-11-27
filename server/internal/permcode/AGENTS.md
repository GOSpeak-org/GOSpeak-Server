# permcode 模块

权限码常量。被路由（`middleware.RequirePermission`）、模型种子（`model.DefaultRolePermissions`）与文档共用。

## 文件

| 文件 | 范围 |
|------|------|
| `permcode.go` | 平台级权限：`room:*` `user:*` `role:*` `mute:manage` `sfu:manage` `email_config:*` `message:*` `storage:*` `oauth:*` `audit:read` `announcement:*` `signal:kick` |
| `domain_permcode.go` | 域内权限：`domain:create/read/manage/delete/invite/invite:manage/member:review/kick/role:manage/room-layout:manage` |

## 命名

`资源:动作`（二级用 `:子动作`），全小写，冒号分隔。域内权限统一以 `domain:` 前缀。

## 使用方式

- 路由：`middleware.RequirePermission(permcode.PermX)`。
- 种子：`model/permission.go` 的 `DefaultPermissions` / `DefaultRolePermissions` 必须同步登记，否则权限无法授予。
- 展示：`docs/permissions.md` 与前端管理页需要能解释新权限。

## 约束

- **不要在路由或 service 里写权限码字面量**，一律引用本包常量。
- 新增权限 = 常量 + 种子 + 文档三处同步；只加常量不加种子会导致「永远没人有该权限」。
- `audit:read` 不进默认种子，需手工授予（设计如此）。
