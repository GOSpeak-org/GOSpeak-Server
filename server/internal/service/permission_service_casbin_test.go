package service

import (
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPermissionService_CasbinAuthoritativeAndRemoteReload(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Permission{}, &model.RolePermission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	perm := model.Permission{Code: "room:read", Name: "read"}
	if err := db.Create(&perm).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := db.Create(&model.RolePermission{RoleName: "tester", PermissionID: perm.ID}).Error; err != nil {
		t.Fatalf("create role permission: %v", err)
	}

	svc, err := NewPermissionService(repository.NewPermissionRepository(db), repository.NewCasbinAdapter(db))
	if err != nil {
		t.Fatalf("new permission service: %v", err)
	}
	if !svc.HasPermission("tester", "room:read") {
		t.Fatal("tester must have room:read from casbin policy")
	}

	if err := db.Where("role_name = ?", "tester").Delete(&model.RolePermission{}).Error; err != nil {
		t.Fatalf("revoke permission: %v", err)
	}
	svc.OnRemoteInvalidate("tester")
	if svc.HasPermission("tester", "room:read") {
		t.Fatal("revoked permission must not pass after remote invalidate")
	}
}
