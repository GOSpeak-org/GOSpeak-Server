package service

import (
	"fmt"
	"strings"
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPushTokenService(t *testing.T) (*PushTokenService, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:push_token_svc_%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.PushToken{}); err != nil {
		t.Fatal(err)
	}
	return NewPushTokenService(repository.NewPushTokenRepository(db)), db
}

func TestPushTokenService_RegisterPersistsWithDefaultPlatform(t *testing.T) {
	svc, db := newPushTokenService(t)
	if err := svc.Register("user-1", "token-1", ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	var row model.PushToken
	if err := db.Where("token = ?", "token-1").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.UserUUID != "user-1" || row.Platform != defaultPushPlatform {
		t.Fatalf("got user=%q platform=%q", row.UserUUID, row.Platform)
	}
}

func TestPushTokenService_RegisterReassignsExistingToken(t *testing.T) {
	svc, db := newPushTokenService(t)
	if err := svc.Register("user-1", "token-1", "android"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Register("user-2", "token-1", "ios"); err != nil {
		t.Fatal(err)
	}
	var row model.PushToken
	if err := db.Where("token = ?", "token-1").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.UserUUID != "user-2" || row.Platform != "ios" {
		t.Fatalf("got user=%q platform=%q", row.UserUUID, row.Platform)
	}
	var count int64
	if err := db.Model(&model.PushToken{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}
}

func TestPushTokenService_RegisterValidatesInput(t *testing.T) {
	svc, _ := newPushTokenService(t)
	cases := []struct {
		name     string
		userUUID string
		token    string
		platform string
		want     pkg.ErrCode
	}{
		{name: "missing user", token: "tok", want: pkg.TOKEN_NOT_EXIST},
		{name: "empty token", userUUID: "u-1", want: pkg.INVALID_PARAMS},
		{name: "oversized token", userUUID: "u-1", token: strings.Repeat("x", maxPushTokenLen+1), want: pkg.INVALID_PARAMS},
		{name: "oversized platform", userUUID: "u-1", token: "tok", platform: strings.Repeat("p", maxPushTokenPlatform+1), want: pkg.INVALID_PARAMS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Register(tc.userUUID, tc.token, tc.platform)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Code != tc.want {
				t.Fatalf("code = %d, want %d", err.Code, tc.want)
			}
		})
	}
}
