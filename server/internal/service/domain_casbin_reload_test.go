package service

import (
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/permcode"
	"GOSpeak/internal/repository"

	casbinmodel "github.com/casbin/casbin/v2/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type countingDomainAdapter struct {
	inner *repository.DomainCasbinAdapter
	calls int
}

func (a *countingDomainAdapter) LoadPolicy(m casbinmodel.Model) error {
	a.calls++
	return a.inner.LoadPolicy(m)
}

func (a *countingDomainAdapter) SavePolicy(m casbinmodel.Model) error {
	return a.inner.SavePolicy(m)
}

func (a *countingDomainAdapter) AddPolicy(sec, ptype string, rule []string) error {
	return a.inner.AddPolicy(sec, ptype, rule)
}

func (a *countingDomainAdapter) RemovePolicy(sec, ptype string, rule []string) error {
	return a.inner.RemovePolicy(sec, ptype, rule)
}

func (a *countingDomainAdapter) RemoveFilteredPolicy(sec, ptype string, fieldIndex int, fieldValues ...string) error {
	return a.inner.RemoveFilteredPolicy(sec, ptype, fieldIndex, fieldValues...)
}

func TestDomainPolicy_ReloadOnlyOnMutations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Domain{}, &model.DomainMember{}, &model.DomainRole{}, &model.DomainRolePermission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	domain := &model.Domain{Name: "A", OwnerUUID: "owner-a"}
	if err := db.Create(domain).Error; err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := repository.SeedDefaultDomainRoles(db, domain.UUID); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	adapter := &countingDomainAdapter{inner: repository.NewDomainCasbinAdapter(db)}
	svc, err := NewDomainService(repository.NewDomainRepository(db), repository.NewDomainRoleRepository(db), nil, adapter)
	if err != nil {
		t.Fatalf("new domain service: %v", err)
	}
	baseline := adapter.calls
	for range 10 {
		svc.HasDomainPermission(domain.UUID, "new-user", permcode.PermRoomRead)
	}
	if adapter.calls != baseline {
		t.Fatalf("LoadPolicy called %d times during read-only checks, want no extra calls", adapter.calls-baseline)
	}
	if err := svc.AddMember(domain.UUID, "new-user"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if adapter.calls <= baseline {
		t.Fatal("expected policy reload after AddMember")
	}
	if !svc.HasDomainPermission(domain.UUID, "new-user", permcode.PermRoomRead) {
		t.Fatal("new member should have room:read immediately after AddMember")
	}
}
