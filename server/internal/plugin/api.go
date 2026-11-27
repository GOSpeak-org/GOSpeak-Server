package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"GOSpeak/internal/audit"
	"GOSpeak/internal/logger"
	"GOSpeak/internal/model"
	"GOSpeak/internal/plugin/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const (
	pluginAuditActorName = "plugin"
	dedupStatusPending   = "pending"
	dedupStatusSuccess   = "success"
)

// APIService 实现 PluginAPIServiceServer。写操作统一以系统用户身份委托现有 service，
// 成功后再记录 ActorKind=plugin 的审计，审计旁路不阻断结果。
type APIService struct {
	gen.UnimplementedPluginAPIServiceServer
	userSvc    apiUserService
	roomSvc    apiRoomService
	muteSvc    apiMuteService
	auditor    apiAuditor
	configRepo APIConfigStore
	kvRepo     APIKVStore
	dedup      APIDedup
	pluginID   string
	stats      APIStatsSink
}

type apiUserService interface {
	GetByUUID(uuid string) (*model.User, error)
}

type apiRoomService interface {
	List(page, pageSize int, roomType, domainUUID string) ([]model.Room, int64, error)
	Create(room *model.Room) error
}

type apiMuteService interface {
	MuteUser(muterUUID, userUUID string, duration int64, permanent bool, reason string) (*model.Mute, error)
}

type apiAuditor interface {
	Log(e audit.Entry)
}

// APIConfigStore 按 plugin_id 读取插件配置。
type APIConfigStore interface {
	GetConfig(pluginID string) (string, error)
}

// APIKVStore 按 plugin_id + key 读写插件 KV。
type APIKVStore interface {
	SaveKV(pluginID, key, valueJSON string) error
	LoadKV(pluginID, key string) (string, bool, error)
}

// APIDedup 以原子占位实现幂等。Claim 成功返回 false 表示调用方获得执行权；
// 返回 true 表示同 (plugin_id, request_id) 已有占位，应当直接返回成功。
type APIDedup interface {
	Claim(pluginID, requestID string) (bool, error)
	MarkDone(pluginID, requestID, status string)
	Release(pluginID, requestID string)
}

// APIDeps 是 NewAPIService 的类型化依赖集合；所有依赖必须在启动期就绪。
type APIDeps struct {
	UserSvc    apiUserService
	RoomSvc    apiRoomService
	MuteSvc    apiMuteService
	Auditor    apiAuditor
	ConfigRepo APIConfigStore
	KVRepo     APIKVStore
	Dedup      APIDedup
}

// NewAPIService 创建插件 API 服务；启动期校验依赖，缺失依赖立即返回错误。
func NewAPIService(deps APIDeps) (*APIService, error) {
	if deps.UserSvc == nil {
		return nil, fmt.Errorf("api service: user service is nil")
	}
	if deps.RoomSvc == nil {
		return nil, fmt.Errorf("api service: room service is nil")
	}
	if deps.MuteSvc == nil {
		return nil, fmt.Errorf("api service: mute service is nil")
	}
	if deps.Auditor == nil {
		return nil, fmt.Errorf("api service: auditor is nil")
	}
	if deps.ConfigRepo == nil {
		return nil, fmt.Errorf("api service: config store is nil")
	}
	if deps.KVRepo == nil {
		return nil, fmt.Errorf("api service: kv store is nil")
	}
	if deps.Dedup == nil {
		return nil, fmt.Errorf("api service: dedup is nil")
	}
	return &APIService{
		userSvc:    deps.UserSvc,
		roomSvc:    deps.RoomSvc,
		muteSvc:    deps.MuteSvc,
		auditor:    deps.Auditor,
		configRepo: deps.ConfigRepo,
		kvRepo:     deps.KVRepo,
		dedup:      deps.Dedup,
		pluginID:   pluginAuditActorName,
	}, nil
}

// WithPluginID 设置本 API 服务对外声明的插件 ID，用于写请求幂等键。
func (s *APIService) WithStats(sink APIStatsSink) *APIService {
	s.stats = sink
	return s
}

func (s *APIService) WithPluginID(id string) *APIService {
	if id != "" {
		s.pluginID = id
	}
	return s
}

func (s *APIService) countResult(success bool) {
	if s.stats != nil {
		s.stats.RecordAPI(success)
	}
}

func grpcErr(code codes.Code, err error) error {
	if err == nil {
		return nil
	}
	return status.Error(code, err.Error())
}

func (s *APIService) GetUser(_ context.Context, req *gen.GetUserRequest) (_ *gen.GetUserResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil || req.UserUuid == "" {
		return nil, status.Error(codes.InvalidArgument, "user_uuid is required")
	}
	if s.userSvc == nil {
		return nil, status.Error(codes.Unavailable, "user service not initialized")
	}
	user, err := s.userSvc.GetByUUID(req.UserUuid)
	if err != nil {
		return nil, grpcErr(codes.NotFound, err)
	}
	if user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return &gen.GetUserResponse{UserUuid: user.UUID, Username: user.Name, DisplayName: user.DisplayName}, nil
}

