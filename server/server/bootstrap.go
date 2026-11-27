package server

import (
	"GOSpeak/internal/logger"
	"GOSpeak/internal/repository"
	"GOSpeak/internal/service"

	"gorm.io/gorm"
)

// seedControlPlane 执行启动写面初始化：种子、迁移回填。
// 必须在构造依赖策略表的 service（permSvc / domainSvc）之前调用，
// 否则其 Casbin enforcer 会加载到空策略。
func seedControlPlane(
	db *gorm.DB,
	roleRepo *repository.RoleRepository,
	userRepo *repository.UserRepository,
	permRepo *repository.PermissionRepository,
	sfuConfigSvc *service.SFUConfigService,
) error {
	seedRoles(roleRepo)
	adminUUID := seedAdminUser(userRepo)
	if adminUUID != "" {
		if err := repository.EnsureDefaultDomain(db, adminUUID); err != nil {
			logger.WithComponent("Seed").Warnf("同步默认语音域失败: %v", err)
		}
	}
	if err := repository.BackfillDomainRoleDefaults(db); err != nil {
		logger.WithComponent("Seed").Warnf("存量域系统角色回填失败: %v", err)
	}
	seedPermissions(permRepo)
	if err := sfuConfigSvc.SyncFromEnv(); err != nil {
		return err
	}
	return nil
}
