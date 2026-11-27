package repository

import (
	"testing"

	"GOSpeak/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// seedLegacyConversationSchema 建出「last_message_id 还在」的旧库结构：
// conversation_participants 带该列与一条存量会话行。
func seedLegacyConversationSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE conversation_participants (
			conversation_id text primary key, identity_a text not null, identity_b text not null,
			last_message_id text, last_content text, last_sender_identity text, last_message_at datetime,
			unread_count_a integer default 0, unread_count_b integer default 0,
			created_at datetime, updated_at datetime)`,
		`CREATE INDEX idx_conv_part_a ON conversation_participants(identity_a)`,
		`CREATE INDEX idx_conv_part_b ON conversation_participants(identity_b)`,
		`INSERT INTO conversation_participants (conversation_id,identity_a,identity_b,last_message_id,last_content,last_sender_identity,unread_count_a,unread_count_b)
			VALUES ('conv-1','alice','bob','22222222-2222-2222-2222-222222222222','hello','alice',1,0)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed legacy schema (%s): %v", stmt, err)
		}
	}
}

// TestDropLegacyConversationColumns_RemovesLastMessageID 覆盖真实升级路径：
// 老库（列在）→ 删列 → 其余列与数据保留 → 二次执行幂等。
func TestDropLegacyConversationColumns_RemovesLastMessageID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:conv_legacy_drop_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	seedLegacyConversationSchema(t, db)

	if !db.Migrator().HasColumn("conversation_participants", "last_message_id") {
		t.Fatal("seed must create conversation_participants.last_message_id")
	}
	if err := dropLegacyConversationColumns(db); err != nil {
		t.Fatalf("dropLegacyConversationColumns: %v", err)
	}
	if db.Migrator().HasColumn("conversation_participants", "last_message_id") {
		t.Fatal("conversation_participants.last_message_id must be dropped")
	}

	for _, col := range []string{"conversation_id", "identity_a", "identity_b", "last_content", "last_sender_identity", "last_message_at", "unread_count_a", "unread_count_b"} {
		if !db.Migrator().HasColumn("conversation_participants", col) {
			t.Fatalf("conversation_participants.%s must survive", col)
		}
	}

	var content, sender string
	var unreadA int
	if err := db.Raw(`SELECT last_content, last_sender_identity, unread_count_a FROM conversation_participants WHERE conversation_id = 'conv-1'`).
		Row().Scan(&content, &sender, &unreadA); err != nil {
		t.Fatalf("read surviving conversation row: %v", err)
	}
	if content != "hello" || sender != "alice" || unreadA != 1 {
		t.Fatalf("conversation row corrupted: content=%q sender=%q unread_a=%d", content, sender, unreadA)
	}

	if err := dropLegacyConversationColumns(db); err != nil {
		t.Fatalf("second dropLegacyConversationColumns must be a no-op: %v", err)
	}
}

// TestDropLegacyConversationColumns_OnFreshDatabase 新库（无旧列）必须静默通过。
func TestDropLegacyConversationColumns_OnFreshDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:conv_legacy_drop_fresh?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ConversationParticipant{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if db.Migrator().HasColumn("conversation_participants", "last_message_id") {
		t.Fatal("AutoMigrate must not create conversation_participants.last_message_id")
	}
	if err := dropLegacyConversationColumns(db); err != nil {
		t.Fatalf("dropLegacyConversationColumns on fresh db: %v", err)
	}
}

// TestDropLegacyConversationColumns_NoTablesAtAll 空库（连表都没有）也必须静默通过。
func TestDropLegacyConversationColumns_NoTablesAtAll(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:conv_legacy_drop_empty?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := dropLegacyConversationColumns(db); err != nil {
		t.Fatalf("dropLegacyConversationColumns on empty db: %v", err)
	}
}