func (s *APIService) ListRooms(_ context.Context, req *gen.ListRoomsRequest) (_ *gen.ListRoomsResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.roomSvc == nil {
		return nil, status.Error(codes.Unavailable, "room service not initialized")
	}
	rooms, _, err := s.roomSvc.List(1, 100, "", req.DomainUuid)
	if err != nil {
		return nil, grpcErr(codes.Internal, err)
	}
	out := make([]*gen.Room, 0, len(rooms))
	for i := range rooms {
		out = append(out, &gen.Room{RoomUuid: rooms[i].UUID, Name: rooms[i].Name})
	}
	return &gen.ListRoomsResponse{Rooms: out}, nil
}

func (s *APIService) claim(pluginID, requestID string) (bool, error) {
	if s.dedup == nil || requestID == "" {
		return false, nil
	}
	return s.dedup.Claim(pluginID, requestID)
}

func (s *APIService) auditWrite(action, targetType, targetID, detail string) {
	if s.auditor == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logger.WithComponent("plugin").Errorf("audit write panic: %v", r)
		}
	}()
	s.auditor.Log(audit.Entry{
		ActorKind:  "plugin",
		ActorUUID:  model.PluginSystemUserUUID,
		ActorName:  pluginAuditActorName,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     detail,
		Success:    true,
	})
}

func (s *APIService) MuteUser(_ context.Context, req *gen.MuteUserRequest) (_ *gen.MuteUserResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil || req.UserUuid == "" {
		return nil, status.Error(codes.InvalidArgument, "user_uuid is required")
	}
	if s.muteSvc == nil {
		return nil, status.Error(codes.Unavailable, "mute service not initialized")
	}
	duplicate, err := s.claim(s.pluginID, req.RequestId)
	if err != nil {
		return nil, grpcErr(codes.Internal, err)
	}
	if duplicate {
		return &gen.MuteUserResponse{Ok: true}, nil
	}
	if _, err := s.muteSvc.MuteUser(model.PluginSystemUserUUID, req.UserUuid, 0, true, req.Reason); err != nil {
		s.dedup.Release(s.pluginID, req.RequestId)
		return nil, grpcErr(codes.Internal, err)
	}
	s.dedup.MarkDone(s.pluginID, req.RequestId, dedupStatusSuccess)
	s.auditWrite(audit.ActionMuteUser, audit.TargetUser, req.UserUuid, fmt.Sprintf("room=%s reason=%q", req.RoomUuid, req.Reason))
	return &gen.MuteUserResponse{Ok: true}, nil
}

func (s *APIService) CreateRoom(_ context.Context, req *gen.CreateRoomRequest) (_ *gen.CreateRoomResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil || req.Name == "" || req.DomainUuid == "" {
		return nil, status.Error(codes.InvalidArgument, "name and domain_uuid are required")
	}
	if s.roomSvc == nil {
		return nil, status.Error(codes.Unavailable, "room service not initialized")
	}
	duplicate, err := s.claim(s.pluginID, req.RequestId)
	if err != nil {
		return nil, grpcErr(codes.Internal, err)
	}
	if duplicate {
		return &gen.CreateRoomResponse{}, nil
	}
	room := &model.Room{Name: req.Name, DomainUUID: req.DomainUuid, CreatedBy: model.PluginSystemUserUUID}
	if err := s.roomSvc.Create(room); err != nil {
		s.dedup.Release(s.pluginID, req.RequestId)
		return nil, grpcErr(codes.Internal, err)
	}
	s.dedup.MarkDone(s.pluginID, req.RequestId, dedupStatusSuccess)
	s.auditWrite(audit.ActionPluginWrite, audit.TargetRoom, room.UUID, fmt.Sprintf("name=%q reason=%q", req.Name, req.Reason))
	return &gen.CreateRoomResponse{RoomUuid: room.UUID}, nil
}

func (s *APIService) GetConfig(_ context.Context, req *gen.GetConfigRequest) (_ *gen.GetConfigResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil || req.PluginId == "" {
		return nil, status.Error(codes.InvalidArgument, "plugin_id is required")
	}
	if s.configRepo == nil {
		return &gen.GetConfigResponse{}, nil
	}
	cfg, err := s.configRepo.GetConfig(req.PluginId)
	if err != nil {
		return nil, grpcErr(codes.Internal, err)
	}
	return &gen.GetConfigResponse{ConfigJson: cfg}, nil
}

