package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"GOSpeak/internal/audit"
	"GOSpeak/internal/authstate"
	"GOSpeak/internal/bus"
	"GOSpeak/internal/config"
	"GOSpeak/internal/handler"
	"GOSpeak/internal/jobs"
	"GOSpeak/internal/livekitproxy"
	"GOSpeak/internal/logger"
	"GOSpeak/internal/metrics"
	"GOSpeak/internal/middleware"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/plugin"
	"GOSpeak/internal/plugin/gen"
	"GOSpeak/internal/policy"
	"GOSpeak/internal/protocol"
	"GOSpeak/internal/repository"
	"GOSpeak/internal/router"
	"GOSpeak/internal/service"
	"GOSpeak/internal/sfu"
	"GOSpeak/internal/sfu/factory"
	"GOSpeak/internal/signal"
	"GOSpeak/internal/storage"
	"GOSpeak/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
)

type EnvEnum string

const (
	Dev  EnvEnum = "dev"
	Prod EnvEnum = "prod"
)

func StartGin(env EnvEnum) error {
	appEnv := string(env)
	if appEnv == "" {
		appEnv = string(Dev)
	}
	// 进程环境优先；文件仅填充未设置变量
	config.LoadEnvFiles(appEnv)
	if os.Getenv("APP_ENV") == "" {
		_ = os.Setenv("APP_ENV", appEnv)
	}

	cfg := config.MustLoad()
	authstate.Configure(cfg)

	// 统一日志初始化（先于其它组件，后续 log/fmt 可逐步迁移）
	if err := logger.Init(logger.OptionsFrom(cfg.LoggerOptions())); err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	logger.SetupGin()
	logger.WithComponent("Server").WithFields(logger.Fields{
		"env":      appEnv,
		"port":     cfg.ServerPort,
		"provider": cfg.SFUProvider,
	}).Info("logger ready")

	if env == Prod || cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	if cfg.GinMode != "" {
		gin.SetMode(cfg.GinMode)
	}

	db, err := repository.InitDB(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	storage.InitCipher(cfg.StorageEncryptKey, cfg.IsProduction())

	if env == Prod || cfg.IsProduction() {
		authstate.SetProductionMode()
	}

	roleRepo := repository.NewRoleRepository(db)

	userRepo := repository.NewUserRepository(db)
	userGroupRepo := repository.NewUserGroupRepository(db)
	roomRepo := repository.NewRoomRepository(db)
	oauthProviderRepo := repository.NewOAuthProviderRepository(db)
	oauthAccountRepo := repository.NewOAuthAccountRepository(db)
	emailConfigRepo := repository.NewEmailConfigRepository(db)
	emailVerificationRepo := repository.NewEmailVerificationCodeRepository(db)
	permRepo := repository.NewPermissionRepository(db)
	muteRepo := repository.NewMuteRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	conversationRepo := repository.NewConversationRepository(db)
	sfuConfigRepo := repository.NewSFUConfigRepository(db)
	storageConfigRepo := repository.NewStorageConfigRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	userDomainLayoutRepo := repository.NewUserDomainLayoutRepository(db)
	domainRoleRepo := repository.NewDomainRoleRepository(db)
	roomLayoutRepo := repository.NewRoomLayoutRepository(db)
	announcementRepo := repository.NewAnnouncementRepository(db)
	pushTokenRepo := repository.NewPushTokenRepository(db)
	sfuConfigSvc := service.NewSFUConfigService(sfuConfigRepo, cfg)
	if err := seedControlPlane(db, roleRepo, userRepo, permRepo, sfuConfigSvc); err != nil {
		return fmt.Errorf("seed control plane: %w", err)
	}
	inviteSvc := service.NewInviteService(repository.NewDomainInviteRepository(db), domainRepo, cfg.StorageEncryptKey, cfg.JWTKey)
	domainSvc, err := service.NewDomainService(domainRepo, domainRoleRepo, inviteSvc, repository.NewDomainCasbinAdapter(db))
	if err != nil {
		return fmt.Errorf("failed to init domain casbin: %w", err)
	}
	domainLayoutSvc := service.NewDomainLayoutService(userDomainLayoutRepo)
	joinRequestSvc := service.NewJoinRequestService(
		repository.NewDBTxRunner(db),
		repository.NewDomainJoinRequestRepository(db),
		domainRepo,
		userRepo,
	)
	joinRequestSvc.SetMemberAddedHook(domainSvc.OnMemberAdded)

	permSvc, err := service.NewPermissionService(permRepo, repository.NewCasbinAdapter(db))
	if err != nil {
		return fmt.Errorf("failed to init casbin: %w", err)
	}
	if err := permSvc.LoadCache(); err != nil {
		return fmt.Errorf("failed to load permission cache: %w", err)
	}

	// 角色改名/删除后连锁重载权限缓存与 Casbin 策略（角色名是两者共用的 key），
	// 由 RoleService 直接依赖 permSvc 实现，不再走回调注入。
	roleSvc := service.NewRoleService(roleRepo, permSvc)
	emailConfigSvc := service.NewEmailConfigService(emailConfigRepo, cfg)
	emailSvc := service.NewEmailService(emailConfigSvc.ResolveConfig)
	emailVerificationSvc := service.NewEmailVerificationService(emailVerificationRepo, userRepo, emailSvc, emailConfigSvc.ResolveConfig)
	authSvc := service.NewAuthService(userRepo, emailConfigSvc, emailVerificationSvc)
	service.SetBcryptCost(cfg.BcryptCost)
	pkg.AccessTokenTTL = cfg.JWTAccessTTLDuration()
	pkg.RefreshTokenTTL = cfg.JWTRefreshTTLDuration()

	storageSvc := service.NewStorageService(storageConfigRepo, cfg)
	userSvc := service.NewUserService(userRepo)
	userGroupSvc := service.NewUserGroupService(userGroupRepo)
	oauthSvc := service.NewOAuthService(oauthProviderRepo, oauthAccountRepo, userRepo, cfg.OAuthAutoCreateUser)
	roomSvc := service.NewRoomService(roomRepo)
	messageSvc := service.NewMessageService(messageRepo, roomRepo, domainSvc, userRepo)
	muteSvc := service.NewMuteService(muteRepo, userRepo)
	pushTokenSvc := service.NewPushTokenService(pushTokenRepo)
	var sfuProvider sfu.Provider = factory.NewDynamicProvider(sfuConfigSvc.ResolveConfig)

	r := gin.New()

	// 只信任配置的代理网段注入的 X-Forwarded-*，避免外部客户端伪造。
	// TRUSTED_PROXIES 为逗号分隔的 IP/CIDR 列表；为空时回退本机回环地址。
	trustedProxies := cfg.TrustedProxiesList()
	if len(trustedProxies) == 0 {
		trustedProxies = []string{"127.0.0.1", "::1"}
	}
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		logger.WithComponent("HTTP").Warnf("set trusted proxies failed: %v", err)
	}
	r.Use(middleware.RequestID(), logger.GinRecovery(), logger.GinLogger(), middleware.CORS(strings.Split(cfg.CORSOrigin, ",")), middleware.SecurityHeaders())

	wsFanout := ws.NewFanout()
	wsHandler := ws.NewHandlerRegistry()

	deliverer := bus.NewWSDeliverer(wsFanout)
	initCfg, err := eventBusInitConfig(cfg, deliverer)
	if err != nil {
		return err
	}
	eventBus, closeEventBus, err := bus.Init(initCfg)
	if err != nil {
		return fmt.Errorf("failed to init event bus: %w", err)
	}

	pluginLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to listen plugin api: %w", err)
	}
	grpcServer := grpc.NewServer()
	hookRegistry := plugin.NewHookRegistry()
	var pluginManager *plugin.Manager
	hookDispatcher := plugin.NewHookDispatcher(hookRegistry, func(pluginID string) {
		if pluginManager != nil {
			_ = pluginManager.Disable(context.Background(), pluginID)
		}
	})
	pages := plugin.NewPageProxy()
	bp := plugin.NewBrowserProxy(pages, middleware.CurrentUserUUID)

	permSvc.SetEventBus(eventBus)
	domainSvc.SetEventBus(eventBus)
	muteSvc.SetEventBus(eventBus)
	messageSvc.SetEventBus(eventBus)
	var natsConn *nats.Conn
	instanceID := eventBus.InstanceID()
	if nb, ok := eventBus.(*bus.NATSBus); ok {
		natsConn = nb.Conn()
	}

	// membership KV: 强制 NATS（内嵌或外部 URL，bus.Init 已保证可用）。
	store, backend, err := bus.ResolveMembershipStore(bus.ResolveMembershipConfig{
		Prefix: cfg.NATSSubjectPrefix,
		NATS:   natsConn,
	})
	if err != nil {
		return fmt.Errorf("failed to init membership store: %w", err)
	}
	logger.WithComponent("StateStore").Infof("ready backend=%s instance=%s", backend, instanceID)

	var streamResolver signal.StreamNameResolver
	if snr, ok := sfuProvider.(signal.StreamNameResolver); ok && sfuProvider.SupportsStream() {
		streamResolver = snr
	}
	conversationSvc := service.NewConversationService(conversationRepo, messageRepo)
	conversationSvc.SetUserRepo(userRepo)
	conversationSvc.SetEventBus(eventBus)
	signalHub := signal.NewHubWithOptions(roomSvc, muteSvc, userSvc, permSvc, signal.HubOptions{
		Fanout:                  wsFanout,
		EventBus:                eventBus,
		SFUProvider:             sfuProvider,
		StreamResolver:          streamResolver,
		MessageSender:           messageSvc,
		ConversationSender:      conversationSvc,
		DomainChecker:           domainSvc.IsMember,
		DomainPermissionChecker: domainSvc.HasDomainPermission,
		MembershipStore:         store,
		LayoutStore:             roomLayoutRepo,
		InstanceID:              instanceID,
		StateNotifier:           eventBus,
		DisconnectGrace:         signal.DefaultDisconnectGracePeriod,
	})
	signalHub.StartMembershipHeartbeat()
	signalHub.StartEmptyRoomReconciler(0)
	// 内部事件统一经 InternalDispatcher 查表分发；未注册事件交回 Hub 处理远端状态同步。
	dispatcher := bus.NewInternalDispatcher()
	dispatcher.Register(service.EventPermissionsInvalidated, permSvc.OnRemoteInvalidate)
	dispatcher.Register(service.EventDomainMembershipChanged, domainSvc.OnRemoteInvalidate)
	dispatcher.Register(service.EventDomainPolicyChanged, domainSvc.OnRemotePolicyChanged)
	dispatcher.Register(service.EventMuteChanged, muteSvc.OnRemoteInvalidate)
	dispatcher.Register(service.EventMuteExpired, func(payload interface{}) {
		if p, err := bus.DecodePayload[service.MuteExpiredPayload](payload); err == nil && p.UserUUID != "" {
			signalHub.BroadcastUnmute(p.UserUUID)
		}
	})
	dispatcher.Register(service.EventDomainMemberAdded, func(payload interface{}) {
		if p, err := bus.DecodePayload[service.DomainMemberAddedPayload](payload); err == nil && p.DomainUUID != "" && p.UserUUID != "" {
			domainSvc.OnMemberAdded(p.DomainUUID, p.UserUUID)
		}
	})
	dispatcher.Register(service.EventRoomListChanged, func(payload interface{}) {
		if p, err := bus.DecodePayload[service.RoomListChangedPayload](payload); err == nil {
			signalHub.BroadcastRoomList(p.DomainUUID)
		}
	})
	dispatcher.Register(protocol.EventMessageCreated, func(payload interface{}) {
		hookDispatcher.Dispatch(context.Background(), protocol.EventMessageCreated, payload)
	})
	dispatcher.Register(protocol.EventMemberJoined, func(payload interface{}) {
		hookDispatcher.Dispatch(context.Background(), protocol.EventMemberJoined, payload)
	})
	dispatcher.Register(protocol.EventMemberLeft, func(payload interface{}) {
		hookDispatcher.Dispatch(context.Background(), protocol.EventMemberLeft, payload)
	})
	if nb, ok := eventBus.(*bus.NATSBus); ok {
		nb.SetRemoteHook(func(event string, payload interface{}) {
			if !dispatcher.Dispatch(event, payload) {
				signalHub.HandleRemoteEvent(event, payload)
			}
		})
	}

	// JWT blacklist + signing key: 强制 NATS KV（bus.Init 保证内嵌或外部 NATS 可用）。
	if natsConn == nil {
		return fmt.Errorf("failed to init auth store: nats connection required")
	}
	authStore, err := bus.OpenAuthStore(bus.AuthStoreConfig{
		Prefix: cfg.NATSSubjectPrefix,
		NC:     natsConn,
	})
	if err != nil {
		return fmt.Errorf("failed to init auth store: %w", err)
	}
	authstate.SetBackend(authStore)
	logger.WithComponent("AuthStore").Infof("ready backend=%s", authStore.Backend())

	var jobQueue *bus.JobQueue
	if nb, ok := eventBus.(*bus.NATSBus); ok && nb.Conn() != nil {
		q, err := bus.OpenJobQueue(bus.JobQueueConfig{
			Prefix: cfg.NATSSubjectPrefix,
			NC:     nb.Conn(),
		})
		if err != nil {
			logger.WithComponent("JobQueue").Warnf("unavailable: %v", err)
		} else {
			jobQueue = q
		}
	}
	signalHub.SetupFanout(wsFanout, wsHandler)
	sfuSvc := service.NewSFUService(sfuProvider, signalHub, domainSvc.IsMember)
	resolveLiveKitSecret := func() string {
		resolved, err := sfuConfigSvc.ResolveConfig()
		if err != nil || resolved == nil {
			return cfg.LiveKitSecret
		}
		if resolved.LiveKitSecret != "" {
			return resolved.LiveKitSecret
		}
		return cfg.LiveKitSecret
	}
	resolveLiveKitKey := func() string {
		resolved, err := sfuConfigSvc.ResolveConfig()
		if err != nil || resolved == nil {
			return cfg.LiveKitKey
		}
		if resolved.LiveKitKey != "" {
			return resolved.LiveKitKey
		}
		return cfg.LiveKitKey
	}
	signalH := handler.NewSignalHandler(sfuSvc, jobQueue, resolveLiveKitSecret, resolveLiveKitKey)

	authCookieCfg := handler.NewAuthCookieConfig(cfg)
	authH := handler.NewAuthHandler(authSvc, authCookieCfg, permSvc)
	joinRequestSvc.SetEventBus(eventBus)
	inviteJoinSvc := service.NewInviteJoinService(
		repository.NewDBTxRunner(db), inviteSvc, joinRequestSvc,
		repository.NewDomainInviteRepository(db), domainRepo, userRepo,
		emailConfigSvc, emailVerificationSvc,
	)
	inviteJoinSvc.SetEventBus(eventBus)
	inviteJoinSvc.SetMemberAddedHook(domainSvc.OnMemberAdded)
	inviteH := handler.NewInviteHandler(inviteSvc, domainSvc, permSvc)
	inviteJoinH := handler.NewInviteJoinHandler(inviteJoinSvc, authCookieCfg)
	joinRequestH := handler.NewJoinRequestHandler(joinRequestSvc, domainSvc, permSvc)
	auditSvc := audit.NewService(db)
	emailH := handler.NewEmailVerificationHandler(emailVerificationSvc)
	emailConfigH := handler.NewEmailConfigHandler(emailConfigSvc)
	userH := handler.NewUserHandler(userSvc, permSvc, auditSvc)
	userGroupH := handler.NewUserGroupHandler(userGroupSvc, userSvc)
	oauthH := handler.NewOAuthHandler(oauthSvc, authCookieCfg)
	roleH := handler.NewRoleHandler(roleSvc)
	roomH := handler.NewRoomHandler(roomSvc, permSvc, domainSvc, eventBus, auditSvc)
	msgH := handler.NewMessageHandler(messageSvc, permSvc, roomSvc, domainSvc)
	permH := handler.NewPermissionHandler(permSvc)
	muteH := handler.NewMuteHandler(muteSvc, userSvc, signalHub, auditSvc)
	pushTokenH := handler.NewPushTokenHandler(pushTokenSvc)
	pluginRoot := "/app/plugins"
	if appEnv == "test" {
		if override := os.Getenv("GOSPEAK_TEST_PLUGIN_ROOT"); override != "" {
			pluginRoot = override
		}
	}
	pluginManager = plugin.NewManager(pluginRoot, func(_ context.Context, req *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		hookRegistry.Register(req.GetPluginId(), req.GetHookEndpoint(), req.GetHookTypes())
		for _, page := range req.GetPages() {
			if err := pages.Confirm(req.GetPluginId(), page.GetMode(), page.GetRoutePath(), page.GetEntryUrl()); err != nil {
				return nil, err
			}
		}
		return &gen.RegisterResponse{ApiEndpoint: pluginLis.Addr().String()}, nil
	})
	pluginManager.SetPageConfirmer(pages)
	pluginManager.SetRegistrationStore(repository.NewPluginRegistrationStore(db))
	apiSvc, err := plugin.NewAPIService(plugin.APIDeps{
		UserSvc:    userSvc,
		RoomSvc:    roomSvc,
		MuteSvc:    muteSvc,
		Auditor:    auditSvc,
		ConfigRepo: plugin.NewPluginConfigStore(db),
		KVRepo:     plugin.NewPluginKVStore(db),
		Dedup:      plugin.NewPluginDedup(db),
	})
	if err != nil {
		return fmt.Errorf("failed to init plugin api service: %w", err)
	}
	apiSvc.WithStats(pluginManager)
	gen.RegisterPluginAPIServiceServer(grpcServer, apiSvc)
	pluginSvc := service.NewPluginService(db, pluginManager)
	pluginH := handler.NewPluginHandler(pluginSvc)

	// 临时禁言到期扫描：ListActiveMutes 会清理过期记录并触发 onExpired（广播 + SFU 恢复）。
	// 之前的到期清理依赖查询路径惰性触发，无定时任务时到期后订阅端会残留静音状态。
	// stop 必须保存并交给优雅关闭：否则关闭时扫描器仍在跑，会继续广播/调 SFU。
	stopMuteExpiryScanner := jobs.StartMuteExpiryScanner(context.Background(), func() {
		if _, err := muteSvc.ListActiveMutes(); err != nil {
			logger.WithComponent("MuteExpiry").Warnf("scan expired mutes: %v", err)
		}
	}, time.Minute)
	sfuConfigH := handler.NewSFUConfigHandler(sfuConfigSvc, signalHub)
	storageH := handler.NewStorageHandler(storageSvc)
	middleware.Configure(middleware.Dependencies{
		PermissionChecker:      permSvc,
		TokenVersionChecker:    authSvc,
		DefaultPasswordChecker: authSvc.NeedsPasswordChange,
		DomainChecker:          domainSvc.IsMember,
		BlacklistChecker:       authstate.IsBlacklistedErr,
		AuthCookieName:         cfg.AccessCookieName,
	})
	// 改密/改角色成功后主动失效 token 版本缓存，消除 ≤1 TTL 的旧值窗口；
	// 同时清默认口令门禁缓存，避免改密后新会话被旧 needsChange 结果误拦。
	onIdentityChanged := func(userUUID string) {
		middleware.InvalidateTokenVersion(userUUID)
		middleware.ClearDefaultPasswordCache(userUUID)
	}
	authSvc.SetTokenVersionInvalidator(onIdentityChanged)
	userSvc.SetTokenVersionInvalidator(onIdentityChanged)

	startJobConsumers(jobQueue, eventBus.InstanceID(), signalHub, messageSvc, conversationSvc)

	domainH := handler.NewDomainHandler(domainSvc, permSvc, auditSvc,
		signalHub.KickUserFromDomain, signalHub.KickUserFromDomain,
		func(serverUUID string) { signalHub.OnDomainDelete(serverUUID) })
	domainLayoutH := handler.NewDomainLayoutHandler(domainLayoutSvc)
	roomLayoutH := handler.NewRoomLayoutHandler(service.NewRoomLayoutService(roomLayoutRepo), permSvc, domainSvc, signalHub.BroadcastRoomList)
	auditH := handler.NewAuditHandler(auditSvc)
	announcementSvc := service.NewAnnouncementService(announcementRepo, domainRepo)
	announcementSvc.SetEventBus(eventBus)
	announcementSvc.SetDomainPermissionChecker(domainSvc.HasDomainPermission)
	announcementH := handler.NewAnnouncementHandler(announcementSvc, permSvc)
	conversationH := handler.NewConversationHandler(conversationSvc)

	monitorH := handler.NewMonitorHandler(signalHub, cfg, newServerInfraStats(eventBus))
	monitorH.SetPluginStats(pluginStatsProvider{manager: pluginManager})
	systemUpdateH := handler.NewSystemUpdateHandler(service.NewSystemUpdateService())

	// Prometheus /metrics：业务指标快照 + HTTP 请求指标，可选用 METRICS_TOKEN 保护。
	metricsSrv := metrics.New(func() metrics.Snapshot { return toMetricsSnapshot(monitorH.Collect()) })
	r.Use(metricsSrv.Middleware())
	metricsHandler := metrics.RequireToken(metricsSrv.Handler(), cfg.MetricsToken)
	r.GET("/metrics", gin.WrapH(metricsHandler))

	// 启动签名密钥轮换检查
	authstate.StartKeyRotationLoop()

	// WS 升级路由：JWT 鉴权由 Upgrader 内部完成，BanCheck 等价策略
	// 由 installWSPolicy 注入（/ws 绕过 Gin，不能依赖中间件链）。
	wsUpgraderCfg := ws.UpgraderConfig{
		Fanout:         wsFanout,
		Handler:        wsHandler,
		AllowedOrigins: wsAllowedOrigins(cfg),
		AuthCookieName: cfg.AccessCookieName,
		OnConnect: func(c *ws.Client) {
			if err := signalHub.OnConnect(c); err != nil {
				logger.WithComponent("WS").Warnf("on connect failed: %v", err)
				c.Close()
			}
		},
		OnDisconnect: func(c *ws.Client) {
			signalHub.OnDisconnect(c)
		},
	}
	policyEngine := policy.NewEngine()
	installWSPolicy(policyEngine, wsHandler, &wsUpgraderCfg, signalHub)
	wsUpgrader := ws.NewUpgrader(wsUpgraderCfg)

	router.SetupRoutes(r, &router.Handlers{
		Config: cfg,
		ReadyCheck: func() error {
			if _, err := db.DB(); err != nil {
				return err
			}
			return nil
		},
		Auth:         authH,
		Invite:       inviteH,
		InviteJoin:   inviteJoinH,
		JoinRequest:  joinRequestH,
		User:         userH,
		UserGroup:    userGroupH,
		Signal:       signalH,
		OAuth:        oauthH,
		Role:         roleH,
		Room:         roomH,
		Permission:   permH,
		Mute:         muteH,
		Message:      msgH,
		SFUConfig:    sfuConfigH,
		Storage:      storageH,
		Email:        emailH,
		EmailConfig:  emailConfigH,
		Monitor:      monitorH,
		SystemUpdate: systemUpdateH,
		Domain:       domainH,
		DomainLayout: domainLayoutH,
		RoomLayout:   roomLayoutH,
		Conversation: conversationH,
		Audit:        auditH,
		Announcement: announcementH,
		PushToken:    pushTokenH,
		Plugin:       pluginH,
		PageProxy:    pages,
		BrowserProxy: bp,
	})

	port := cfg.ServerPort
	if port == "" {
		port = "8998"
	}

	scheme := "http"
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" {
		scheme = "https"
	}
	logger.WithComponent("Swagger").Infof("API 文档地址: %s://localhost:%s/swagger/index.html", scheme, port)

	// 同源 LiveKit 反代：浏览器侧 serverUrl 为 wss://<域名>/livekit 时，
	// 由应用把 /livekit/* 转发到 LIVEKIT_HOST（含 WebSocket 升级）。
	// 这样 LiveKit 不需要独立域名、证书或反代规则，反代层只面对应用一个上游。
	livekitProxy := livekitproxy.NewHandler(livekitproxy.Cached(func() string {
		resolved, err := sfuConfigSvc.ResolveConfig()
		if err == nil && resolved != nil && strings.TrimSpace(resolved.LiveKitHost) != "" {
			return resolved.LiveKitHost
		}
		return cfg.LiveKitHost
	}, 2*time.Second))

	// /ws 必须绕过 Gin：nhooyr/websocket 的 hijack 与 gin.ResponseWriter 不兼容。
	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if wsUpgrader != nil && req.URL.Path == "/ws" {
			wsUpgrader.ServeHTTP(w, req)
			return
		}
		if req.URL.Path == livekitproxy.PathPrefix || strings.HasPrefix(req.URL.Path, livekitproxy.PathPrefix+"/") {
			livekitProxy.ServeHTTP(w, req)
			return
		}
		r.ServeHTTP(w, req)
	})
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: rootHandler,
	}

	go func() {
		if err := grpcServer.Serve(pluginLis); err != nil {
			logger.WithComponent("PluginAPI").Errorf("serve error: %v", err)
		}
	}()
	if err := pluginManager.Start(context.Background()); err != nil {
		panic(err)
	}

	// 优雅关闭：监听系统信号，先关 WebSocket 连接再关 HTTP
	go runGracefulShutdown(shutdownDeps{
		srv:                   srv,
		signalHub:             signalHub,
		wsUpgrader:            wsUpgrader,
		auditSvc:              auditSvc,
		closeEventBus:         closeEventBus,
		stopMuteExpiryScanner: stopMuteExpiryScanner,
		stopPluginManager: func(ctx context.Context) error {
			_ = hookDispatcher.Close()
			return pluginManager.Stop(ctx)
		},
	})

	if err := serveHTTP(srv, cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
		logger.WithComponent("HTTP").Fatalf("listen error: %v", err)
	}
	return nil
}

