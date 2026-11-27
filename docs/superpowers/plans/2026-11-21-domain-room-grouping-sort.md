# 域内房间分组与排序 实现计划

> 【状态】2026-11-27 核验：该计划主体已交付（后端布局/分组与前端均已落地），仅供历史参考。

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让平台管理员、域 owner/admin 及被授权的自定义域角色为域内房间配置可折叠分组与两级排序；管理页与侧边栏两处均可维护，结果通过 WS 向全域成员生效。

**架构：** 规范化三处存储（分组表 / 布局表 / room 扩展列）+ 全量布局事务保存；WS `RoomInfo` 增加分组字段、`RoomListResultPayload.Layout` 携带分组名与区块顺序，一次推送即可渲染；前端管理页用 dnd-kit 编辑，侧边栏复用同一 save 接口做快捷拖拽。

**技术栈：** Go 1.27 / Gin / GORM / SQLite · React 19 / TypeScript / zustand / react-query / @dnd-kit / TanStack Virtual · Biome / Vitest

---

## 文件结构

**后端新建**
- `app/server/internal/model/domain_room_group.go` — 分组实体与 UUID 生成
- `app/server/internal/model/domain_room_layout.go` — 域级布局实体（未分组区块位置）
- `app/server/internal/repository/room_layout_repo.go` — 布局查询与全量事务保存
- `app/server/internal/service/room_layout_service.go` — 布局 DTO / 校验 / 业务规则
- `app/server/internal/handler/room_layout_handler.go` — HTTP 入口与权限判定
- 同名 `*_test.go` 与 `test/domain/room_layout_test.go`

**后端修改**
- `model/room.go`（加 `GroupUUID`/`SortOrder`）、`model/permission.go`、`model/domain_role.go`、`permcode/domain_permcode.go`、`repository/db_migrations.go`
- `protocol/payloads.go`、`protocol/events.go`、`signal/hub.go`、`signal/hub_room_list.go`
- `router/routes/domain/routes.go`、`server/gin.go`、`docs/swagger.*`

**前端新建**
- `app/web/src/components/manage/DomainRoomLayoutPanel.tsx` 与 `.test.tsx`
- `app/web/src/components/room/components/roomGroupHeader.tsx`、`roomLayoutEditBar.tsx`、`roomListLayout.test.ts`

**前端修改**
- `api/domain.ts`、`components/room/roomListUtils.ts`、`components/room/roomList.tsx`、`pages/(app)/manage/domains/$domainUUID/index.tsx`、`protocol/generated/*`、`docs/permissions.md`

> 工作区已有未提交改动（`roomItem.tsx`、域管理页、`DomainRolePanel.tsx` 等），所有修改必须用 edit 追加，禁止整体覆盖写。

---

### 任务 1：权限码、实体与迁移注册

**文件：**
- 创建：`app/server/internal/model/room_layout_seed_test.go`
- 创建：`app/server/internal/model/domain_room_group.go`、`app/server/internal/model/domain_room_layout.go`
- 修改：`app/server/internal/model/room.go`、`model/permission.go`、`model/domain_role.go`、`permcode/domain_permcode.go`、`repository/db_migrations.go`

`- [ ]` **步骤 1：编写失败的种子测试**

新建 `app/server/internal/model/room_layout_seed_test.go`：

`go`
package model

import (
	"GOSpeak/internal/permcode"
	"testing"
)

func roomLayoutHas(codes []string, want string) bool {
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	return false
}

func TestRoomLayoutPermissionSeeds(t *testing.T) {
	want := permcode.PermDomainRoomLayoutManage
	if want != "domain:room-layout:manage" {
		t.Fatalf("unexpected permcode: %s", want)
	}
	defs := make([]string, 0, len(DefaultPermissions))
	for _, p := range DefaultPermissions {
		defs = append(defs, p.Code)
	}
	if !roomLayoutHas(defs, want) {
		t.Fatal("DefaultPermissions missing room-layout code")
	}
	if !roomLayoutHas(DefaultRolePermissions["admin"], want) {
		t.Fatal("global admin missing room-layout code")
	}
	if !roomLayoutHas(AssignableDomainPermissions, want) {
		t.Fatal("assignable domain permissions missing room-layout code")
	}
	if !roomLayoutHas(DefaultDomainRolePermissions[DomainRoleAdmin], want) {
		t.Fatal("domain admin missing room-layout code")
	}
}
`

`- [ ]` **步骤 2：运行测试确认失败**

运行：`go test ./internal/model -run TestRoomLayoutPermissionSeeds`
预期：编译失败，`undefined: permcode.PermDomainRoomLayoutManage`。

`- [ ]` **步骤 3：实现常量、种子、实体与迁移注册**

`permcode/domain_permcode.go` 常量块末尾追加：

`go`
	PermDomainRoomLayoutManage = "domain:room-layout:manage"
`

