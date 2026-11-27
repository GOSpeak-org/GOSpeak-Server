package repository

import (
	"testing"

	"GOSpeak/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newDomainTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Domain{}, &model.DomainMember{}, &model.Room{}, &model.User{},
		&model.DomainRole{}, &model.DomainRolePermission{},
		&model.DomainInvite{}, &model.DomainInviteRedemption{}, &model.DomainJoinRequest{},
		&model.DomainRoomGroup{}, &model.DomainRoomLayout{}, &model.UserDomainLayoutItem{},
		&model.Message{}, &model.MessageReaction{}, &model.MessageMention{},
		&model.Announcement{}, &model.AnnouncementRead{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDomainRepo_Create(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test Domain", OwnerUUID: "owner-uuid-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if domain.UUID == "" {
		t.Fatal("expected UUID to be set after create")
	}

	var saved model.Domain
	if err := db.First(&saved, "uuid = ?", domain.UUID).Error; err != nil {
		t.Fatalf("domain not found in DB: %v", err)
	}
	if saved.Name != "Test Domain" {
		t.Fatalf("expected name 'Test Domain', got %q", saved.Name)
	}
}

func TestDomainRepo_GetByUUID(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	got, err := repo.GetByUUID(domain.UUID)
	if err != nil {
		t.Fatalf("GetByUUID error: %v", err)
	}
	if got.Name != "Test" {
		t.Fatalf("expected name 'Test', got %q", got.Name)
	}
}
func TestDomainRepo_List(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	for _, name := range []string{"Domain-A", "Domain-B", "Domain-C", "Domain-D", "Domain-E"} {
		g := &model.Domain{
			Name:      name,
			OwnerUUID: "owner-1",
		}
		if err := repo.Create(g); err != nil {
			t.Fatalf("Create error: %v", err)
		}
	}

	domains, total, err := repo.List(1, 3)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(domains) != 3 {
		t.Fatalf("expected 3 domains, got %d", len(domains))
	}
	if total != 5 {
		t.Fatalf("expected total=5, got %d", total)
	}

	// page 2
	domains2, _, err := repo.List(2, 3)
	if err != nil {
		t.Fatalf("List page2 error: %v", err)
	}
	if len(domains2) != 2 {
		t.Fatalf("expected 2 domains on page2, got %d", len(domains2))
	}
}

func TestDomainRepo_ListPublic(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	for i := 0; i < 3; i++ {
		g := &model.Domain{
			Name:      "Public-" + string(rune('A'+i)),
			OwnerUUID: "owner-1",
			IsPublic:  true,
		}
		if err := repo.Create(g); err != nil {
			t.Fatalf("Create error: %v", err)
		}
	}
	// private domain
	g := &model.Domain{Name: "Private", OwnerUUID: "owner-1", IsPublic: false}
	if err := repo.Create(g); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	domains, total, err := repo.ListPublic(1, 10, "")
	if err != nil {
		t.Fatalf("ListPublic error: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if len(domains) != 3 {
		t.Fatalf("expected 3 domains, got %d", len(domains))
	}
}

func TestDomainRepo_AddMember(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	member := &model.DomainMember{
		DomainUUID: domain.UUID,
		UserUUID:   "user-1",
		RoleName:   "member",
	}
	if err := repo.AddMember(member); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}

	var saved model.DomainMember
	if err := db.Where("domain_uuid = ? AND user_uuid = ?", domain.UUID, "user-1").First(&saved).Error; err != nil {
		t.Fatalf("member not found: %v", err)
	}
	if saved.RoleName != "member" {
		t.Fatalf("expected role 'member', got %q", saved.RoleName)
	}
}

func TestDomainRepo_RemoveMember(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	member := &model.DomainMember{DomainUUID: domain.UUID, UserUUID: "user-1"}
	if err := repo.AddMember(member); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}

	if err := repo.RemoveMember(domain.UUID, "user-1"); err != nil {
		t.Fatalf("RemoveMember error: %v", err)
	}

	var count int64
	db.Model(&model.DomainMember{}).Where("domain_uuid = ? AND user_uuid = ?", domain.UUID, "user-1").Count(&count)
	if count != 0 {
		t.Fatal("expected no rows after remove")
	}
}

func TestDomainRepo_GetMember(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	member := &model.DomainMember{DomainUUID: domain.UUID, UserUUID: "user-1", RoleName: "admin"}
	if err := repo.AddMember(member); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}

	got, err := repo.GetMember(domain.UUID, "user-1")
	if err != nil {
		t.Fatalf("GetMember error: %v", err)
	}
	if got.RoleName != "admin" {
		t.Fatalf("expected role 'admin', got %q", got.RoleName)
	}

	_, err = repo.GetMember(domain.UUID, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent member")
	}
}

