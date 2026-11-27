package service

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newDomainLayoutSvc(t *testing.T) (*DomainLayoutService, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:layout_svc_%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatalf("db pool: %v", err)
	}
	pool.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.UserDomainGroup{}, &model.UserDomainLayoutItem{}, &model.DomainMember{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewDomainLayoutService(repository.NewUserDomainLayoutRepository(db)), db
}

func addJoinedDomains(t *testing.T, db *gorm.DB, userUUID string, domains ...string) {
	t.Helper()
	for _, domainUUID := range domains {
		if err := db.Create(&model.DomainMember{DomainUUID: domainUUID, UserUUID: userUUID}).Error; err != nil {
			t.Fatalf("create member: %v", err)
		}
	}
}

func assertLayoutItem(t *testing.T, layout *Layout, domainUUID, groupUUID string, sortOrder int) {
	t.Helper()
	for _, item := range layout.Items {
		if item.DomainUUID == domainUUID {
			if item.GroupUUID != groupUUID || item.SortOrder != sortOrder {
				t.Fatalf("item %s = (group=%q, sort=%d), want (group=%q, sort=%d)", domainUUID, item.GroupUUID, item.SortOrder, groupUUID, sortOrder)
			}
			return
		}
	}
	t.Fatalf("item %s missing from layout: %+v", domainUUID, layout.Items)
}

func TestDomainLayoutService_LayoutItemJSON(t *testing.T) {
	raw, err := json.Marshal(LayoutItem{DomainUUID: "d1"})
	if err != nil {
		t.Fatalf("marshal ungrouped item: %v", err)
	}
	if got, want := string(raw), `{"domain_uuid":"d1","group_uuid":null,"sort_order":0}`; got != want {
		t.Fatalf("ungrouped JSON = %s, want %s", got, want)
	}
	raw, err = json.Marshal(LayoutItem{DomainUUID: "d1", GroupUUID: "g1", SortOrder: 2})
	if err != nil {
		t.Fatalf("marshal grouped item: %v", err)
	}
	if got, want := string(raw), `{"domain_uuid":"d1","group_uuid":"g1","sort_order":2}`; got != want {
		t.Fatalf("grouped JSON = %s, want %s", got, want)
	}
}

func TestDomainLayoutService_GetLayoutNormalizes(t *testing.T) {
	svc, db := newDomainLayoutSvc(t)
	addJoinedDomains(t, db, "user-a", "d1", "d2", "d3")
	repo := repository.NewUserDomainLayoutRepository(db)
	err := repo.Replace("user-a",
		[]model.UserDomainGroup{{UUID: "g1", Name: "Group", Color: "lime", SortOrder: 7}},
		[]model.UserDomainLayoutItem{
			{DomainUUID: "d1", GroupUUID: sql.NullString{String: "g1", Valid: true}, SortOrder: 9},
			{DomainUUID: "dX", SortOrder: 0},
			{DomainUUID: "d2", GroupUUID: sql.NullString{String: "gGone", Valid: true}, SortOrder: 0},
		})
	if err != nil {
		t.Fatalf("seed layout: %v", err)
	}

	layout, appErr := svc.GetLayout("user-a")
	if appErr != nil {
		t.Fatalf("GetLayout: %v", appErr)
	}
	if len(layout.Groups) != 1 || layout.Groups[0].UUID != "g1" || layout.Groups[0].SortOrder != 0 {
		t.Fatalf("unexpected groups: %+v", layout.Groups)
	}
	if len(layout.Items) != 3 {
		t.Fatalf("expected 3 joined items, got %+v", layout.Items)
	}
	for _, item := range layout.Items {
		if item.DomainUUID == "dX" {
			t.Fatal("left domain dX should be dropped")
		}
	}
	assertLayoutItem(t, layout, "d1", "g1", 0)
	assertLayoutItem(t, layout, "d2", "", 0)
	assertLayoutItem(t, layout, "d3", "", 1)
}

