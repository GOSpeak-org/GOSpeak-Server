package model

import (
	"GOSpeak/internal/permcode"
	"testing"
)

func roomLayoutHas(codes []string, want string) bool {
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	return false
}

func TestRoomLayoutPermissionSeeds(t *testing.T) {
	want := permcode.PermDomainRoomLayoutManage
	if want != "domain:room-layout:manage" {
		t.Fatalf("unexpected permcode: %s", want)
	}
	defs := make([]string, 0, len(DefaultPermissions))
	for _, p := range DefaultPermissions {
		defs = append(defs, p.Code)
	}
	if !roomLayoutHas(defs, want) {
		t.Fatal("DefaultPermissions missing room-layout code")
	}
	if !roomLayoutHas(DefaultRolePermissions["admin"], want) {
		t.Fatal("global admin missing room-layout code")
	}
	if !roomLayoutHas(AssignableDomainPermissions, want) {
		t.Fatal("assignable domain permissions missing room-layout code")
	}
	if !roomLayoutHas(DefaultDomainRolePermissions[DomainRoleAdmin], want) {
		t.Fatal("domain admin missing room-layout code")
	}
}