func (s *APIService) KVSave(_ context.Context, req *gen.KVSaveRequest) (_ *gen.KVSaveResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil || req.PluginId == "" || req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "plugin_id and key are required")
	}
	if s.kvRepo == nil {
		return nil, status.Error(codes.Unavailable, "kv store not initialized")
	}
	duplicate, err := s.claim(req.PluginId, req.RequestId)
	if err != nil {
		return nil, grpcErr(codes.Internal, err)
	}
	if duplicate {
		return &gen.KVSaveResponse{}, nil
	}
	if err := s.kvRepo.SaveKV(req.PluginId, req.Key, req.ValueJson); err != nil {
		s.dedup.Release(req.PluginId, req.RequestId)
		return nil, grpcErr(codes.Internal, err)
	}
	s.dedup.MarkDone(req.PluginId, req.RequestId, dedupStatusSuccess)
	s.auditWrite(audit.ActionPluginWrite, audit.TargetPlugin, req.PluginId, fmt.Sprintf("kv key=%q reason=%q", req.Key, req.Reason))
	return &gen.KVSaveResponse{}, nil
}

func (s *APIService) KVLoad(_ context.Context, req *gen.KVLoadRequest) (_ *gen.KVLoadResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil || req.PluginId == "" || req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "plugin_id and key are required")
	}
	if s.kvRepo == nil {
		return nil, status.Error(codes.Unavailable, "kv store not initialized")
	}
	value, found, err := s.kvRepo.LoadKV(req.PluginId, req.Key)
	if err != nil {
		return nil, grpcErr(codes.Internal, err)
	}
	return &gen.KVLoadResponse{ValueJson: value, Found: found}, nil
}

func (s *APIService) ReportError(_ context.Context, req *gen.ReportErrorRequest) (_ *gen.ReportErrorResponse, err error) {
	defer func() { s.countResult(err == nil) }()
	if req == nil || req.PluginId == "" {
		return nil, status.Error(codes.InvalidArgument, "plugin_id is required")
	}
	if req.RequestId != "" && s.dedup != nil {
		duplicate, err := s.claim(req.PluginId, req.RequestId)
		if err != nil {
			return nil, grpcErr(codes.Internal, err)
		}
		if duplicate {
			return &gen.ReportErrorResponse{}, nil
		}
	}
	logger.WithComponent("plugin:"+req.PluginId).Errorf("[plugin] error report msg=%q details=%s request_id=%s", req.Message, req.DetailsJson, req.RequestId)
	return &gen.ReportErrorResponse{}, nil
}

// PluginConfigStore 基于 GORM 的插件配置仓库。
type PluginConfigStore struct{ db *gorm.DB }

func NewPluginConfigStore(db *gorm.DB) *PluginConfigStore { return &PluginConfigStore{db: db} }

func (r *PluginConfigStore) GetConfig(pluginID string) (string, error) {
	var cfg model.PluginConfig
	err := r.db.Where("plugin_id = ?", pluginID).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return cfg.ConfigJSON, nil
}

// PluginKVStore 基于 GORM 的插件 KV 仓库。
type PluginKVStore struct{ db *gorm.DB }

func NewPluginKVStore(db *gorm.DB) *PluginKVStore { return &PluginKVStore{db: db} }

func (r *PluginKVStore) SaveKV(pluginID, key, valueJSON string) error {
	return r.db.Save(&model.PluginKV{PluginID: pluginID, Key: key, ValueJSON: valueJSON}).Error
}

func (r *PluginKVStore) LoadKV(pluginID, key string) (string, bool, error) {
	var kv model.PluginKV
	err := r.db.Where("plugin_id = ? AND key = ?", pluginID, key).First(&kv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return kv.ValueJSON, true, nil
}

// PluginDedup 基于 plugin_request_log 的幂等仓储，Claim 用唯一索引原子占位。
type PluginDedup struct{ db *gorm.DB }

func NewPluginDedup(db *gorm.DB) *PluginDedup { return &PluginDedup{db: db} }

func (r *PluginDedup) Claim(pluginID, requestID string) (bool, error) {
	if requestID == "" {
		return false, nil
	}
	row := &model.PluginRequestLog{
		PluginID: pluginID, RequestID: requestID,
		Status: dedupStatusPending, CreatedAt: time.Now(),
	}
	err := r.db.Create(row).Error
	if err == nil {
		return false, nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) || isUniqueViolation(err) {
		return true, nil
	}
	return false, err
}

func (r *PluginDedup) Release(pluginID, requestID string) {
	if requestID == "" {
		return
	}
	if err := r.db.Where("plugin_id = ? AND request_id = ?", pluginID, requestID).Delete(&model.PluginRequestLog{}).Error; err != nil {
		logger.WithComponent("plugin").Errorf("release plugin request claim: %v", err)
	}
}

func (r *PluginDedup) MarkDone(pluginID, requestID, status string) {
	if requestID == "" {
		return
	}
	if err := r.db.Model(&model.PluginRequestLog{}).
		Where("plugin_id = ? AND request_id = ?", pluginID, requestID).
		Update("status", status).Error; err != nil {
		logger.WithComponent("plugin").Errorf("update plugin request log status: %v", err)
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint") ||
		strings.Contains(msg, "Duplicate entry")
}

var _ gen.PluginAPIServiceServer = (*APIService)(nil)