func TestDomainLayoutService_SaveLayoutNormalizes(t *testing.T) {
	svc, db := newDomainLayoutSvc(t)
	addJoinedDomains(t, db, "user-a", "d1", "d2", "d3", "d4")

	layout, appErr := svc.SaveLayout("user-a", &Layout{
		Groups: []LayoutGroup{
			{UUID: "g2", Name: "Second", Color: "sky", SortOrder: 5},
			{UUID: "g1", Name: " First ", Color: "lime", SortOrder: 2},
		},
		Items: []LayoutItem{
			{DomainUUID: "d2", GroupUUID: "g2", SortOrder: 9},
			{DomainUUID: "d1", GroupUUID: "g1", SortOrder: 7},
			{DomainUUID: "d3", SortOrder: 3},
			{DomainUUID: "d4", GroupUUID: "g1", SortOrder: 1},
		},
	})
	if appErr != nil {
		t.Fatalf("SaveLayout: %v", appErr)
	}
	if len(layout.Groups) != 2 || layout.Groups[0].UUID != "g1" || layout.Groups[1].UUID != "g2" {
		t.Fatalf("groups not sorted: %+v", layout.Groups)
	}
	if layout.Groups[0].SortOrder != 0 || layout.Groups[1].SortOrder != 1 || layout.Groups[0].Name != "First" {
		t.Fatalf("groups not normalized: %+v", layout.Groups)
	}
	assertLayoutItem(t, layout, "d4", "g1", 0)
	assertLayoutItem(t, layout, "d1", "g1", 1)
	assertLayoutItem(t, layout, "d2", "g2", 0)
	assertLayoutItem(t, layout, "d3", "", 0)

	reloaded, appErr := svc.GetLayout("user-a")
	if appErr != nil {
		t.Fatalf("GetLayout: %v", appErr)
	}
	if len(reloaded.Items) != len(layout.Items) {
		t.Fatalf("round trip changed item count: %+v vs %+v", reloaded, layout)
	}
}

func TestDomainLayoutService_DeleteGroupMovesItemsToUngrouped(t *testing.T) {
	svc, db := newDomainLayoutSvc(t)
	addJoinedDomains(t, db, "user-a", "d1", "d2")
	if _, appErr := svc.SaveLayout("user-a", &Layout{
		Groups: []LayoutGroup{{UUID: "g1", Name: "One", Color: "lime"}, {UUID: "g2", Name: "Two", Color: "sky"}},
		Items:  []LayoutItem{{DomainUUID: "d1", GroupUUID: "g1"}, {DomainUUID: "d2", GroupUUID: "g2"}},
	}); appErr != nil {
		t.Fatalf("initial save: %v", appErr)
	}

	layout, appErr := svc.SaveLayout("user-a", &Layout{
		Groups: []LayoutGroup{{UUID: "g1", Name: "One", Color: "lime"}},
		Items: []LayoutItem{
			{DomainUUID: "d1", GroupUUID: "g1"},
			{DomainUUID: "d2", GroupUUID: "g2"},
		},
	})
	if appErr != nil {
		t.Fatalf("save after group deletion: %v", appErr)
	}
	if len(layout.Groups) != 1 {
		t.Fatalf("group g2 should be gone: %+v", layout.Groups)
	}
	assertLayoutItem(t, layout, "d2", "", 0)
}