`model/permission.go`：常量块追加 `PermDomainRoomLayoutManage = permcode.PermDomainRoomLayoutManage`；`DefaultPermissions` 追加：

`go`
	{Code: PermDomainRoomLayoutManage, Name: "管理房间分组与排序", Description: "配置域内房间的分组与排序"},
`

`DefaultRolePermissions["admin"]` 追加 `PermDomainRoomLayoutManage`；`model/domain_role.go` 的 `AssignableDomainPermissions` 与 `DefaultDomainRolePermissions[DomainRoleAdmin]` 各追加 `PermDomainRoomLayoutManage`。

`model/room.go` 的 `Room` 在 `DomainUUID` 后追加：

`go`
	GroupUUID string `gorm:"size:36;index" json:"group_uuid"`
	SortOrder int    `gorm:"not null;default:0" json:"sort_order"`
`

新建 `model/domain_room_group.go`：

`go`
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DomainRoomGroup struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UUID       string    `gorm:"type:uuid;uniqueIndex" json:"uuid"`
	DomainUUID string    `gorm:"size:32;not null;uniqueIndex:idx_domain_room_group_name,priority:1" json:"domain_uuid"`
	Name       string    `gorm:"size:64;not null;uniqueIndex:idx_domain_room_group_name,priority:2" json:"name"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (g *DomainRoomGroup) BeforeCreate(_ *gorm.DB) error {
	if g.UUID == "" {
		g.UUID = uuid.New().String()
	}
	return nil
}

func (DomainRoomGroup) TableName() string { return "domain_room_groups" }
`

新建 `model/domain_room_layout.go`：

`go`
package model

import "time"

