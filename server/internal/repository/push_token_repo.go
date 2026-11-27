package repository

import (
	"GOSpeak/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PushTokenRepository struct {
	db *gorm.DB
}

func NewPushTokenRepository(db *gorm.DB) *PushTokenRepository {
	return &PushTokenRepository{db: db}
}

// Upsert 按 token 幂等写入；同一 token 被另一账号/平台上报时更新归属。
func (r *PushTokenRepository) Upsert(userUUID, token, platform string) error {
	row := model.PushToken{UserUUID: userUUID, Token: token, Platform: platform}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_uuid", "platform", "updated_at"}),
	}).Create(&row).Error
}

func (r *PushTokenRepository) GetByToken(token string) (*model.PushToken, error) {
	ctx, cancel := repoTimeoutCtx()
	defer cancel()
	var row model.PushToken
	err := r.db.WithContext(ctx).Where("token = ?", token).First(&row).Error
	return &row, err
}
