package repository

import (
	"GOSpeak/internal/model"

	"gorm.io/gorm"
)

type OAuthAccountRepository struct {
	db *gorm.DB
}

func NewOAuthAccountRepository(db *gorm.DB) *OAuthAccountRepository {
	return &OAuthAccountRepository{db: db}
}

func (r *OAuthAccountRepository) GetByProviderAndUID(provider, uid string) (*model.OAuthAccount, error) {
	var account model.OAuthAccount
	err := r.db.Where("provider = ? AND provider_uid = ?", provider, uid).First(&account).Error
	if err != nil {
		return nil, err
	}
	if err := decryptOAuthAccountSecrets(&account); err != nil {
		return nil, err
	}
	return &account, nil
}

// CreateWithUser 原子地创建用户并绑定 OAuth 账户，避免第二步失败留下孤儿用户。
func (r *OAuthAccountRepository) CreateWithUser(user *model.User, account *model.OAuthAccount) error {
	if err := encryptOAuthAccountSecrets(account); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		account.UserID = user.ID
		return tx.Create(account).Error
	})
}

func (r *OAuthAccountRepository) Update(account *model.OAuthAccount) error {
	if err := encryptOAuthAccountSecrets(account); err != nil {
		return err
	}
	return r.db.Save(account).Error
}

// Create 新建独立的 OAuth 绑定（用户已存在，用于「关联已有账号」）。
func (r *OAuthAccountRepository) Create(account *model.OAuthAccount) error {
	if err := encryptOAuthAccountSecrets(account); err != nil {
		return err
	}
	return r.db.Create(account).Error
}

// GetByUserID 列出用户已绑定的所有第三方账号（token 已解密，仅服务层内部使用）。
func (r *OAuthAccountRepository) GetByUserID(userID uint) ([]model.OAuthAccount, error) {
	var accounts []model.OAuthAccount
	if err := r.db.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		return nil, err
	}
	for i := range accounts {
		if err := decryptOAuthAccountSecrets(&accounts[i]); err != nil {
			return nil, err
		}
	}
	return accounts, nil
}

// Delete 解除指定用户在某 provider 上的绑定。
func (r *OAuthAccountRepository) Delete(userID uint, provider string) error {
	return r.db.Where("user_id = ? AND provider = ?", userID, provider).Delete(&model.OAuthAccount{}).Error
}