type DomainRoomLayout struct {
	DomainUUID        string    `gorm:"size:32;primaryKey" json:"domain_uuid"`
	UngroupedPosition int       `gorm:"not null;default:0" json:"ungrouped_position"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (DomainRoomLayout) TableName() string { return "domain_room_layouts" }
`

`repository/db_migrations.go` 的 `AutoMigrate` 列表在 `&model.Room{}` 后追加 `&model.DomainRoomGroup{}` 与 `&model.DomainRoomLayout{}`。

`- [ ]` **步骤 4：运行测试确认通过**

运行：`go test ./internal/model -run TestRoomLayoutPermissionSeeds` 与 `go test ./internal/repository -run TestRoomRepository`
预期：均 PASS。

`- [ ]` **步骤 5：Commit**

`bash`
git add app/server/internal/model app/server/internal/permcode app/server/internal/repository/db_migrations.go
git commit -m "feat(server): 房间分组实体与权限种子"
`

### 任务 2：布局 Repository（全量事务保存）

**文件：**
- 创建：`app/server/internal/repository/room_layout_repo_test.go`
- 创建：`app/server/internal/repository/room_layout_repo.go`

`- [ ]` **步骤 1：编写失败的仓储测试**

`go`
package repository

import (
	"GOSpeak/internal/model"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openRoomLayoutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRoomLayoutRepository_SaveLayout(t *testing.T) {
	db := openRoomLayoutTestDB(t)
	if err := db.Create([]model.Room{
		{Name: "a", DomainUUID: "domain-a", UUID: "room-a"},
		{Name: "b", DomainUUID: "domain-a", UUID: "room-b"},
		{Name: "c", DomainUUID: "domain-b", UUID: "room-c"},
	}).Error; err != nil {
		t.Fatalf("seed rooms: %v", err)
	}

	repo := NewRoomLayoutRepository(db)
	err := repo.SaveLayout("domain-a", []GroupInput{
		{UUID: "", Name: "常用", SortOrder: 0, RoomUUIDs: []string{"room-a"}},
	}, []string{"room-b"}, 1)
	if err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}

	groups, err := repo.ListGroups("domain-a")
	if err != nil || len(groups) != 1 || groups[0].Name != "常用" {
		t.Fatalf("groups: %+v err=%v", groups, err)
	}
	var gotA, gotB, gotC model.Room
	db.First(&gotA, "uuid = ?", "room-a")
	db.First(&gotB, "uuid = ?", "room-b")
	db.First(&gotC, "uuid = ?", "room-c")
	if gotA.GroupUUID != groups[0].UUID || gotA.SortOrder != 0 {
		t.Fatalf("room-a not assigned: %+v", gotA)
	}
	if gotB.GroupUUID != "" || gotB.SortOrder != 1 {
		t.Fatalf("room-b not ungrouped first: %+v", gotB)
	}
	if gotC.GroupUUID != "" || gotC.SortOrder != 0 {
		t.Fatalf("domain-b room touched: %+v", gotC)
	}
	layout, err := repo.GetLayout("domain-a")
	if err != nil || layout.UngroupedPosition != 1 {
		t.Fatalf("layout: %+v err=%v", layout, err)
	}
}
`

`- [ ]` **步骤 2：运行测试确认失败**

运行：`go test ./internal/repository -run TestRoomLayoutRepository_SaveLayout`
预期：编译失败，`undefined: NewRoomLayoutRepository`。

`- [ ]` **步骤 3：实现 Repository**

新建 `app/server/internal/repository/room_layout_repo.go`：

`go`
package repository

import (
	"GOSpeak/internal/model"

	"gorm.io/gorm"
)

type GroupInput struct {
	UUID      string
	Name      string
	SortOrder int
	RoomUUIDs []string
}

type RoomLayoutRepository struct {
	db *gorm.DB
}

func NewRoomLayoutRepository(db *gorm.DB) *RoomLayoutRepository {
	return &RoomLayoutRepository{db: db}
}

func (r *RoomLayoutRepository) ListGroups(domainUUID string) ([]model.DomainRoomGroup, error) {
	var groups []model.DomainRoomGroup
	err := r.db.Where("domain_uuid = ?", domainUUID).Order("sort_order ASC, id ASC").Find(&groups).Error
	return groups, err
}

func (r *RoomLayoutRepository) GetLayout(domainUUID string) (model.DomainRoomLayout, error) {
	var layout model.DomainRoomLayout
	err := r.db.Where("domain_uuid = ?", domainUUID).First(&layout).Error
	if err == gorm.ErrRecordNotFound {
		groups, _ := r.ListGroups(domainUUID)
		return model.DomainRoomLayout{DomainUUID: domainUUID, UngroupedPosition: len(groups)}, nil
	}
	return layout, err
}

func (r *RoomLayoutRepository) DomainRooms(domainUUID string) ([]model.Room, error) {
	var rooms []model.Room
	err := r.db.Where("domain_uuid = ?", domainUUID).Order("id ASC").Find(&rooms).Error
	return rooms, err
}

func (r *RoomLayoutRepository) SaveLayout(domainUUID string, groups []GroupInput, ungroupedUUIDs []string, ungroupedPosition int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing []model.DomainRoomGroup
		if err := tx.Where("domain_uuid = ?", domainUUID).Find(&existing).Error; err != nil {
			return err
		}
		keep := make(map[string]bool, len(groups))
		for _, g := range groups {
			if g.UUID != "" {
				keep[g.UUID] = true
			}
		}
		for _, old := range existing {
			if keep[old.UUID] {
				continue
			}
			if err := tx.Model(&model.Room{}).
				Where("domain_uuid = ? AND group_uuid = ?", domainUUID, old.UUID).
				Updates(map[string]interface{}{"group_uuid": "", "sort_order": 0}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}

		byUUID := make(map[string]model.DomainRoomGroup, len(existing))
		for _, g := range existing {
			byUUID[g.UUID] = g
		}
		for _, g := range groups {
			row := byUUID[g.UUID]
			row.DomainUUID = domainUUID
			row.Name = g.Name
			row.SortOrder = g.SortOrder
			if row.UUID == "" {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			} else if err := tx.Save(&row).Error; err != nil {
				return err
			}
			for i, roomUUID := range g.RoomUUIDs {
				if err := tx.Model(&model.Room{}).
					Where("domain_uuid = ? AND uuid = ?", domainUUID, roomUUID).
					Updates(map[string]interface{}{"group_uuid": row.UUID, "sort_order": i}).Error; err != nil {
					return err
				}
			}
		}
		for i, roomUUID := range ungroupedUUIDs {
			if err := tx.Model(&model.Room{}).
				Where("domain_uuid = ? AND uuid = ?", domainUUID, roomUUID).
				Updates(map[string]interface{}{"group_uuid": "", "sort_order": i}).Error; err != nil {
				return err
			}
		}

		layout := model.DomainRoomLayout{DomainUUID: domainUUID, UngroupedPosition: ungroupedPosition}
		return tx.Save(&layout).Error
	})
}
`

`- [ ]` **步骤 4：运行测试确认通过**

运行：`go test ./internal/repository -run TestRoomLayoutRepository_SaveLayout` 及 `go test ./internal/repository`
预期：PASS。

`- [ ]` **步骤 5：Commit**

`bash`
git add app/server/internal/repository/room_layout_repo.go app/server/internal/repository/room_layout_repo_test.go
git commit -m "feat(server): 房间布局仓储与全量事务保存"
`

---

### 任务 3：RoomLayoutService（DTO、校验、组装）

**文件：**
- 创建：`app/server/internal/service/room_layout_service_test.go`
- 创建：`app/server/internal/service/room_layout_service.go`

`- [ ]` **步骤 1：编写失败的服务测试**

