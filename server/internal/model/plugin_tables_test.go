package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newPluginTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func TestPluginTables(t *testing.T) {
	tables := []struct {
		model any
		name  string
		cols  []string
	}{
		{&PluginRegistration{}, "plugin_registrations", []string{"plugin_id", "source", "version", "enabled", "allowed_route_prefix", "created_at", "updated_at"}},
		{&PluginConfig{}, "plugin_configs", []string{"plugin_id", "config_json", "updated_at"}},
		{&PluginKV{}, "plugin_kvs", []string{"plugin_id", "key", "value_json"}},
		{&PluginRequestLog{}, "plugin_request_logs", []string{"plugin_id", "request_id", "created_at"}},
	}
	db := newPluginTestDB(t)
	if err := db.AutoMigrate(&PluginRegistration{}, &PluginConfig{}, &PluginKV{}, &PluginRequestLog{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	for _, tt := range tables {
		if !db.Migrator().HasTable(tt.name) {
			t.Fatalf("table %s missing", tt.name)
		}
		for _, col := range tt.cols {
			if !db.Migrator().HasColumn(tt.name, col) {
				t.Fatalf("table %s missing column %s", tt.name, col)
			}
		}
	}
}

func TestNewPluginRegistrationDefaultsEnabled(t *testing.T) {
	reg := NewPluginRegistration("demo", "builtin", "1.0.0")
	if !reg.Enabled {
		t.Fatal("NewPluginRegistration Enabled = false, want true")
	}
}

func TestPluginSystemUserUUID(t *testing.T) {
	parsed, err := uuid.Parse(PluginSystemUserUUID)
	if err != nil {
		t.Fatalf("PluginSystemUserUUID %q is not a UUID: %v", PluginSystemUserUUID, err)
	}
	if parsed.Version() != 5 {
		t.Fatalf("PluginSystemUserUUID version = %d, want 5", parsed.Version())
	}
}

func TestAuditLogActorKindColumn(t *testing.T) {
	db := newPluginTestDB(t)
	if err := db.AutoMigrate(&AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	if !db.Migrator().HasColumn("audit_logs", "actor_kind") {
		t.Fatal("audit_logs.actor_kind column missing")
	}
	if !db.Migrator().HasIndex("audit_logs", "idx_audit_logs_actor_kind") {
		t.Fatal("audit_logs.actor_kind index missing")
	}
}
func TestAuditLogEmptyActorKindDefaultsToUserAfterFind(t *testing.T) {
	db := newPluginTestDB(t)
	if err := db.AutoMigrate(&AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	if err := db.Exec(`INSERT INTO audit_logs (actor_kind, actor_uuid, action) VALUES ('', 'u1', 'mute_user')`).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	var rec AuditLog
	if err := db.First(&rec).Error; err != nil {
		t.Fatalf("read legacy row: %v", err)
	}
	if rec.ActorKind != "user" {
		t.Fatalf("legacy actor_kind = %q, want user", rec.ActorKind)
	}
}