func TestDomainRepo_ListMembers(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	for i := 0; i < 3; i++ {
		uid := string(rune('A' + i))
		member := &model.DomainMember{DomainUUID: domain.UUID, UserUUID: uid, RoleName: "member"}
		if err := repo.AddMember(member); err != nil {
			t.Fatalf("AddMember error: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		uid := string(rune('A' + i))
		user := &model.User{UUID: uid, Name: "user-" + uid, DisplayName: "User " + uid}
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user %s: %v", uid, err)
		}
	}

	members, err := repo.ListMembers(domain.UUID)
	if err != nil {
		t.Fatalf("ListMembers error: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	if members[0].Name != "user-A" || members[0].DisplayName != "User A" {
		t.Fatalf("expected member user name, got %q / %q", members[0].Name, members[0].DisplayName)
	}
}

func TestDomainRepo_ListUserDomains(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	g1 := &model.Domain{Name: "G1", OwnerUUID: "o1"}
	if err := repo.Create(g1); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	g2 := &model.Domain{Name: "G2", OwnerUUID: "o2"}
	if err := repo.Create(g2); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	repo.AddMember(&model.DomainMember{DomainUUID: g1.UUID, UserUUID: "user-x"})
	repo.AddMember(&model.DomainMember{DomainUUID: g2.UUID, UserUUID: "user-x"})

	uuids, err := repo.ListUserDomains("user-x")
	if err != nil {
		t.Fatalf("ListUserDomains error: %v", err)
	}
	if len(uuids) != 2 {
		t.Fatalf("expected 2 domain UUIDs, got %d", len(uuids))
	}
}

func TestDomainRepo_CountMembers(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	for i := 0; i < 4; i++ {
		uid := string(rune('A' + i))
		repo.AddMember(&model.DomainMember{DomainUUID: domain.UUID, UserUUID: uid})
	}

	count, err := repo.CountMembers(domain.UUID)
	if err != nil {
		t.Fatalf("CountMembers error: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 members, got %d", count)
	}
}

func TestDomainRepo_Delete(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if err := repo.Delete(domain.UUID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	var count int64
	db.Model(&model.Domain{}).Where("uuid = ?", domain.UUID).Count(&count)
	if count != 0 {
		t.Fatal("expected no rows after delete")
	}
}

func TestDomainRepo_Delete_NotFound(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	// Deleting a non-existent UUID should not error (GORM returns nil for zero rows)
	if err := repo.Delete("nonexistent-uuid"); err != nil {
		t.Fatalf("Delete nonexistent error: %v", err)
	}
}

func TestDomainRepo_Delete_CleansMembersAndRooms(t *testing.T) {
	db := newDomainTestDB(t)
	repo := NewDomainRepository(db)

	domain := &model.Domain{Name: "Test", OwnerUUID: "owner-1"}
	if err := repo.Create(domain); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := repo.AddMember(&model.DomainMember{DomainUUID: domain.UUID, UserUUID: "member-1"}); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}
	if err := db.Create(&model.Room{Name: "room-1", DomainUUID: domain.UUID}).Error; err != nil {
		t.Fatalf("Create room error: %v", err)
	}

	if err := repo.Delete(domain.UUID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	var count int64
	db.Model(&model.Domain{}).Where("uuid = ?", domain.UUID).Count(&count)
	if count != 0 {
		t.Fatalf("expected domain removed, count=%d", count)
	}
	db.Model(&model.DomainMember{}).Where("domain_uuid = ?", domain.UUID).Count(&count)
	if count != 0 {
		t.Fatalf("expected members removed, count=%d", count)
	}
	db.Model(&model.Room{}).Where("domain_uuid = ?", domain.UUID).Count(&count)
	if count != 0 {
		t.Fatalf("expected rooms removed, count=%d", count)
	}
}

func TestCreateWithOwner_SeedsDefaultDomainRoles(t *testing.T) {
	db := newDomainTestDB(t)
	if err := db.AutoMigrate(&model.DomainRole{}, &model.DomainRolePermission{}); err != nil {
		t.Fatalf("migrate roles: %v", err)
	}
	repo := NewDomainRepository(db)
	domain := &model.Domain{Name: "Roles", OwnerUUID: "owner-1"}
	owner := &model.DomainMember{DomainUUID: "", UserUUID: "owner-1", RoleName: model.DomainRoleOwner}
	if err := repo.CreateWithOwner(domain, owner); err != nil {
		t.Fatalf("CreateWithOwner: %v", err)
	}

	roleRepo := NewDomainRoleRepository(db)
	roles, err := roleRepo.ListRoles(domain.UUID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) != 3 {
		t.Fatalf("expected 3 seeded roles, got %d", len(roles))
	}
}

func newDomainCascadeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Domain{}, &model.DomainMember{}, &model.Room{},
		&model.DomainRole{}, &model.DomainRolePermission{},
		&model.DomainInvite{}, &model.DomainInviteRedemption{}, &model.DomainJoinRequest{},
		&model.DomainRoomGroup{}, &model.DomainRoomLayout{}, &model.UserDomainLayoutItem{},
		&model.Message{}, &model.MessageReaction{}, &model.MessageMention{},
		&model.ConversationParticipant{},
		&model.Announcement{}, &model.AnnouncementRead{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDomainRepo_Delete_CascadesAllRelatedRows(t *testing.T) {
	db := newDomainCascadeTestDB(t)
	repo := NewDomainRepository(db)

	target := &model.Domain{Name: "target", OwnerUUID: "owner-t"}
	if err := repo.Create(target); err != nil {
		t.Fatalf("create target domain: %v", err)
	}
	other := &model.Domain{Name: "other", OwnerUUID: "owner-o"}
	if err := repo.Create(other); err != nil {
		t.Fatalf("create other domain: %v", err)
	}

	seedDomainRows := func(domainUUID, suffix string) uint {
		if err := repo.AddMember(&model.DomainMember{DomainUUID: domainUUID, UserUUID: "user-" + suffix}); err != nil {
			t.Fatalf("add member %s: %v", suffix, err)
		}
		if err := db.Create(&model.DomainRole{DomainUUID: domainUUID, Name: model.DomainRoleAdmin}).Error; err != nil {
			t.Fatalf("create role %s: %v", suffix, err)
		}
		if err := db.Create(&model.DomainRolePermission{DomainUUID: domainUUID, RoleName: model.DomainRoleAdmin, PermissionCode: model.PermDomainManage}).Error; err != nil {
			t.Fatalf("create role permission %s: %v", suffix, err)
		}
		invite := &model.DomainInvite{DomainUUID: domainUUID, Code: "code-" + suffix}
		if err := db.Create(invite).Error; err != nil {
			t.Fatalf("create invite %s: %v", suffix, err)
		}
		redemption := &model.DomainInviteRedemption{
			InviteID: invite.ID, DomainUUID: domainUUID, UserUUID: "user-" + suffix,
			Code: invite.Code, Success: true,
		}
		if err := db.Create(redemption).Error; err != nil {
			t.Fatalf("create redemption %s: %v", suffix, err)
		}
		if err := db.Create(&model.DomainJoinRequest{DomainUUID: domainUUID, UserUUID: "joiner-" + suffix, InviteID: invite.ID}).Error; err != nil {
			t.Fatalf("create join request %s: %v", suffix, err)
		}
		if err := db.Create(&model.DomainRoomGroup{DomainUUID: domainUUID, Name: "group-" + suffix}).Error; err != nil {
			t.Fatalf("create room group %s: %v", suffix, err)
		}
		if err := db.Create(&model.DomainRoomLayout{DomainUUID: domainUUID, UngroupedPosition: 1}).Error; err != nil {
			t.Fatalf("create room layout %s: %v", suffix, err)
		}
		if err := db.Create(&model.UserDomainLayoutItem{UserUUID: "user-" + suffix, DomainUUID: domainUUID}).Error; err != nil {
			t.Fatalf("create layout item %s: %v", suffix, err)
		}
		return invite.ID
	}

	targetInviteID := seedDomainRows(target.UUID, "t")
	seedDomainRows(other.UUID, "o")

	seedRoomMessages := func(domainUUID, roomName, msgUUID string) string {
		room := &model.Room{Name: roomName, DomainUUID: domainUUID}
		if err := db.Create(room).Error; err != nil {
			t.Fatalf("create room %s: %v", roomName, err)
		}
		msg := &model.Message{UUID: msgUUID, RoomUUID: room.UUID, AuthorID: "alice", Content: "hello"}
		if err := db.Create(msg).Error; err != nil {
			t.Fatalf("create message %s: %v", msgUUID, err)
		}
		if err := db.Create(&model.MessageReaction{MessageUUID: msgUUID, UserID: "alice", Emoji: "ok"}).Error; err != nil {
			t.Fatalf("create reaction %s: %v", msgUUID, err)
		}
		if err := db.Create(&model.MessageMention{MessageUUID: msgUUID, UserID: "bob"}).Error; err != nil {
			t.Fatalf("create mention %s: %v", msgUUID, err)
		}
		softMsg := &model.Message{UUID: msgUUID + "-soft", RoomUUID: room.UUID, AuthorID: "alice", Content: "gone"}
		if err := db.Create(softMsg).Error; err != nil {
			t.Fatalf("create soft message %s: %v", msgUUID, err)
		}
		if err := db.Delete(softMsg).Error; err != nil {
			t.Fatalf("soft delete message %s: %v", msgUUID, err)
		}
		if err := db.Create(&model.MessageReaction{MessageUUID: softMsg.UUID, UserID: "alice", Emoji: "ok"}).Error; err != nil {
			t.Fatalf("create soft reaction %s: %v", msgUUID, err)
		}
		return room.UUID
	}

	targetRoomUUID := seedRoomMessages(target.UUID, "target-room", "msg-target")
	seedRoomMessages(other.UUID, "other-room", "msg-other")

	// 域级公告及其已读记录应随域删除
	targetAnn := &model.Announcement{UUID: "ann-target", ScopeType: model.AnnouncementScopeDomain, ScopeID: target.UUID, Title: "t"}
	if err := db.Create(targetAnn).Error; err != nil {
		t.Fatalf("create target announcement: %v", err)
	}
	if err := db.Create(&model.AnnouncementRead{AnnouncementUUID: targetAnn.UUID, UserUUID: "user-t"}).Error; err != nil {
		t.Fatalf("create announcement read: %v", err)
	}
	if err := db.Create(&model.Announcement{UUID: "ann-other", ScopeType: model.AnnouncementScopeDomain, ScopeID: other.UUID, Title: "t"}).Error; err != nil {
		t.Fatalf("create other announcement: %v", err)
	}

	privateConv := &model.ConversationParticipant{ConversationID: "conv-private", IdentityA: "alice", IdentityB: "bob"}
	if err := db.Create(privateConv).Error; err != nil {
		t.Fatalf("create private conversation: %v", err)
	}

	countRows := func(value interface{}, query string, arg interface{}) int64 {
		var n int64
		if err := db.Unscoped().Model(value).Where(query, arg).Count(&n).Error; err != nil {
			t.Fatalf("count %T: %v", value, err)
		}
		return n
	}

	type check struct {
		name  string
		value interface{}
		query string
		arg   interface{}
	}
	targetChecks := []check{
		{"domains", &model.Domain{}, "uuid = ?", target.UUID},
		{"domain_members", &model.DomainMember{}, "domain_uuid = ?", target.UUID},
		{"domain_roles", &model.DomainRole{}, "domain_uuid = ?", target.UUID},
		{"domain_role_permissions", &model.DomainRolePermission{}, "domain_uuid = ?", target.UUID},
		{"domain_invites", &model.DomainInvite{}, "domain_uuid = ?", target.UUID},
		{"domain_invite_redemptions by domain_uuid", &model.DomainInviteRedemption{}, "domain_uuid = ?", target.UUID},
		{"domain_invite_redemptions by invite_id", &model.DomainInviteRedemption{}, "invite_id = ?", targetInviteID},
		{"domain_join_requests", &model.DomainJoinRequest{}, "domain_uuid = ?", target.UUID},
		{"domain_room_groups", &model.DomainRoomGroup{}, "domain_uuid = ?", target.UUID},
		{"domain_room_layouts", &model.DomainRoomLayout{}, "domain_uuid = ?", target.UUID},
		{"user_domain_layout_items", &model.UserDomainLayoutItem{}, "domain_uuid = ?", target.UUID},
		{"rooms", &model.Room{}, "domain_uuid = ?", target.UUID},
		{"room messages", &model.Message{}, "room_uuid = ?", targetRoomUUID},
		{"soft-deleted room messages", &model.Message{}, "room_uuid = ?", targetRoomUUID},
		{"message reactions", &model.MessageReaction{}, "message_uuid = ?", "msg-target"},
		{"soft-deleted message reactions", &model.MessageReaction{}, "message_uuid = ?", "msg-target-soft"},
		{"message mentions", &model.MessageMention{}, "message_uuid = ?", "msg-target"},
		{"domain announcements", &model.Announcement{}, "scope_id = ?", target.UUID},
		{"announcement reads", &model.AnnouncementRead{}, "announcement_uuid = ?", "ann-target"},
	}
	if err := repo.Delete(target.UUID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	for _, c := range targetChecks {
		if n := countRows(c.value, c.query, c.arg); n != 0 {
			t.Errorf("expected %s removed after Delete, got %d rows", c.name, n)
		}
	}

	otherChecks := []check{
		{"other domain", &model.Domain{}, "uuid = ?", other.UUID},
		{"other domain invites", &model.DomainInvite{}, "domain_uuid = ?", other.UUID},
		{"other domain rooms", &model.Room{}, "domain_uuid = ?", other.UUID},
		{"other domain room messages", &model.Message{}, "uuid = ?", "msg-other"},
		{"private conversation", &model.ConversationParticipant{}, "conversation_id = ?", privateConv.ConversationID},
		{"other domain announcement", &model.Announcement{}, "uuid = ?", "ann-other"},
	}
	for _, c := range otherChecks {
		if n := countRows(c.value, c.query, c.arg); n != 1 {
			t.Errorf("expected %s preserved after Delete, got %d rows", c.name, n)
		}
	}
}
