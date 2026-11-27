package service

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"GOSpeak/internal/bus"
	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
)

const muteCacheTTL = 5 * time.Second
const maxMuteReasonRunes = 500

type muteCacheEntry struct {
	muted    bool
	mute     *model.Mute
	identity string
	expires  time.Time
}

// MuteChecker 供 middleware 查询禁言状态的接口
type MuteChecker interface {
	IsMuted(userUUID string) (bool, *model.Mute, error)
}

type MuteService struct {
	muteRepo *repository.MuteRepository
	userRepo *repository.UserRepository
	bus      cacheBus
	expiryMu sync.Mutex
	expiring map[string]bool

	byIdentity map[string]muteCacheEntry
	byUserUUID map[string]muteCacheEntry
	cacheMu    sync.RWMutex
}

func NewMuteService(muteRepo *repository.MuteRepository, userRepo *repository.UserRepository) *MuteService {
	return &MuteService{
		muteRepo:   muteRepo,
		userRepo:   userRepo,
		byIdentity: make(map[string]muteCacheEntry),
		byUserUUID: make(map[string]muteCacheEntry),
		expiring:   make(map[string]bool),
	}
}

func (s *MuteService) SetEventBus(b cacheBus) { s.bus = b }

func (s *MuteService) OnRemoteInvalidate(payload interface{}) {
	p, err := bus.DecodePayload[MuteChangedPayload](payload)
	if err != nil {
		log.Printf("[Mute] invalid mute-changed payload: %T (%v)", payload, err)
		return
	}
	s.invalidateMute(p.UserUUID, p.Identity, false)
}

func (s *MuteService) invalidateMute(userUUID string, identity string, publish bool) {
	s.cacheMu.Lock()
	if userUUID != "" {
		if entry, ok := s.byUserUUID[userUUID]; ok && identity == "" {
			identity = entry.identity
		}
		delete(s.byUserUUID, userUUID)
	}
	if identity != "" {
		delete(s.byIdentity, identity)
	}
	s.cacheMu.Unlock()
	if publish && s.bus != nil {
		payload := MuteChangedPayload{UserUUID: userUUID, Identity: identity}
		if err := s.bus.PublishInternal(context.Background(), EventMuteChanged, payload); err != nil {
			log.Printf("[Mute] publish invalidate: %v", err)
		}
	}
}

// IsMuted 检查用户是否被禁言
func (s *MuteService) IsMuted(userUUID string) (bool, *model.Mute, error) {
	s.cacheMu.RLock()
	if entry, ok := s.byUserUUID[userUUID]; ok && time.Now().Before(entry.expires) {
		muted, mute := entry.muted, entry.mute
		s.cacheMu.RUnlock()
		return muted, mute, nil
	}
	s.cacheMu.RUnlock()

	mute, err := s.muteRepo.GetByUserUUID(userUUID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.cacheMute(userUUID, "", false, nil)
			return false, nil, nil
		}
		return false, nil, pkg.NewAppError(pkg.INTERNAL_ERROR)
	}
	if mute.Permanent {
		s.cacheMute(userUUID, "", true, mute)
		return true, mute, nil
	}
	if mute.ExpiresAt != nil && mute.ExpiresAt.Before(time.Now()) {
		if delErr := s.muteRepo.DeleteByUserUUID(userUUID); delErr != nil && !errors.Is(delErr, gorm.ErrRecordNotFound) {
			return false, nil, pkg.NewAppError(pkg.INTERNAL_ERROR)
		}
		s.invalidateMute(userUUID, "", true)
		s.notifyExpired(userUUID)
		s.cacheMute(userUUID, "", false, nil)
		return false, nil, nil
	}
	s.cacheMute(userUUID, "", true, mute)
	return true, mute, nil
}

// IsMutedByIdentity 通过 identity（用户名）检查禁言状态
func (s *MuteService) IsMutedByIdentity(identity string) (bool, *model.Mute, error) {
	s.cacheMu.RLock()
	if entry, ok := s.byIdentity[identity]; ok && time.Now().Before(entry.expires) {
		if !entry.muted || entry.mute != nil {
			muted, mute := entry.muted, entry.mute
			s.cacheMu.RUnlock()
			return muted, mute, nil
		}
	}
	s.cacheMu.RUnlock()
	user, err := s.userRepo.GetByName(identity)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.cacheMute("", identity, false, nil)
			return false, nil, nil
		}
		return false, nil, pkg.NewAppError(pkg.INTERNAL_ERROR)
	}
	muted, mute, err := s.IsMuted(user.UUID)
	if err != nil {
		return false, nil, err
	}
	s.cacheMute(user.UUID, identity, muted, mute)
	return muted, mute, nil
}