`go`
package service

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/repository"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRoomLayoutService_GetAndSave(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{}); err != nil {
		t.Fatal(err)
	}
	db.Create([]model.Room{
		{Name: "a", DomainUUID: "domain-a", UUID: "room-a"},
		{Name: "b", DomainUUID: "domain-a", UUID: "room-b"},
	})
	svc := NewRoomLayoutService(repository.NewRoomLayoutRepository(db))

	view, appErr := svc.GetLayout("domain-a", true)
	if appErr != nil || view.UngroupedPosition != 0 || len(view.Ungrouped) != 2 {
		t.Fatalf("initial view: %+v err=%v", view, appErr)
	}
	if _, appErr = svc.SaveLayout("domain-a", []repository.GroupInput{
		{Name: "语音", RoomUUIDs: []string{"room-a"}},
	}, []string{"room-b"}, 1); appErr != nil {
		t.Fatalf("SaveLayout: %v", appErr)
	}
	view, appErr = svc.GetLayout("domain-a", false)
	if appErr != nil {
		t.Fatal(appErr)
	}
	if len(view.Groups) != 1 || view.Groups[0].Name != "语音" || len(view.Groups[0].Rooms) != 1 {
		t.Fatalf("groups: %+v", view.Groups)
	}
	if len(view.Ungrouped) != 1 || view.Ungrouped[0].UUID != "room-b" {
		t.Fatalf("ungrouped: %+v", view.Ungrouped)
	}
	if view.UngroupedPosition != 1 || view.CanManage {
		t.Fatalf("meta: pos=%d canManage=%v", view.UngroupedPosition, view.CanManage)
	}
}

func TestRoomLayoutService_RejectsForeignRoom(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{})
	db.Create(&model.Room{Name: "x", DomainUUID: "domain-b", UUID: "room-x"})
	svc := NewRoomLayoutService(repository.NewRoomLayoutRepository(db))
	if _, appErr := svc.SaveLayout("domain-a", nil, []string{"room-x"}, 0); appErr == nil {
		t.Fatal("expected error for foreign room")
	}
}
`

`- [ ]` **步骤 2：运行测试确认失败**

运行：`go test ./internal/service -run TestRoomLayoutService`
预期：编译失败，`undefined: NewRoomLayoutService`。

`- [ ]` **步骤 3：实现 Service**

新建 `app/server/internal/service/room_layout_service.go`：

`go`
package service

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
	"strings"
)

type RoomBrief struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Count       int    `json:"count"`
	HasPassword bool   `json:"has_password"`
	SortOrder   int    `json:"sort_order"`
}

type GroupLayout struct {
	UUID      string      `json:"uuid"`
	Name      string      `json:"name"`
	SortOrder int         `json:"sort_order"`
	Rooms     []RoomBrief `json:"rooms"`
}

type LayoutView struct {
	Groups            []GroupLayout `json:"groups"`
	Ungrouped         []RoomBrief   `json:"ungrouped"`
	UngroupedPosition int           `json:"ungrouped_position"`
	CanManage         bool          `json:"can_manage"`
}

type RoomLayoutService struct {
	repo *repository.RoomLayoutRepository
}

func NewRoomLayoutService(repo *repository.RoomLayoutRepository) *RoomLayoutService {
	return &RoomLayoutService{repo: repo}
}

func toBrief(r model.Room, order int) RoomBrief {
	return RoomBrief{
		UUID: r.UUID, Name: r.Name, Type: r.Type,
		HasPassword: r.Password != "", SortOrder: order,
	}
}

func (s *RoomLayoutService) GetLayout(domainUUID string, canManage bool) (*LayoutView, *pkg.AppError) {
	groups, err := s.repo.ListGroups(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	rooms, err := s.repo.DomainRooms(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	layout, err := s.repo.GetLayout(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	byGroup := make(map[string][]RoomBrief)
	var ungrouped []RoomBrief
	for _, r := range rooms {
		if r.GroupUUID == "" {
			ungrouped = append(ungrouped, toBrief(r, r.SortOrder))
		} else {
			byGroup[r.GroupUUID] = append(byGroup[r.GroupUUID], toBrief(r, r.SortOrder))
		}
	}
	view := &LayoutView{
		Groups: make([]GroupLayout, 0, len(groups)),
		Ungrouped: ungrouped, UngroupedPosition: layout.UngroupedPosition,
		CanManage: canManage,
	}
	for _, g := range groups {
		items := byGroup[g.UUID]
		if items == nil {
			items = []RoomBrief{}
		}
		view.Groups = append(view.Groups, GroupLayout{
			UUID: g.UUID, Name: g.Name, SortOrder: g.SortOrder, Rooms: items,
		})
	}
	return view, nil
}

func (s *RoomLayoutService) SaveLayout(domainUUID string, groups []repository.GroupInput, ungroupedUUIDs []string, ungroupedPosition int) (*LayoutView, *pkg.AppError) {
	rooms, err := s.repo.DomainRooms(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	valid := make(map[string]bool, len(rooms))
	for _, r := range rooms {
		valid[r.UUID] = true
	}
	seen := make(map[string]bool)
	names := make(map[string]bool)
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" || names[name] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "分组名不能为空且不能重复")
		}
		names[name] = true
		for _, uuid := range g.RoomUUIDs {
			if !valid[uuid] || seen[uuid] {
				return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "房间不存在或重复分配")
			}
			seen[uuid] = true
		}
	}
	for _, uuid := range ungroupedUUIDs {
		if !valid[uuid] || seen[uuid] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "房间不存在或重复分配")
		}
		seen[uuid] = true
	}
	if ungroupedPosition < 0 {
		ungroupedPosition = 0
	}
	if ungroupedPosition > len(groups) {
		ungroupedPosition = len(groups)
	}
	if err := s.repo.SaveLayout(domainUUID, groups, ungroupedUUIDs, ungroupedPosition); err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	return s.GetLayout(domainUUID, true)
}
`

