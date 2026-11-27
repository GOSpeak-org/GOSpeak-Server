package model

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestUserDomainLayout_TableNames(t *testing.T) {
	if got := (UserDomainGroup{}).TableName(); got != "user_domain_groups" {
		t.Fatalf("UserDomainGroup.TableName() = %q, want user_domain_groups", got)
	}
	if got := (UserDomainLayoutItem{}).TableName(); got != "user_domain_layout_items" {
		t.Fatalf("UserDomainLayoutItem.TableName() = %q, want user_domain_layout_items", got)
	}
}

func TestUserDomainLayout_FieldTags(t *testing.T) {
	group := reflect.TypeOf(UserDomainGroup{})
	for _, name := range []string{"ID", "UUID", "UserUUID", "Name", "Color", "SortOrder", "CreatedAt", "UpdatedAt"} {
		field, ok := group.FieldByName(name)
		if !ok {
			t.Fatalf("UserDomainGroup missing field %s", name)
		}
		if tag := field.Tag.Get("json"); tag == "" {
			t.Fatalf("UserDomainGroup.%s missing json tag", name)
		}
	}
	item := reflect.TypeOf(UserDomainLayoutItem{})
	for _, name := range []string{"ID", "UserUUID", "DomainUUID", "GroupUUID", "SortOrder"} {
		if _, ok := item.FieldByName(name); !ok {
			t.Fatalf("UserDomainLayoutItem missing field %s", name)
		}
	}
	groupUUID, _ := item.FieldByName("GroupUUID")
	if groupUUID.Type != reflect.TypeOf(sql.NullString{}) {
		t.Fatalf("GroupUUID type = %v, want sql.NullString", groupUUID.Type)
	}
	domainUUID, _ := item.FieldByName("DomainUUID")
	if !strings.Contains(domainUUID.Tag.Get("gorm"), "uniqueIndex:idx_user_domain_layout_item") {
		t.Fatalf("DomainUUID should join the (user_uuid, domain_uuid) unique index")
	}
	userUUID, _ := item.FieldByName("UserUUID")
	if !strings.Contains(userUUID.Tag.Get("gorm"), "uniqueIndex:idx_user_domain_layout_item") {
		t.Fatalf("UserUUID should join the (user_uuid, domain_uuid) unique index")
	}
}