func TestDomainLayoutService_SaveLayoutValidation(t *testing.T) {
	svc, db := newDomainLayoutSvc(t)
	addJoinedDomains(t, db, "user-a", "d1")
	addJoinedDomains(t, db, "user-b", "d2")
	if _, appErr := svc.SaveLayout("user-b", &Layout{
		Groups: []LayoutGroup{{UUID: "g-b", Name: "B Group", Color: "sky"}},
		Items:  []LayoutItem{{DomainUUID: "d2", GroupUUID: "g-b"}},
	}); appErr != nil {
		t.Fatalf("seed user-b layout: %v", appErr)
	}

	tooMany := make([]LayoutGroup, 0, maxLayoutGroups+1)
	for i := 0; i <= maxLayoutGroups; i++ {
		tooMany = append(tooMany, LayoutGroup{UUID: fmt.Sprintf("g%d", i), Name: fmt.Sprintf("G%d", i), Color: "lime"})
	}

	cases := []struct {
		name   string
		layout *Layout
	}{
		{"nil layout", nil},
		{"empty name", &Layout{Groups: []LayoutGroup{{UUID: "g1", Name: "", Color: "lime"}}}},
		{"blank name", &Layout{Groups: []LayoutGroup{{UUID: "g1", Name: "   ", Color: "lime"}}}},
		{"name too long", &Layout{Groups: []LayoutGroup{{UUID: "g1", Name: strings.Repeat("x", 17), Color: "lime"}}}},
		{"invalid color", &Layout{Groups: []LayoutGroup{{UUID: "g1", Name: "Group", Color: "pink"}}}},
		{"duplicate name", &Layout{Groups: []LayoutGroup{{UUID: "g1", Name: "Same", Color: "lime"}, {UUID: "g2", Name: " Same ", Color: "sky"}}}},
		{"duplicate uuid", &Layout{Groups: []LayoutGroup{{UUID: "g1", Name: "One", Color: "lime"}, {UUID: "g1", Name: "Two", Color: "sky"}}}},
		{"empty uuid", &Layout{Groups: []LayoutGroup{{Name: "One", Color: "lime"}}}},
		{"too many groups", &Layout{Groups: tooMany}},
		{"domain not joined", &Layout{Items: []LayoutItem{{DomainUUID: "d-unknown"}}}},
		{"other user group", &Layout{Items: []LayoutItem{{DomainUUID: "d1", GroupUUID: "g-b"}}}},
		{"duplicate domain", &Layout{Items: []LayoutItem{{DomainUUID: "d1"}, {DomainUUID: "d1"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, appErr := svc.SaveLayout("user-a", tc.layout)
			if appErr == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !pkg.IsCode(appErr, pkg.INVALID_PARAMS) {
				t.Fatalf("expected INVALID_PARAMS, got %v", appErr)
			}
		})
	}
}

func TestDomainLayoutService_UserIsolation(t *testing.T) {
	svc, db := newDomainLayoutSvc(t)
	addJoinedDomains(t, db, "user-a", "d1")
	addJoinedDomains(t, db, "user-b", "d2")

	if _, appErr := svc.SaveLayout("user-a", &Layout{
		Groups: []LayoutGroup{{UUID: "g-a", Name: "A", Color: "lime"}},
		Items:  []LayoutItem{{DomainUUID: "d1", GroupUUID: "g-a"}},
	}); appErr != nil {
		t.Fatalf("save user-a: %v", appErr)
	}
	if _, appErr := svc.SaveLayout("user-b", &Layout{
		Items: []LayoutItem{{DomainUUID: "d2"}},
	}); appErr != nil {
		t.Fatalf("save user-b: %v", appErr)
	}

	layoutA, appErr := svc.GetLayout("user-a")
	if appErr != nil {
		t.Fatalf("get user-a: %v", appErr)
	}
	if len(layoutA.Groups) != 1 || layoutA.Groups[0].UUID != "g-a" || len(layoutA.Items) != 1 || layoutA.Items[0].DomainUUID != "d1" {
		t.Fatalf("user-a layout corrupted: %+v", layoutA)
	}
	layoutB, appErr := svc.GetLayout("user-b")
	if appErr != nil {
		t.Fatalf("get user-b: %v", appErr)
	}
	if len(layoutB.Groups) != 0 || len(layoutB.Items) != 1 || layoutB.Items[0].DomainUUID != "d2" {
		t.Fatalf("user-b layout corrupted: %+v", layoutB)
	}
	if _, appErr := svc.SaveLayout("user-b", &Layout{Items: []LayoutItem{{DomainUUID: "d2", GroupUUID: "g-a"}}}); !pkg.IsCode(appErr, pkg.INVALID_PARAMS) {
		t.Fatalf("user-b must not reference user-a's group, got %v", appErr)
	}
}

func TestDomainLayoutService_RejectsForeignGroupUUID(t *testing.T) {
	svc, db := newDomainLayoutSvc(t)
	addJoinedDomains(t, db, "user-a", "d1")
	repo := repository.NewUserDomainLayoutRepository(db)
	if err := repo.Replace("user-b",
		[]model.UserDomainGroup{{UUID: "g-foreign", Name: "Others", Color: "sky", SortOrder: 0}},
		nil); err != nil {
		t.Fatalf("seed foreign group: %v", err)
	}

	_, appErr := svc.SaveLayout("user-a", &Layout{
		Groups: []LayoutGroup{{UUID: "g-foreign", Name: "Hijack", Color: "lime", SortOrder: 0}},
		Items:  []LayoutItem{{DomainUUID: "d1"}},
	})
	if appErr == nil {
		t.Fatal("expected foreign group uuid to be rejected")
	}
	if appErr.Code != pkg.INVALID_PARAMS {
		t.Fatalf("code = %d, want %d", appErr.Code, pkg.INVALID_PARAMS)
	}

	groups, err := repo.ListGroups("user-b")
	if err != nil {
		t.Fatalf("list foreign groups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Others" || groups[0].Color != "sky" {
		t.Fatalf("foreign group mutated: %+v", groups)
	}
}