`- [ ]` **步骤 4：运行测试确认通过**

运行：`go test ./internal/service -run TestRoomLayoutService`
预期：PASS（`pkg.INTERNAL_ERROR` = 5001、`pkg.FORBIDDEN` = 1013，已与 `internal/pkg/errors.go` 核对）。

`- [ ]` **步骤 5：Commit**

`bash`
git add app/server/internal/service/room_layout_service.go app/server/internal/service/room_layout_service_test.go
git commit -m "feat(server): 房间布局服务与校验"
`

---

### 任务 4：HTTP Handler、路由与依赖装配

**文件：**
- 创建：`app/server/internal/handler/room_layout_handler_test.go`
- 创建：`app/server/internal/handler/room_layout_handler.go`
- 修改：`app/server/internal/router/routes/domain/routes.go`、`app/server/server/gin.go`

`- [ ]` **步骤 1：编写失败的 Handler 测试**

`go`
package handler

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/repository"
	"GOSpeak/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRoomLayoutHandler_Get(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{})
	db.Create(&model.Room{Name: "a", DomainUUID: "domain-a", UUID: "room-a"})
	h := NewRoomLayoutHandler(service.NewRoomLayoutService(repository.NewRoomLayoutRepository(db)), nil, nil)

	r := gin.New()
	r.POST("/get", h.Get)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/get", strings.NewReader(`{"domain_uuid":"domain-a"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "room-a") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
`

`- [ ]` **步骤 2：运行测试确认失败**

运行：`go test ./internal/handler -run TestRoomLayoutHandler_Get`
预期：编译失败，`undefined: NewRoomLayoutHandler`。

`- [ ]` **步骤 3：实现 Handler**

新建 `app/server/internal/handler/room_layout_handler.go`：

`go`
package handler

import (
	"GOSpeak/internal/middleware"
	"GOSpeak/internal/permcode"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
)

type RoomLayoutHandler struct {
	svc       *service.RoomLayoutService
	permSvc   *service.PermissionService
	domainSvc *service.DomainService
}

func NewRoomLayoutHandler(svc *service.RoomLayoutService, permSvc *service.PermissionService, domainSvc *service.DomainService) *RoomLayoutHandler {
	return &RoomLayoutHandler{svc: svc, permSvc: permSvc, domainSvc: domainSvc}
}

type roomLayoutGetReq struct {
	DomainUUID string `json:"domain_uuid" binding:"required"`
}

type roomLayoutSaveReq struct {
	DomainUUID         string                  `json:"domain_uuid" binding:"required"`
	Groups             []repository.GroupInput `json:"groups"`
	UngroupedPosition  int                     `json:"ungrouped_position"`
	UngroupedRoomUUIDs []string                `json:"ungrouped_room_uuids"`
}

func (h *RoomLayoutHandler) canManage(c *gin.Context, domainUUID string) bool {
	if h.permSvc != nil && h.permSvc.HasPermission(middleware.CurrentRole(c), permcode.PermDomainRoomLayoutManage) {
		return true
	}
	if h.domainSvc == nil {
		return false
	}
	return h.domainSvc.HasDomainPermission(domainUUID, middleware.CurrentUserUUID(c), permcode.PermDomainRoomLayoutManage)
}

func (h *RoomLayoutHandler) Get(c *gin.Context) {
	var req roomLayoutGetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	view, appErr := h.svc.GetLayout(req.DomainUUID, h.canManage(c, req.DomainUUID))
	if appErr != nil {
		pkg.HandleError(c, appErr)
		return
	}
	pkg.Success(c, view)
}

func (h *RoomLayoutHandler) Save(c *gin.Context) {
	var req roomLayoutSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	if !h.canManage(c, req.DomainUUID) {
		pkg.Fail(c, pkg.FORBIDDEN, "无配置房间分组的权限")
		return
	}
	view, appErr := h.svc.SaveLayout(req.DomainUUID, req.Groups, req.UngroupedRoomUUIDs, req.UngroupedPosition)
	if appErr != nil {
		pkg.HandleError(c, appErr)
		return
	}
	pkg.Success(c, view)
}
`