// IsMutedBatch 按用户 UUID 批量检查生效禁言，返回 uuid -> muted 映射。
func (s *MuteService) IsMutedBatch(userUUIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(userUUIDs))
	var missing []string
	now := time.Now()
	s.cacheMu.RLock()
	for _, u := range userUUIDs {
		if entry, ok := s.byUserUUID[u]; ok && now.Before(entry.expires) {
			result[u] = entry.muted
			continue
		}
		missing = append(missing, u)
	}
	s.cacheMu.RUnlock()
	if len(missing) == 0 {
		return result, nil
	}
	muted, err := s.muteRepo.IsMutedBatch(missing)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	s.cacheMu.Lock()
	for _, u := range missing {
		isMuted := muted[u]
		result[u] = isMuted
		s.byUserUUID[u] = muteCacheEntry{muted: isMuted, expires: time.Now().Add(muteCacheTTL)}
	}
	s.cacheMu.Unlock()
	return result, nil
}

func (s *MuteService) cacheMute(userUUID string, identity string, muted bool, mute *model.Mute) {
	entry := muteCacheEntry{muted: muted, mute: mute, identity: identity, expires: time.Now().Add(muteCacheTTL)}
	s.cacheMu.Lock()
	if userUUID != "" {
		s.byUserUUID[userUUID] = entry
	}
	if identity != "" {
		s.byIdentity[identity] = entry
	}
	s.cacheMu.Unlock()
}

// IsMutedByUUID 通过 UUID 检查禁言状态（供 middleware 使用）
func (s *MuteService) IsMutedByUUID(uuid string) (bool, error) {
	muted, _, err := s.IsMuted(uuid)
	return muted, err
}

// MuteUser 禁言用户（按 user_uuid）
func (s *MuteService) MuteUser(muterUUID string, userUUID string, duration int64, permanent bool, reason string) (*model.Mute, error) {
	if !permanent && duration <= 0 {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "duration is required for temporary mute")
	}
	if userUUID == "" {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "user_uuid is required")
	}
	if utf8.RuneCountInString(reason) > maxMuteReasonRunes {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "reason is too long")
	}
	target, err := s.userRepo.GetByUUID(userUUID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, pkg.NewAppError(pkg.USER_NOT_FOUND)
		}
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR)
	}
	if target.Role == model.RoleAdmin {
		muter, merr := s.userRepo.GetByUUID(muterUUID)
		if merr != nil || muter == nil || muter.Role != "admin" || muter.IsBot {
			return nil, pkg.NewAppError(pkg.FORBIDDEN, "cannot mute admin")
		}
	}
	mute := &model.Mute{
		UserUUID:  userUUID,
		MuterUUID: muterUUID,
		Duration:  duration,
		Permanent: permanent,
		Reason:    reason,
	}
	if !permanent && duration > 0 {
		expiresAt := time.Now().Add(time.Duration(duration) * time.Second)
		mute.ExpiresAt = &expiresAt
	}
	if err := s.muteRepo.Upsert(mute); err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, "failed to create mute record")
	}
	s.invalidateMute(userUUID, target.Name, true)
	return mute, nil
}

// UnmuteUser 取消禁言
func (s *MuteService) UnmuteUser(userUUID string) error {
	if err := s.muteRepo.DeleteByUserUUID(userUUID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return pkg.NewAppError(pkg.NOT_FOUND, "mute record not found")
		}
		return pkg.NewAppError(pkg.INTERNAL_ERROR)
	}
	identity := ""
	s.cacheMu.RLock()
	if entry, ok := s.byUserUUID[userUUID]; ok {
		identity = entry.identity
	}
	s.cacheMu.RUnlock()
	if identity == "" {
		if user, userErr := s.userRepo.GetByUUID(userUUID); userErr == nil && user != nil {
			identity = user.Name
		}
	}
	s.invalidateMute(userUUID, identity, true)
	return nil
}

// notifyExpired 发布禁言到期事件（订阅方执行完整 unmute），并防止并发重复触发。
func (s *MuteService) notifyExpired(userUUID string) {
	if userUUID == "" || s.bus == nil {
		return
	}
	s.expiryMu.Lock()
	if s.expiring[userUUID] {
		s.expiryMu.Unlock()
		return
	}
	s.expiring[userUUID] = true
	s.expiryMu.Unlock()
	defer func() {
		s.expiryMu.Lock()
		delete(s.expiring, userUUID)
		s.expiryMu.Unlock()
	}()
	payload := MuteExpiredPayload{UserUUID: userUUID}
	if err := s.bus.PublishInternal(context.Background(), EventMuteExpired, payload); err != nil {
		log.Printf("[Mute] publish expiry: %v", err)
	}
}

// GetMuteStatus 查询禁言状态
func (s *MuteService) GetMuteStatus(userUUID string) (*model.Mute, error) {
	muted, mute, err := s.IsMuted(userUUID)
	if err != nil {
		return nil, err
	}
	if !muted {
		return nil, nil
	}
	return mute, nil
}

// ListActiveMutes 获取所有生效禁言
func (s *MuteService) ListActiveMutes() ([]model.Mute, error) {
	expired, err := s.muteRepo.ListExpired()
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR)
	}
	if err := s.muteRepo.DeleteExpired(); err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR)
	}
	for _, mute := range expired {
		s.notifyExpired(mute.UserUUID)
	}
	return s.muteRepo.ListActive()
}