// eventBusInitConfig 把进程配置映射为 bus.Init 参数。StoreDir 必须来自
// cfg.NATStoreDirPath()：漏传时内嵌 NATS 会退回 MkdirTemp 临时目录，
// JWT 签名密钥/黑名单/refresh-family 随重启丢失。单独抽出便于回归测试。
func eventBusInitConfig(cfg *config.Config, deliverer bus.Deliverer) (bus.InitConfig, error) {
	timeout, err := time.ParseDuration(cfg.NATSConnectTimeout)
	if err != nil || timeout <= 0 {
		timeout = 2 * time.Second
	}

	embeddedPort := 0
	if cfg.NATSEmbeddedPort != "" {
		p, err := strconv.Atoi(cfg.NATSEmbeddedPort)
		if err != nil {
			return bus.InitConfig{}, fmt.Errorf("invalid NATS_EMBEDDED_PORT %q: %w", cfg.NATSEmbeddedPort, err)
		}
		embeddedPort = p
	}

	return bus.InitConfig{
		Prefix:         cfg.NATSSubjectPrefix,
		Name:           cfg.NATSName,
		ConnectTimeout: timeout,
		EmbeddedPort:   embeddedPort,
		StoreDir:       cfg.NATStoreDirPath(),
		Deliverer:      deliverer,
		WALPath:        cfg.NATSWALPath(),
	}, nil
}

// serveHTTP 按 TLS 配置选择监听方式：两者均未配置时保持明文 HTTP/1.1；
// 配置证书后启用 HTTPS，Go 标准库会自动提供 h2 + HTTP/1.1 ALPN fallback。
func serveHTTP(srv *http.Server, tlsCert, tlsKey string) error {
	if tlsCert == "" && tlsKey == "" {
		return srv.ListenAndServe()
	}
	if tlsCert == "" || tlsKey == "" {
		return fmt.Errorf("TLS_CERT and TLS_KEY must be configured together")
	}
	return srv.ListenAndServeTLS(tlsCert, tlsKey)
}