`- [ ]` **步骤 4：注册路由与装配依赖**

`app/server/internal/router/routes/domain/routes.go` 的 `Register` 内追加：

`go`
	r.POST("/room-layout/get", middleware.RequireDomainMember(), h.RoomLayout.Get)
	r.POST("/room-layout/save", middleware.RequireDomainMember(), h.RoomLayout.Save)
`

在 `app/server/server/gin.go` 按既有注入风格构造 `RoomLayoutRepository → RoomLayoutService → RoomLayoutHandler`，并传入域路由装配处（与 `DomainHandler` 一致）。

`- [ ]` **步骤 5：运行测试确认通过、Commit**

运行：`go test ./internal/handler -run TestRoomLayoutHandler_Get` 与 `go test ./internal/router/...`
预期：PASS。

`bash`
git add app/server/internal/handler/room_layout_handler.go app/server/internal/handler/room_layout_handler_test.go app/server/internal/router/routes/domain/routes.go app/server/server/gin.go
git commit -m "feat(server): 房间布局 HTTP 接口与路由"
`

---

### 任务 5：WS 协议与信令排序

**文件：**
- 修改：`app/server/internal/protocol/payloads.go`、`protocol/events.go`、`signal/hub.go`、`signal/hub_room_list.go`
- 新增：`app/server/internal/signal/hub_room_list_layout_test.go`

`- [ ]` **步骤 1：编写失败的排序测试**

新建 `hub_room_list_layout_test.go`：构造三间 DB 房间（组 A 两间、未分组一间），给 Hub 注入 fake layout store（返回分组顺序与 `ungrouped_position=1`），调用 `getMergedRoomsScoped` 后断言结果顺序为「组 A 房间 → 未分组房间」，且 `result[0].GroupUUID == groupUUID`、`result[0].SortOrder == 0`。

`- [ ]` **步骤 2：运行测试确认失败**

运行：`go test ./internal/signal -run TestHubRoomListLayoutOrder`
预期：编译失败或顺序断言失败。

`- [ ]` **步骤 3：扩展协议 payload**

`protocol/payloads.go` 的 `RoomInfo` 追加 `GroupUUID string `json:"group_uuid,omitempty"`` 与 `SortOrder int `json:"sort_order"``；`RoomListResultPayload` 追加 `Layout RoomListLayoutPayload `json:"layout"``，并新增：

`go`
type RoomListLayoutPayload struct {
	Groups            []RoomListGroupPayload `json:"groups"`
	UngroupedPosition int                    `json:"ungrouped_position"`
}

type RoomListGroupPayload struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}
`

`protocol/events.go` 的 `payloadStructs` 登记 `RoomListLayoutPayload` 与 `RoomListGroupPayload`；事件名与 Catalog 不变（复用 `EventRoomListResult`）。

`- [ ]` **步骤 4：实现 Hub 注入与排序**

`signal/hub.go` 新增只读接口并挂到 `Hub` / `HubOptions`：

`go`
type roomLayoutStore interface {
	Groups(domainUUID string) ([]model.DomainRoomGroup, error)
	Layout(domainUUID string) (model.DomainRoomLayout, error)
}
`

`Hub` 增加 `layoutStore roomLayoutStore`，`HubOptions` 增加 `LayoutStore roomLayoutStore` 并在构造处赋值。

`hub_room_list.go`：DB 房间构造 `RoomInfo` 时补 `GroupUUID: r.GroupUUID, SortOrder: r.SortOrder`；末尾排序替换为：

`go`
	groups, _ := h.layoutStore.Groups(domainUUID)
	layout, _ := h.layoutStore.Layout(domainUUID)
	groupOrder := make(map[string]int, len(groups))
	for i, g := range groups {
		groupOrder[g.UUID] = i
	}
	blockOf := func(r RoomInfo) int {
		if r.GroupUUID == "" {
			return layout.UngroupedPosition
		}
		if i, ok := groupOrder[r.GroupUUID]; ok {
			return i
		}
		return len(groups)
	}
	sort.SliceStable(result, func(i, j int) bool {
		bi, bj := blockOf(result[i]), blockOf(result[j])
		if bi != bj {
			return bi < bj
		}
		if result[i].SortOrder != result[j].SortOrder {
			return result[i].SortOrder < result[j].SortOrder
		}
		if result[i].CreatedAt != result[j].CreatedAt {
			return result[i].CreatedAt < result[j].CreatedAt
		}
		return result[i].ID < result[j].ID
	})
