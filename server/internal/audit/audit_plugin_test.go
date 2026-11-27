package audit

import (
	"testing"

	"GOSpeak/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPluginAuditWhitelist(t *testing.T) {
	if !IsValidAction("plugin_write") {
		t.Fatal("plugin_write must be a valid audit action")
	}
	if !IsValidTargetType("plugin") {
		t.Fatal("plugin must be a valid audit target type")
	}
}

func TestEntryDefaultActorKind(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	s := NewService(db)
	if s == nil {
		t.Fatal("nil audit service")
	}
	s.Log(Entry{
		ActorUUID:  "00000000-0000-0000-0000-000000000001",
		ActorName:  "alice",
		Action:     ActionMuteUser,
		TargetType: TargetUser,
		TargetID:   "u1",
		Success:    true,
	})
	s.Stop()

	var rec model.AuditLog
	if err := db.First(&rec).Error; err != nil {
		t.Fatalf("read audit record: %v", err)
	}
	if rec.ActorKind != "user" {
		t.Fatalf("default actor_kind = %q, want user", rec.ActorKind)
	}
}

func TestLogPluginActorKind(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	s := NewService(db)
	if s == nil {
		t.Fatal("nil audit service")
	}
	s.Log(Entry{
		ActorKind:  "plugin",
		ActorUUID:  model.PluginSystemUserUUID,
		ActorName:  "plugin:com.example.audit",
		Action:     "plugin_write",
		TargetType: "plugin",
		TargetID:   "com.example.audit",
		Success:    true,
	})
	s.Stop()

	var rec model.AuditLog
	if err := db.First(&rec).Error; err != nil {
		t.Fatalf("read audit record: %v", err)
	}
	if rec.ActorKind != "plugin" {
		t.Fatalf("actor_kind = %q, want plugin", rec.ActorKind)
	}
	if rec.ActorUUID != model.PluginSystemUserUUID {
		t.Fatalf("actor_uuid = %q, want %q", rec.ActorUUID, model.PluginSystemUserUUID)
	}
}
