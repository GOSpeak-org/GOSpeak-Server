package signal

import (
	"errors"
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/sfu"
)

type capsProvider struct {
	sfu.BaseProvider
	caps        sfu.Capabilities
	kickErr     error
	muteErr     error
	muteResults []error
	kicked      []string
	muted       []string
	ttlSeen     int
	timed       bool
}

func (p *capsProvider) ProviderName() string           { return "test" }
func (p *capsProvider) Capabilities() sfu.Capabilities { return p.caps }
func (p *capsProvider) GenerateToken(string, string) (string, error) {
	return "t", nil
}
func (p *capsProvider) ListRooms() ([]sfu.RoomSummary, error) { return nil, nil }
func (p *capsProvider) ListParticipants(string) ([]sfu.ParticipantSummary, error) {
	return nil, nil
}
func (p *capsProvider) MuteParticipant(room, identity, trackSid string, muted bool) error {
	p.muted = append(p.muted, room+":"+identity)
	if len(p.muteResults) > 0 {
		err := p.muteResults[0]
		p.muteResults = p.muteResults[1:]
		return err
	}
	return p.muteErr
}
func (p *capsProvider) MuteParticipantTimed(room, identity, trackSid string, muted bool, ttlSeconds int) error {
	p.timed = true
	p.ttlSeen = ttlSeconds
	return p.MuteParticipant(room, identity, trackSid, muted)
}
func (p *capsProvider) RemoveParticipant(room, identity string) error {
	p.kicked = append(p.kicked, room+":"+identity)
	return p.kickErr
}
func (p *capsProvider) DeleteRoom(string) error { return nil }
func (p *capsProvider) GetHost() string         { return "host" }

type idUserStore struct {
	users map[uint]*model.User
}

