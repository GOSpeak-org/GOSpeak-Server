package plugin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"GOSpeak/internal/audit"
	"GOSpeak/internal/logger"
	"GOSpeak/internal/model"
	"GOSpeak/internal/plugin/gen"

	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type apiFakeUserSvc struct {
	user *model.User
	err  error
	got  string
}

func (f *apiFakeUserSvc) GetByUUID(uuid string) (*model.User, error) {
	f.got = uuid
	return f.user, f.err
}

type apiFakeRoomSvc struct {
	mu    sync.Mutex
	rooms []model.Room
	err   error
	got   string
	last  *model.Room
}

func (f *apiFakeRoomSvc) List(page, pageSize int, roomType, domainUUID string) ([]model.Room, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = domainUUID
	return f.rooms, int64(len(f.rooms)), f.err
}

func (f *apiFakeRoomSvc) Create(room *model.Room) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = room
	return f.err
}

type apiFakeMuteSvc struct {
	mu   sync.Mutex
	err  error
	call int
	args [5]string
}

func (f *apiFakeMuteSvc) MuteUser(muterUUID, userUUID string, duration int64, permanent bool, reason string) (*model.Mute, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call++
	f.args = [5]string{muterUUID, userUUID, reason}
	return &model.Mute{UserUUID: userUUID, Reason: reason}, f.err
}

func (f *apiFakeMuteSvc) calls() int { f.mu.Lock(); defer f.mu.Unlock(); return f.call }

type auditRecorder struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (a *auditRecorder) Log(e audit.Entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
}
func (a *auditRecorder) count() int { a.mu.Lock(); defer a.mu.Unlock(); return len(a.entries) }

type auditFailRecorder struct{}

func (*auditFailRecorder) Log(audit.Entry) { panic("audit must be side-car only") }

type apiCountingDedup struct {
	mu      sync.Mutex
	entries map[string]string
}

func newAPICountingDedup() *apiCountingDedup {
	return &apiCountingDedup{entries: make(map[string]string)}
}

func (d *apiCountingDedup) Claim(pluginID, requestID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := pluginID + "\x00" + requestID
	if _, ok := d.entries[key]; ok {
		return true, nil
	}
	d.entries[key] = dedupStatusPending
	return false, nil
}

func (d *apiCountingDedup) Release(pluginID, requestID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, pluginID+"\x00"+requestID)
}

func (d *apiCountingDedup) MarkDone(pluginID, requestID, status string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[pluginID+"\x00"+requestID] = status
}

type apiConfigStoreSpy struct {
	mu  sync.Mutex
	got string
}

func (s *apiConfigStoreSpy) GetConfig(pluginID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = pluginID
	return "{}", nil
}

func newAPITestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.PluginConfig{}, &model.PluginKV{}, &model.PluginRequestLog{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func depsFor(db *gorm.DB, userSvc *apiFakeUserSvc, roomSvc *apiFakeRoomSvc, muteSvc *apiFakeMuteSvc, auditor apiAuditor) APIDeps {
	return APIDeps{UserSvc: userSvc, RoomSvc: roomSvc, MuteSvc: muteSvc, Auditor: auditor, ConfigRepo: NewPluginConfigStore(db), KVRepo: NewPluginKVStore(db), Dedup: NewPluginDedup(db)}
}

func newTestAPI(t *testing.T, db *gorm.DB, userSvc *apiFakeUserSvc, roomSvc *apiFakeRoomSvc, muteSvc *apiFakeMuteSvc, auditor apiAuditor) *APIService {
	t.Helper()
	api, err := NewAPIService(depsFor(db, userSvc, roomSvc, muteSvc, auditor))
	if err != nil {
		t.Fatalf("NewAPIService: %v", err)
	}
	return api.WithPluginID("test-plugin")
}

func TestAPIGetUserReadsUserService(t *testing.T) {
	userSvc := &apiFakeUserSvc{user: &model.User{UUID: "u-1", Name: "alice", DisplayName: "Alice"}}
	api := newTestAPI(t, newAPITestDB(t), userSvc, &apiFakeRoomSvc{}, &apiFakeMuteSvc{}, &auditRecorder{})
	resp, err := api.GetUser(context.Background(), &gen.GetUserRequest{UserUuid: "u-1"})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if resp.UserUuid != "u-1" || resp.Username != "alice" || resp.DisplayName != "Alice" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if userSvc.got != "u-1" {
		t.Fatalf("user service got %q", userSvc.got)
	}
}

func TestAPIMuteUserDelegatesAndAudits(t *testing.T) {
	db := newAPITestDB(t)
	muteSvc := &apiFakeMuteSvc{}
	auditor := &auditRecorder{}
	api := newTestAPI(t, db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, muteSvc, auditor)
	resp, err := api.MuteUser(context.Background(), &gen.MuteUserRequest{UserUuid: "u-1", RoomUuid: "r-1", Reason: "spam", RequestId: "req-1"})
	if err != nil || !resp.Ok {
		t.Fatalf("MuteUser: %+v err=%v", resp, err)
	}
	if muteSvc.calls() != 1 || muteSvc.args[0] != model.PluginSystemUserUUID || muteSvc.args[1] != "u-1" || muteSvc.args[2] != "spam" {
		t.Fatalf("unexpected mute call: %+v", muteSvc)
	}
	if auditor.count() != 1 {
		t.Fatalf("audit entries = %d, want 1", auditor.count())
	}
	e := auditor.entries[0]
	if e.ActorKind != "plugin" || e.ActorUUID != model.PluginSystemUserUUID || e.Action != audit.ActionMuteUser || e.TargetType != audit.TargetUser || e.TargetID != "u-1" || !e.Success {
		t.Fatalf("unexpected audit entry: %+v", e)
	}
}

func TestAPIMuteUserFailureNoAudit(t *testing.T) {
	db := newAPITestDB(t)
	muteSvc := &apiFakeMuteSvc{err: errors.New("boom")}
	auditor := &auditRecorder{}
	api := newTestAPI(t, db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, muteSvc, auditor)
	if _, err := api.MuteUser(context.Background(), &gen.MuteUserRequest{UserUuid: "u-1", RequestId: "req-fail"}); err == nil {
		t.Fatal("want error")
	}
	if auditor.count() != 0 {
		t.Fatalf("audit entries = %d, want 0", auditor.count())
	}
	var n int64
	db.Model(&model.PluginRequestLog{}).Count(&n)
	if n != 0 {
		t.Fatalf("request log rows = %d, want 0", n)
	}
}

func TestAPIMuteUserRequestIdempotent(t *testing.T) {
	db := newAPITestDB(t)
	muteSvc := &apiFakeMuteSvc{}
	auditor := &auditRecorder{}
	api := newTestAPI(t, db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, muteSvc, auditor)
	req := &gen.MuteUserRequest{UserUuid: "u-1", RequestId: "req-dup"}
	if _, err := api.MuteUser(context.Background(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	resp, err := api.MuteUser(context.Background(), req)
	if err != nil || !resp.Ok {
		t.Fatalf("second call: %+v err=%v", resp, err)
	}
	if muteSvc.calls() != 1 {
		t.Fatalf("mute called %d times, want 1", muteSvc.calls())
	}
	if auditor.count() != 1 {
		t.Fatalf("audit entries = %d, want 1", auditor.count())
	}
	var n int64
	db.Model(&model.PluginRequestLog{}).Where("plugin_id = ? AND request_id = ?", "test-plugin", "req-dup").Count(&n)
	if n != 1 {
		t.Fatalf("request log rows = %d, want 1", n)
	}
	var status string
	db.Model(&model.PluginRequestLog{}).Where("plugin_id = ? AND request_id = ?", "test-plugin", "req-dup").Pluck("status", &status)
	if status != dedupStatusSuccess {
		t.Fatalf("request status = %q, want %q", status, dedupStatusSuccess)
	}
}

func TestAPIConcurrentSameRequestExecutesOnce(t *testing.T) {
	db := newAPITestDB(t)
	muteSvc := &apiFakeMuteSvc{}
	auditor := &auditRecorder{}
	deps := depsFor(db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, muteSvc, auditor)
	deps.Dedup = newAPICountingDedup()
	api, err := NewAPIService(deps)
	if err != nil {
		t.Fatal(err)
	}
	api = api.WithPluginID("test-plugin")
	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := api.MuteUser(context.Background(), &gen.MuteUserRequest{UserUuid: "u-1", RequestId: "req-concurrent"}); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("MuteUser concurrent: %v", err)
	}
	if calls := muteSvc.calls(); calls != 1 {
		t.Fatalf("mute called %d times, want 1", calls)
	}
	if auditor.count() != 1 {
		t.Fatalf("audit entries = %d, want 1", auditor.count())
	}
}

func TestAPIAuditFailureDoesNotCauseRetryExecution(t *testing.T) {
	db := newAPITestDB(t)
	muteSvc := &apiFakeMuteSvc{}
	api := newTestAPI(t, db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, muteSvc, &auditFailRecorder{})
	req := &gen.MuteUserRequest{UserUuid: "u-1", RequestId: "req-audit-fail"}
	for i := 0; i < 2; i++ {
		if _, err := api.MuteUser(context.Background(), req); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if muteSvc.calls() != 1 {
		t.Fatalf("mute called %d times, want 1", muteSvc.calls())
	}
}

func TestAPINewServiceRejectsNilDependencies(t *testing.T) {
	db := newAPITestDB(t)
	deps := depsFor(db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, &apiFakeMuteSvc{}, &auditRecorder{})
	if _, err := NewAPIService(deps); err != nil {
		t.Fatalf("valid deps: %v", err)
	}
	deps.Dedup = nil
	if _, err := NewAPIService(deps); err == nil {
		t.Fatal("nil dedup should fail")
	}
}

func TestAPICreateRoomDelegatesAndAudits(t *testing.T) {
	db := newAPITestDB(t)
	roomSvc := &apiFakeRoomSvc{}
	auditor := &auditRecorder{}
	api := newTestAPI(t, db, &apiFakeUserSvc{}, roomSvc, &apiFakeMuteSvc{}, auditor)
	resp, err := api.CreateRoom(context.Background(), &gen.CreateRoomRequest{Name: "room", DomainUuid: "d-1", Reason: "test", RequestId: "cr-1"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	roomSvc.mu.Lock()
	last := *roomSvc.last
	roomSvc.mu.Unlock()
	if last.Name != "room" || last.DomainUUID != "d-1" || last.CreatedBy != model.PluginSystemUserUUID {
		t.Fatalf("unexpected CreateRoom call: %+v", last)
	}
	if auditor.count() != 1 {
		t.Fatalf("unexpected response/audit: %+v %d", resp, auditor.count())
	}
}

func TestAPIListRoomsDelegatesToService(t *testing.T) {
	db := newAPITestDB(t)
	roomSvc := &apiFakeRoomSvc{rooms: []model.Room{{UUID: "r-1", Name: "first"}}}
	api := newTestAPI(t, db, &apiFakeUserSvc{}, roomSvc, &apiFakeMuteSvc{}, &auditRecorder{})
	resp, err := api.ListRooms(context.Background(), &gen.ListRoomsRequest{DomainUuid: "d-1"})
	if err != nil || len(resp.Rooms) != 1 || resp.Rooms[0].RoomUuid != "r-1" {
		t.Fatalf("ListRooms: %+v err=%v", resp, err)
	}
	roomSvc.mu.Lock()
	got := roomSvc.got
	roomSvc.mu.Unlock()
	if got != "d-1" {
		t.Fatalf("ListRooms domain got %q", got)
	}
}

func TestAPIGetConfigDelegatesToStore(t *testing.T) {
	db := newAPITestDB(t)
	spy := &apiConfigStoreSpy{}
	deps := depsFor(db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, &apiFakeMuteSvc{}, &auditRecorder{})
	deps.ConfigRepo = spy
	api, err := NewAPIService(deps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.GetConfig(context.Background(), &gen.GetConfigRequest{PluginId: "p-1"}); err != nil {
		t.Fatal(err)
	}
	spy.mu.Lock()
	got := spy.got
	spy.mu.Unlock()
	if got != "p-1" {
		t.Fatalf("GetConfig got %q", got)
	}
}

func TestAPIConfigAndKVIsolatedByPluginID(t *testing.T) {
	db := newAPITestDB(t)
	api := newTestAPI(t, db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, &apiFakeMuteSvc{}, &auditRecorder{})
	db.Create(&model.PluginConfig{PluginID: "plugin-a", ConfigJSON: string([]byte{123, 34, 97, 34, 58, 49, 125})})
	db.Create(&model.PluginConfig{PluginID: "plugin-b", ConfigJSON: string([]byte{123, 34, 98, 34, 58, 50, 125})})
	cfg, err := api.GetConfig(context.Background(), &gen.GetConfigRequest{PluginId: "plugin-a"})
	if err != nil || cfg.ConfigJson != string([]byte{123, 34, 97, 34, 58, 49, 125}) {
		t.Fatalf("GetConfig: %+v err=%v", cfg, err)
	}
	cfg, err = api.GetConfig(context.Background(), &gen.GetConfigRequest{PluginId: "plugin-b"})
	if err != nil || cfg.ConfigJson != string([]byte{123, 34, 98, 34, 58, 50, 125}) {
		t.Fatalf("GetConfig b: %+v err=%v", cfg, err)
	}
	if _, err := api.KVSave(context.Background(), &gen.KVSaveRequest{PluginId: "plugin-a", Key: "k", ValueJson: "1", RequestId: "kv-1"}); err != nil {
		t.Fatalf("KVSave: %v", err)
	}
	loaded, err := api.KVLoad(context.Background(), &gen.KVLoadRequest{PluginId: "plugin-a", Key: "k"})
	if err != nil || !loaded.Found || loaded.ValueJson != "1" {
		t.Fatalf("KVLoad a: %+v err=%v", loaded, err)
	}
	loaded, err = api.KVLoad(context.Background(), &gen.KVLoadRequest{PluginId: "plugin-b", Key: "k"})
	if err != nil || loaded.Found {
		t.Fatalf("KVLoad b: %+v err=%v", loaded, err)
	}
	if _, err := api.KVSave(context.Background(), &gen.KVSaveRequest{PluginId: "plugin-a", Key: "k", ValueJson: "2", RequestId: "kv-1"}); err != nil {
		t.Fatalf("KVSave duplicate: %v", err)
	}
	loaded, _ = api.KVLoad(context.Background(), &gen.KVLoadRequest{PluginId: "plugin-a", Key: "k"})
	if loaded.ValueJson != "1" {
		t.Fatalf("idempotent KVSave overwrote value: %s", loaded.ValueJson)
	}
}

func TestAPIReportErrorLogsMessage(t *testing.T) {
	db := newAPITestDB(t)
	api := newTestAPI(t, db, &apiFakeUserSvc{}, &apiFakeRoomSvc{}, &apiFakeMuteSvc{}, &auditRecorder{})
	prevOut := logger.L().Out
	prevFmt := logger.L().Formatter
	var buf bytes.Buffer
	logger.L().SetOutput(&buf)
	logger.L().SetFormatter(&logrus.JSONFormatter{})
	t.Cleanup(func() { logger.L().SetOutput(prevOut); logger.L().SetFormatter(prevFmt) })
	if _, err := api.ReportError(context.Background(), &gen.ReportErrorRequest{PluginId: "plugin-a", Message: "crashed", DetailsJson: "{}", RequestId: "err-1"}); err != nil {
		t.Fatalf("ReportError: %v", err)
	}
	var n int64
	db.Model(&model.PluginRequestLog{}).Where("plugin_id = ? AND request_id = ?", "plugin-a", "err-1").Count(&n)
	if n != 1 {
		t.Fatalf("error report request log rows = %d, want 1", n)
	}
	out := buf.String()
	if !strings.Contains(out, `"component":"plugin:plugin-a"`) || !strings.Contains(out, "crashed") {
		t.Fatalf("log missing component or message: %s", out)
	}
}

func TestAPIDedupSchemaHasUniqueIndex(t *testing.T) {
	db := newAPITestDB(t)
	if !db.Migrator().HasIndex(&model.PluginRequestLog{}, "idx_plugin_request_log_plugin_request") {
		t.Fatal("missing unique index")
	}
	if err := db.Create(&model.PluginRequestLog{PluginID: "p", RequestID: "r", Status: dedupStatusPending}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PluginRequestLog{PluginID: "p", RequestID: "r", Status: dedupStatusPending}).Error; err == nil {
		t.Fatal("duplicate claim should fail on unique index")
	}
}
