package service

import (
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
)

const (
	maxPushTokenLen     = 2048
	maxPushTokenPlatform = 32
	defaultPushPlatform = "web"
)

type PushTokenService struct {
	repo *repository.PushTokenRepository
}

func NewPushTokenService(repo *repository.PushTokenRepository) *PushTokenService {
	return &PushTokenService{repo: repo}
}

// Register 持久化当前用户的推送 token；同一 token 改报时由 repo 更新归属与平台。
func (s *PushTokenService) Register(userUUID, token, platform string) *pkg.AppError {
	if userUUID == "" {
		return pkg.NewAppError(pkg.TOKEN_NOT_EXIST, "missing user identity in context")
	}
	if token == "" || len(token) > maxPushTokenLen {
		return pkg.NewAppError(pkg.INVALID_PARAMS, "invalid push token")
	}
	if platform == "" {
		platform = defaultPushPlatform
	}
	if len(platform) > maxPushTokenPlatform {
		return pkg.NewAppError(pkg.INVALID_PARAMS, "invalid push platform")
	}
	if err := s.repo.Upsert(userUUID, token, platform); err != nil {
		return pkg.NewAppErrorWithCause(pkg.INTERNAL_ERROR, err, "save push token failed")
	}
	return nil
}