`

`broadcastRoomList` 在 payload 里补 `Layout`：由 `layoutStore` 读分组与位置，组装 `protocol.RoomListLayoutPayload`（无 store 时返回空分组、位置 0）。

`- [ ]` **步骤 5：生成协议、验证与 Commit**

运行：`pnpm gen:protocol`、`go test ./internal/signal ./internal/protocol`
预期：generated 三个产物更新，`TestGeneratedArtifactsFresh` PASS，测试全绿。

`bash`
git add app/server/internal/protocol app/server/internal/signal app/web/src/protocol/generated
git commit -m "feat: WS 房间列表携带分组布局并据此排序"
`

---

### 任务 6：管理页分组与排序面板

**文件：**
- 修改：`app/web/src/api/domain.ts`
- 新建：`app/web/src/components/manage/DomainRoomLayoutPanel.tsx`、`DomainRoomLayoutPanel.test.tsx`
- 修改：`app/web/src/pages/(app)/manage/domains/$domainUUID/index.tsx`

`- [ ]` **步骤 1：扩展 API 类型与函数**

`app/web/src/api/domain.ts` 追加：

`ts`
export interface RoomLayoutBrief {
	uuid: string;
	name: string;
	type?: string;
	count: number;
	has_password: boolean;
	sort_order: number;
}
export interface RoomLayoutGroup {
	uuid: string;
	name: string;
	sort_order: number;
	rooms: RoomLayoutBrief[];
}
export interface RoomLayoutView {
	groups: RoomLayoutGroup[];
	ungrouped: RoomLayoutBrief[];
	ungrouped_position: number;
	can_manage: boolean;
}
export interface RoomLayoutSaveReq {
	domain_uuid: string;
	groups: Array<{ uuid?: string; name: string; sort_order: number; room_uuids: string[] }>;
	ungrouped_position: number;
	ungrouped_room_uuids: string[];
}
export function getRoomLayout(domainUUID: string): Promise<RoomLayoutView> {
	return apiClient.post({ url: "/api/v1/domain/room-layout/get", data: { domain_uuid: domainUUID } });
}
export function saveRoomLayout(req: RoomLayoutSaveReq): Promise<RoomLayoutView> {
	return apiClient.post({ url: "/api/v1/domain/room-layout/save", data: req });
}
`

`- [ ]` **步骤 2：编写失败的面板测试**

`DomainRoomLayoutPanel.test.tsx` 覆盖：① `can_manage=false` 不渲染拖拽手柄与「保存布局」；② 点击新增分组按钮增加一行；③ 点保存时调用 `saveRoomLayout` 且携带正确的 `ungrouped_position` 与房间 uuid 顺序。

`- [ ]` **步骤 3：实现面板**

`DomainRoomLayoutPanel.tsx`：用 `useState<RoomLayoutView>` 持有本地布局；用 `DndContext` + `SortableContext`（`@dnd-kit/core` / `@dnd-kit/sortable`）实现分组标题排序与房间跨组/组内移动；未分组区块作为第 `ungrouped_position` 个块参与排序；分组改名用内联 `Input` + Enter；「保存布局」把本地状态转成 `RoomLayoutSaveReq` 调 `saveRoomLayout`，成功 `toast.success("布局已保存")`，失败回滚并 `toastActionError`。只读时只渲染分组标题与房间名。

`- [ ]` **步骤 4：接入管理页**

在域详情页「房间管理」`ManageSection` 之后新增 `ManageSection`（标题「分组与排序」），用 react-query 的 `useQuery({ queryKey: ["room-layout", uuid], queryFn: () => getRoomLayout(uuid) })` 取数并渲染面板。保留该文件已有未提交改动。

`- [ ]` **步骤 5：运行测试确认通过、Commit**

运行：`pnpm --filter @gospeak/web test -- DomainRoomLayoutPanel` 与 `pnpm typecheck`
预期：PASS。

`bash`
git add app/web/src/api/domain.ts app/web/src/components/manage/DomainRoomLayoutPanel.tsx app/web/src/components/manage/DomainRoomLayoutPanel.test.tsx "app/web/src/pages/(app)/manage/domains/$domainUUID/index.tsx"
git commit -m "feat(web): 域管理页房间分组与排序面板"
`

---

### 任务 7：侧边栏分组渲染与快捷拖拽

**文件：**
- 新建：`app/web/src/components/room/components/roomGroupHeader.tsx`、`roomLayoutEditBar.tsx`、`roomListLayout.test.ts`
- 修改：`app/web/src/components/room/roomListUtils.ts`、`roomList.tsx`

`- [ ]` **步骤 1：编写失败的分块纯函数测试**

`roomListLayout.test.ts` 断言 `groupRoomsForSidebar(rooms, layout)` 返回拍平后的行序列：分组标题行与房间行交替，未分组块位于 `ungrouped_position` 指定位置，组内按 `sort_order` 排序。

`- [ ]` **步骤 2：运行测试确认失败**

运行：`pnpm --filter @gospeak/web test -- roomListLayout`
预期：FAIL，`groupRoomsForSidebar is not a function`。