func (s *idUserStore) GetByName(name string) (*model.User, error) {
	for _, u := range s.users {
		if u.Name == name {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (s *idUserStore) GetByNames(names []string) (map[string]*model.User, error) {
	out := make(map[string]*model.User, len(names))
	for _, name := range names {
		for _, u := range s.users {
			if u != nil && u.Name == name {
				out[name] = u
			}
		}
	}
	return out, nil
}

func (s *idUserStore) GetByID(id uint) (*model.User, error) {
	if u, ok := s.users[id]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}

func (s *idUserStore) GetByUUID(uuid string) (*model.User, error) {
	for _, u := range s.users {
		if u != nil && u.UUID == uuid {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func TestRemoveParticipantSafe_SoftWhenNoServerKick(t *testing.T) {
	hub := NewHub(nil, nil, nil, nil)
	p := &capsProvider{caps: sfu.Capabilities{KickLevel: sfu.EnforcementSoft}}
	hub.SetSFU(p)
	if got := hub.removeParticipantSafe("r1", "alice"); got != sfu.EnforcementSoft {
		t.Fatalf("enforcement=%q, want soft", got)
	}
	if len(p.kicked) != 0 {
		t.Fatalf("expected no SFU kick call, got %v", p.kicked)
	}
}

func TestRemoveParticipantSafe_HardWhenSupported(t *testing.T) {
	hub := NewHub(nil, nil, nil, nil)
	p := &capsProvider{caps: sfu.Capabilities{ServerKick: true, KickLevel: sfu.EnforcementHard}}
	hub.SetSFU(p)
	if got := hub.removeParticipantSafe("r1", "alice"); got != sfu.EnforcementHard {
		t.Fatalf("enforcement=%q, want hard", got)
	}
}

func TestRemoveParticipantSafe_DegradedWhenConfigured(t *testing.T) {
	hub := NewHub(nil, nil, nil, nil)
	p := &capsProvider{caps: sfu.Capabilities{ServerKick: true, KickLevel: sfu.EnforcementDegraded}}
	hub.SetSFU(p)
	if got := hub.removeParticipantSafe("r1", "alice"); got != sfu.EnforcementDegraded {
		t.Fatalf("enforcement=%q, want degraded", got)
	}
}

func TestRemoveParticipantSafe_SoftOnSFUError(t *testing.T) {
	hub := NewHub(nil, nil, nil, nil)
	p := &capsProvider{
		caps:    sfu.Capabilities{ServerKick: true, KickLevel: sfu.EnforcementHard},
		kickErr: errors.New("upstream down"),
	}
	hub.SetSFU(p)
	if got := hub.removeParticipantSafe("r1", "alice"); got != sfu.EnforcementSoft {
		t.Fatalf("enforcement=%q, want soft", got)
	}
}

func TestEnforceUserMediaMute_SoftWithoutServerMute(t *testing.T) {
	hub := NewHub(nil, nil, &idUserStore{users: map[uint]*model.User{
		1: {ID: 1, UUID: "1", Name: "alice"},
	}}, nil)
	p := &capsProvider{caps: sfu.Capabilities{MuteLevel: sfu.EnforcementSoft}}
	hub.SetSFU(p)
	if got := hub.enforceUserMediaMute("1", true, 60); got != sfu.EnforcementSoft {
		t.Fatalf("enforcement=%q, want soft", got)
	}
	if len(p.muted) != 0 {
		t.Fatalf("expected no mute call, got %v", p.muted)
	}
}

func TestEnforceUserMediaMute_DegradedWhenOnlineAndSupported(t *testing.T) {
	hub := NewHub(nil, nil, &idUserStore{users: map[uint]*model.User{
		1: {ID: 1, UUID: "1", Name: "alice"},
	}}, nil)
	p := &capsProvider{caps: sfu.Capabilities{ServerMute: true, MuteLevel: sfu.EnforcementDegraded}}
	hub.SetSFU(p)
	hub.rooms["lobby"] = &Room{
		Name: "lobby",
		Members: map[string]*MemberInfo{
			"sid-1": {Identity: "alice"},
		},
		MicMuted: make(map[string]bool),
		Speaking: make(map[string]bool),
	}
	if got := hub.enforceUserMediaMute("1", true, 120); got != sfu.EnforcementDegraded {
		t.Fatalf("enforcement=%q, want degraded", got)
	}
	if !p.timed || p.ttlSeen != 120 {
		t.Fatalf("expected timed mute ttl=120, timed=%v ttl=%d", p.timed, p.ttlSeen)
	}
}

func TestEnforceUserMediaMute_PartialMultiRoomIsSoft(t *testing.T) {
	hub := NewHub(nil, nil, &idUserStore{users: map[uint]*model.User{
		1: {ID: 1, UUID: "1", Name: "alice"},
	}}, nil)
	p := &capsProvider{
		caps:        sfu.Capabilities{ServerMute: true, MuteLevel: sfu.EnforcementHard},
		muteResults: []error{nil, errors.New("second room down")},
	}
	hub.SetSFU(p)
	hub.rooms["a"] = &Room{Name: "a", Members: map[string]*MemberInfo{"1": {Identity: "alice"}}, MicMuted: map[string]bool{}, Speaking: map[string]bool{}}
	hub.rooms["b"] = &Room{Name: "b", Members: map[string]*MemberInfo{"2": {Identity: "alice"}}, MicMuted: map[string]bool{}, Speaking: map[string]bool{}}
	if got := hub.enforceUserMediaMute("1", true, 10); got != sfu.EnforcementSoft {
		t.Fatalf("enforcement=%q, want soft when one of two rooms fails", got)
	}
	if len(p.muted) != 2 {
		t.Fatalf("expected mute attempts in both rooms, got %v", p.muted)
	}
	attempted := map[string]bool{}
	for _, target := range p.muted {
		attempted[target] = true
	}
	if !attempted["a:alice"] || !attempted["b:alice"] {
		t.Fatalf("expected a:alice and b:alice mute attempts, got %v", p.muted)
	}
}

func TestEnforceUserMediaMute_AllRoomsSucceedIsHard(t *testing.T) {
	hub := NewHub(nil, nil, &idUserStore{users: map[uint]*model.User{
		1: {ID: 1, UUID: "1", Name: "alice"},
	}}, nil)
	p := &capsProvider{caps: sfu.Capabilities{ServerMute: true, MuteLevel: sfu.EnforcementHard}}
	hub.SetSFU(p)
	hub.rooms["a"] = &Room{Name: "a", Members: map[string]*MemberInfo{"1": {Identity: "alice"}}, MicMuted: map[string]bool{}, Speaking: map[string]bool{}}
	hub.rooms["b"] = &Room{Name: "b", Members: map[string]*MemberInfo{"2": {Identity: "alice"}}, MicMuted: map[string]bool{}, Speaking: map[string]bool{}}
	if got := hub.enforceUserMediaMute("1", true, 10); got != sfu.EnforcementHard {
		t.Fatalf("enforcement=%q, want hard when every room succeeds", got)
	}
	if len(p.muted) != 2 {
		t.Fatalf("expected mute attempts in both rooms, got %v", p.muted)
	}
}

func TestEnforcementFromLevelHelper(t *testing.T) {
	if sfu.EnforcementFromLevel(sfu.EnforcementHard) != sfu.EnforcementHard {
		t.Fatal("expected hard")
	}
	if sfu.EnforcementFromLevel(sfu.EnforcementDegraded) != sfu.EnforcementDegraded {
		t.Fatal("expected degraded")
	}
	if sfu.EnforcementFromLevel(sfu.EnforcementSoft) != sfu.EnforcementSoft {
		t.Fatal("expected soft")
	}
}

func TestBroadcastMute_PermanentUsesLongTTL(t *testing.T) {
	hub := NewHub(nil, nil, &idUserStore{users: map[uint]*model.User{
		1: {ID: 1, UUID: "1", Name: "alice"},
	}}, nil)
	p := &capsProvider{caps: sfu.Capabilities{ServerMute: true, MuteLevel: sfu.EnforcementDegraded}}
	hub.SetSFU(p)
	hub.rooms["lobby"] = &Room{
		Name: "lobby",
		Members: map[string]*MemberInfo{
			"sid-1": {Identity: "alice"},
		},
		MicMuted: make(map[string]bool),
		Speaking: make(map[string]bool),
	}

	hub.BroadcastMute("1", &MuteInfo{Permanent: true})
	if p.ttlSeen != sfu.PermanentMuteTTLSeconds {
		t.Fatalf("permanent mute ttl=%d, want %d", p.ttlSeen, sfu.PermanentMuteTTLSeconds)
	}
}
