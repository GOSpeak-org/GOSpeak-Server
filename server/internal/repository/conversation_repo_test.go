package repository

import (
	"fmt"
	"testing"
	"time"

	"GOSpeak/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestConversationRepo(t *testing.T) *ConversationRepository {
	t.Helper()
	dsn := fmt.Sprintf("file:conversation_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ConversationParticipant{}); err != nil {
		t.Fatal(err)
	}
	return NewConversationRepository(db)
}

func TestConversationListCursor(t *testing.T) {
	repo := newTestConversationRepo(t)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	seed := []*model.ConversationParticipant{
		{ConversationID: "conv-old", IdentityA: "alice", IdentityB: "bob", CreatedAt: base},
		{ConversationID: "conv-new", IdentityA: "alice", IdentityB: "carol", CreatedAt: base.Add(time.Minute)},
		{ConversationID: "conv-other", IdentityA: "bob", IdentityB: "carol", CreatedAt: base.Add(2 * time.Minute)},
	}
	for _, cp := range seed {
		if err := repo.Upsert(cp); err != nil {
			t.Fatalf("upsert %s: %v", cp.ConversationID, err)
		}
	}

	page, hasMore, err := repo.ListByIdentityCursor("alice", "", 1)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(page) != 1 || page[0].ConversationID != "conv-new" || !hasMore {
		t.Fatalf("first page = %v hasMore=%v, want [conv-new] with more", page, hasMore)
	}

	page, hasMore, err = repo.ListByIdentityCursor("alice", "conv-new", 1)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(page) != 1 || page[0].ConversationID != "conv-old" || hasMore {
		t.Fatalf("second page = %v hasMore=%v, want [conv-old] without more", page, hasMore)
	}

	page, hasMore, err = repo.ListByIdentityCursor("alice", "conv-old", 1)
	if err != nil {
		t.Fatalf("third page: %v", err)
	}
	if len(page) != 0 || hasMore {
		t.Fatalf("third page = %v hasMore=%v, want empty without more", page, hasMore)
	}
}

func TestConversationListCursorPagination(t *testing.T) {
	repo := newTestConversationRepo(t)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 3; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		cp := &model.ConversationParticipant{
			ConversationID: fmt.Sprintf("conv-%d", i),
			IdentityA:      "alice",
			IdentityB:      "bob",
			CreatedAt:      at,
		}
		if i > 0 {
			cp.LastMessageAt = &at
		}
		if err := repo.Upsert(cp); err != nil {
			t.Fatal(err)
		}
	}

	page, hasMore, err := repo.ListByIdentityCursor("alice", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || !hasMore {
		t.Fatalf("want 2 items with more, got %d hasMore=%v", len(page), hasMore)
	}
	if page[0].ConversationID != "conv-2" || page[1].ConversationID != "conv-1" {
		t.Fatalf("want newest-first [conv-2 conv-1], got %q %q", page[0].ConversationID, page[1].ConversationID)
	}

	next, hasMore2, err := repo.ListByIdentityCursor("alice", "conv-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || hasMore2 {
		t.Fatalf("want 1 item with no more, got %d hasMore=%v", len(next), hasMore2)
	}
	if next[0].ConversationID != "conv-0" {
		t.Fatalf("want conv-0, got %q", next[0].ConversationID)
	}

	empty, hasMore3, err := repo.ListByIdentityCursor("alice", "conv-0", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 || hasMore3 {
		t.Fatalf("want empty page, got %d hasMore=%v", len(empty), hasMore3)
	}
}

func TestMigrateConversationQueryIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:conversation_migrate_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateConversationQueryIndexes(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if !db.Migrator().HasTable("conversation_participants") {
		t.Fatal("conversation_participants table missing")
	}
	if !db.Migrator().HasIndex("conversation_participants", "idx_conv_part_identity") {
		t.Fatal("idx_conv_part_identity missing")
	}
	if err := migrateConversationQueryIndexes(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
