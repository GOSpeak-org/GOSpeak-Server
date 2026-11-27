package repository

import (
	"GOSpeak/internal/model"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newUserDomainLayoutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:layout_repo_%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
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
	return db
}

func layoutTestGroup(uuid, name, color string, order int) model.UserDomainGroup {
	return model.UserDomainGroup{UUID: uuid, Name: name, Color: color, SortOrder: order}
}

func layoutTestItem(domainUUID, groupUUID string, order int) model.UserDomainLayoutItem {
	group := sql.NullString{}
	if groupUUID != "" {
		group = sql.NullString{String: groupUUID, Valid: true}
	}
	return model.UserDomainLayoutItem{DomainUUID: domainUUID, GroupUUID: group, SortOrder: order}
}

func TestUserDomainLayoutRepo_ReplaceAndList(t *testing.T) {
	db := newUserDomainLayoutTestDB(t)
	repo := NewUserDomainLayoutRepository(db)

	groups := []model.UserDomainGroup{
		layoutTestGroup("g1", "Games", "lime", 0),
		layoutTestGroup("g2", "Work", "sky", 1),
	}
	items := []model.UserDomainLayoutItem{
		layoutTestItem("d1", "g1", 0),
		layoutTestItem("d2", "", 0),
	}
	if err := repo.Replace("user-a", groups, items); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	gotGroups, err := repo.ListGroups("user-a")
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(gotGroups) != 2 || gotGroups[0].UUID != "g1" || gotGroups[1].UUID != "g2" {
		t.Fatalf("unexpected groups: %+v", gotGroups)
	}
	gotItems, err := repo.ListItems("user-a")
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(gotItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(gotItems))
	}
	byDomain := make(map[string]model.UserDomainLayoutItem, len(gotItems))
	for _, item := range gotItems {
		byDomain[item.DomainUUID] = item
	}
	if group := byDomain["d1"].GroupUUID; !group.Valid || group.String != "g1" {
		t.Fatalf("expected item d1 grouped, got %+v", group)
	}
	if group := byDomain["d2"].GroupUUID; group.Valid {
		t.Fatalf("expected ungrouped item d2, got %+v", group)
	}

	var firstGroup model.UserDomainGroup
	if err := db.Where("uuid = ?", "g1").First(&firstGroup).Error; err != nil {
		t.Fatalf("load first group: %v", err)
	}

	updatedGroups := []model.UserDomainGroup{layoutTestGroup("g1", "Renamed", "rose", 0)}
	updatedItems := []model.UserDomainLayoutItem{layoutTestItem("d3", "", 0)}
	if err := repo.Replace("user-a", updatedGroups, updatedItems); err != nil {
		t.Fatalf("second Replace: %v", err)
	}

	gotGroups, _ = repo.ListGroups("user-a")
	if len(gotGroups) != 1 || gotGroups[0].Name != "Renamed" || gotGroups[0].Color != "rose" {
		t.Fatalf("upsert did not update group: %+v", gotGroups)
	}
	if gotGroups[0].ID != firstGroup.ID || !gotGroups[0].CreatedAt.Equal(firstGroup.CreatedAt) {
		t.Fatalf("upsert should preserve id and created_at: before=%+v after=%+v", firstGroup, gotGroups[0])
	}
	gotItems, _ = repo.ListItems("user-a")
	if len(gotItems) != 1 || gotItems[0].DomainUUID != "d3" {
		t.Fatalf("items were not replaced: %+v", gotItems)
	}
	var orphanCount int64
	db.Model(&model.UserDomainLayoutItem{}).Where("domain_uuid = ?", "d1").Count(&orphanCount)
	if orphanCount != 0 {
		t.Fatalf("stale item d1 still present")
	}
}

func TestUserDomainLayoutRepo_JoinedDomainUUIDs(t *testing.T) {
	db := newUserDomainLayoutTestDB(t)
	repo := NewUserDomainLayoutRepository(db)
	for _, m := range []model.DomainMember{
		{DomainUUID: "d1", UserUUID: "user-a"},
		{DomainUUID: "d2", UserUUID: "user-a"},
		{DomainUUID: "d3", UserUUID: "user-b"},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("create member: %v", err)
		}
	}
	got, err := repo.JoinedDomainUUIDs("user-a")
	if err != nil {
		t.Fatalf("JoinedDomainUUIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 joined domains, got %v", got)
	}
}

func TestUserDomainLayoutRepo_UserIsolation(t *testing.T) {
	db := newUserDomainLayoutTestDB(t)
	repo := NewUserDomainLayoutRepository(db)
	if err := repo.Replace("user-a", []model.UserDomainGroup{layoutTestGroup("g-a", "A", "lime", 0)}, []model.UserDomainLayoutItem{layoutTestItem("d-a", "g-a", 0)}); err != nil {
		t.Fatalf("Replace user-a: %v", err)
	}
	if err := repo.Replace("user-b", []model.UserDomainGroup{layoutTestGroup("g-b", "B", "sky", 0)}, []model.UserDomainLayoutItem{layoutTestItem("d-b", "", 0)}); err != nil {
		t.Fatalf("Replace user-b: %v", err)
	}

	groupsA, _ := repo.ListGroups("user-a")
	itemsA, _ := repo.ListItems("user-a")
	if len(groupsA) != 1 || groupsA[0].UUID != "g-a" || len(itemsA) != 1 || itemsA[0].DomainUUID != "d-a" {
		t.Fatalf("user-a layout corrupted: groups=%+v items=%+v", groupsA, itemsA)
	}
	groupsB, _ := repo.ListGroups("user-b")
	itemsB, _ := repo.ListItems("user-b")
	if len(groupsB) != 1 || groupsB[0].UUID != "g-b" || len(itemsB) != 1 || itemsB[0].DomainUUID != "d-b" {
		t.Fatalf("user-b layout corrupted: groups=%+v items=%+v", groupsB, itemsB)
	}
}