`- [ ]` **步骤 3：实现分块函数与标题组件**

`roomListUtils.ts` 新增：

`ts`
export type SidebarRow =
	| { kind: "group"; uuid: string; name: string }
	| { kind: "room"; room: RoomInfo };

export function groupRoomsForSidebar(rooms: RoomInfo[], layout: RoomListLayoutPayload): SidebarRow[] {
	const groups = [...layout.groups].sort((a, b) => a.sort_order - b.sort_order);
	const byGroup = new Map<string, RoomInfo[]>();
	let ungrouped: RoomInfo[] = [];
	for (const room of rooms) {
		if (room.group_uuid) {
			byGroup.set(room.group_uuid, [...(byGroup.get(room.group_uuid) ?? []), room]);
		} else {
			ungrouped = [...ungrouped, room];
		}
	}
	const sortRooms = (list: RoomInfo[]) =>
		[...list].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0));
	const blocks = groups.map((g) => ({
		kind: "group" as const, uuid: g.uuid, name: g.name,
		rooms: sortRooms(byGroup.get(g.uuid) ?? []),
	}));
	blocks.splice(Math.min(layout.ungrouped_position, blocks.length), 0, {
		kind: "unGrouped" as const, rooms: sortRooms(ungrouped),
	});
	const rows: SidebarRow[] = [];
	for (const block of blocks) {
		if (block.kind === "group") {
			rows.push({ kind: "group", uuid: block.uuid, name: block.name });
		}
		for (const room of block.rooms) rows.push({ kind: "room", room });
	}
	return rows;
}
`

`roomGroupHeader.tsx` 渲染可折叠标题（chevron + 名称 + 房间数），折叠状态用组件 `useState`（默认展开）。

`- [ ]` **步骤 4：接入 RoomList 与虚拟列表**

`roomList.tsx`：从 `room:list:result` 的 payload 取 `layout`（若 `socketStore` 未保存则新增 `roomLayout` state），把 `visibleRooms` 传给 `groupRoomsForSidebar` 得到 `rows`；`useVirtualizer` 的 `count` 改为 `rows.length`，渲染按 `row.kind` 分派到 `RoomGroupHeader` 或 `RoomItem`。`RoomListHeader` 内按 `hasDomainPermission(currentDomainUUID, "domain:room-layout:manage")` 显示 `RoomLayoutEditBar`；开启编辑后用 `DndContext` 包裹列表，拖拽结束组装 `RoomLayoutSaveReq` 调 `saveRoomLayout`。

`- [ ]` **步骤 5：运行测试确认通过、Commit**

运行：`pnpm --filter @gospeak/web test -- roomListLayout` 与 `pnpm typecheck`
预期：PASS。

`bash`
git add app/web/src/components/room app/web/src/api/domain.ts
git commit -m "feat(web): 侧边栏按分组渲染并支持快捷拖拽"
`

---

### 任务 8：文档、Swagger 与全量验证

**文件：**
- 修改：`docs/permissions.md`
- 生成：`app/server/docs/swagger.yaml`、`swagger.json`、`docs.go`

`- [ ]` **步骤 1：补充权限文档**

在 `docs/permissions.md` 域内权限表新增一行：`domain:room-layout:manage` / 管理房间分组与排序 / 默认授予平台 admin、域 owner/admin，可分配给自定义域角色。

`- [ ]` **步骤 2：重新生成 Swagger**

按 `app/server/AGENTS.md` 的既有命令重新生成 `app/server/docs/` 产物，确认新增两个接口出现在 `domain` 标签下。

`- [ ]` **步骤 3：全量门禁验证**

依次运行并记录结果：

`bash`
pnpm gen:protocol
go test ./...
pnpm typecheck
pnpm lint
pnpm test
pnpm build
`

预期全部 PASS；若 `scripts/check-hook-deps.mjs` 报新增告警，按门禁要求用 `biome-ignore` 并写明理由消除。

`- [ ]` **步骤 4：Commit**

`bash`
git add docs/permissions.md app/server/docs
git commit -m "docs: 房间分组权限与接口文档"
`

---

## 自检

- [x] 规格覆盖度：数据模型（任务 1）、权限（任务 1/4/8）、API（任务 2/3/4）、WS 协议与排序（任务 5）、管理页（任务 6）、侧边栏（任务 7）、测试（各任务 + 任务 8）、工作区注意（文件结构段落）。
- [x] 占位符扫描：无「待定 / TODO / 后续实现」；所有步骤均含实际代码或明确命令。
- [x] 类型一致性：`GroupInput`（repository）→ `LayoutView/GroupLayout/RoomBrief`（service）→ `RoomLayoutView`（前端 API）→ `SidebarRow`（前端分块）逐层字段对齐；`GroupUUID/SortOrder` 在 model、protocol、前端类型中均使用同名 snake_case JSON 字段。


