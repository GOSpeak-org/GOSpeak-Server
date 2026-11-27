package repository

import (
	"fmt"
	"strings"
	"testing"

	"GOSpeak/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupPushTokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:push_token_%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.PushToken{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPushTokenRepo_UpsertCreatesAndUpdatesOwner(t *testing.T) {
	db := setupPushTokenDB(t)
	repo := NewPushTokenRepository(db)

	if err := repo.Upsert("user-1", "token-1", "android"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByToken("token-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.UserUUID != "user-1" || got.Platform != "android" {
		t.Fatalf("got user=%q platform=%q", got.UserUUID, got.Platform)
	}

	if err := repo.Upsert("user-2", "token-1", "ios"); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetByToken("token-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.UserUUID != "user-2" || got.Platform != "ios" {
		t.Fatalf("upsert did not refresh owner/platform: user=%q platform=%q", got.UserUUID, got.Platform)
	}
	var count int64
	if err := db.Model(&model.PushToken{}).Where("token = ?", "token-1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("token row count = %d, want 1", count)
	}
}

func TestPushTokenRepo_UpsertKeepsDistinctTokens(t *testing.T) {
	db := setupPushTokenDB(t)
	repo := NewPushTokenRepository(db)

	for _, token := range []string{"token-a", "token-b"} {
		if err := repo.Upsert("user-1", token, "web"); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&model.PushToken{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("row count = %d, want 2", count)
	}
}
