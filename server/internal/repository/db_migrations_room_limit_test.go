package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"GOSpeak/internal/model"
)

func TestMigrateVoiceRoomLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Room{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rooms := []model.Room{
		{Name: "voice-zero", Type: model.RoomTypeVoice, Limit: 0, DomainUUID: "domain-a"},
		{Name: "voice-one", Type: model.RoomTypeVoice, Limit: 1, DomainUUID: "domain-a"},
		{Name: "voice-ok", Type: model.RoomTypeVoice, Limit: 12, DomainUUID: "domain-a"},
		{Name: "text-unlimited", Type: model.RoomTypeText, Limit: 0, DomainUUID: "domain-a"},
	}
	if err := db.Create(&rooms).Error; err != nil {
		t.Fatalf("seed rooms: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := migrateVoiceRoomLimit(db); err != nil {
			t.Fatalf("migrateVoiceRoomLimit run %d: %v", i+1, err)
		}
	}

	want := map[string]uint{
		"voice-zero":     model.MinVoiceRoomLimit,
		"voice-one":      model.MinVoiceRoomLimit,
		"voice-ok":       12,
		"text-unlimited": 0,
	}
	for name, expected := range want {
		var got model.Room
		if err := db.Where("name = ?", name).First(&got).Error; err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		if got.Limit != expected {
			t.Fatalf("%s limit = %d, want %d", name, got.Limit, expected)
		}
	}
}
