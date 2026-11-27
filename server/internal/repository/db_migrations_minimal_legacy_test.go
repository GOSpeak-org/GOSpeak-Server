package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestDropLegacyGuestSchema_MinimalLegacyTable 覆盖「只有旧列、没有索引」的最小老库：
// 与 db_migrations_legacy_test.go 的完整老库场景互补，确认幂等删列删表在最小结构上同样生效。
func TestDropLegacyGuestSchema_MinimalLegacyTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 手工建一张带旧列的表（模拟生产老库）
	if err := db.Exec(`CREATE TABLE domains (
		id integer primary key, uuid text, name text, icon_url text, description text,
		owner_uuid text, invite_code text, is_public numeric,
		allow_guest numeric, guest_can_listen numeric, guest_can_speak numeric,
		guest_can_message numeric, guest_limit integer, created_at datetime, updated_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE users (id integer primary key, uuid text, name text, is_guest numeric)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE domain_guest_bans (id integer primary key, domain_uuid text, user_uuid text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := dropLegacyGuestSchema(db); err != nil {
		t.Fatalf("dropLegacyGuestSchema: %v", err)
	}
	for _, col := range []string{"allow_guest", "guest_can_listen", "guest_can_speak", "guest_can_message", "guest_limit", "invite_code"} {
		if db.Migrator().HasColumn("domains", col) {
			t.Fatalf("domains.%s must be dropped", col)
		}
	}
	if db.Migrator().HasColumn("users", "is_guest") {
		t.Fatal("users.is_guest must be dropped")
	}
	if db.Migrator().HasTable("domain_guest_bans") {
		t.Fatal("domain_guest_bans must be dropped")
	}
}
