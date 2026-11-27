# server 模块

GOSpeak 服务端入口（Gin + 原生 WebSocket）。媒体层经 `sfu.Provider` 抽象接入 **LiveKit（唯一支持的后端）**。

## 目录

```
server/
├── main.go              # 入口，Swagger 注解
├── cmd/                 # cobra：root.go（server/version）、reset_password.go、gen-protocol/
├── server/              # gin.go（DI 装配）、bootstrap.go、seeds.go、ws_policy.go、server_jobs.go、server_shutdown.go、server_stats.go
├── internal/
│   ├── config/          # 环境变量解析、校验、归一化
│   ├── model/           # GORM 实体
│   ├── repository/      # 数据访问 + 迁移
│   ├── service/         # 业务逻辑
│   ├── handler/         # HTTP handler
│   ├── middleware/      # 鉴权 / 域校验 / 限流 / CORS
│   ├── router/          # 路由装配（routes/ 按模块拆分）
│   ├── policy/          # 传输无关的访问策略（HTTP 与 WS 共用）
│   ├── sfu/             # Provider 抽象 + capabilities + factory
│   │   └── providers/livekit/
│   ├── signal/          # WS 信令 Hub
│   ├── ws/              # WS 基础设施（upgrader/fanout/handler/ACK）
│   ├── bus/             # 内嵌 NATS 事件总线 + JetStream KV
│   ├── protocol/        # WS 事件单一事实源
│   ├── genprotocol/     # TS 代码生成器
│   ├── authstate/       # JWT 黑名单 / 密钥轮换
│   ├── jobs/            # 后台任务（聊天持久化、禁言到期）
│   ├── notify/          # 通知广播
│   ├── livekitproxy/    # LiveKit 反向代理
│   ├── metrics/         # 指标
│   ├── webui/           # 内嵌前端资源
│   ├── buildinfo/       # 版本信息
│   ├── permcode/        # 权限码
│   ├── audit/ logger/ storage/ pkg/
├── docs/                # Swagger 产物
└── ../web/              # React 前端
```

## 启动

```bash
go run main.go server          # 未指定 -e 时默认 dev
go run main.go server -e prod  # 生产
```

配置全部来自环境变量，由 `config.LoadEnvFiles()` 按优先级加载：进程环境变量（已设置则不覆盖）> `GOSPEAK_ENV_FILE` > 工作目录 `.env.<env>`/`.env` > 仓库根 `deploy/.env`（逐文件仅填空缺键）。**仓库不提交 `.env*` 文件**，样例见 `deploy/.env.example`；字段清单见 `internal/config/config.go`。

## 依赖装配

`main.go → cmd/ → server/gin.go`：按 repository → service → handler → router 顺序组装；`bootstrap.go` 负责启动期的域/角色种子与回填，`server_jobs.go` 启动后台任务，`server_shutdown.go` 收尾（含 bus cleanup）。

## 禁言语义

- 只维护**用户级禁言**：允许收听，禁止发布本地音轨；以 `user_uuid` 为唯一标识，禁止在新接口用 `user_id`（见根 `AGENTS.md` §4.5）。
- 不引入「房间级禁言/静音」作为产品语义；但信令层在禁言/解禁时会按 SFU 能力尽力做媒体层强制（`hard` / `degraded` / `soft`），并广播成员级 `member:muted` / `member:unmuted` 事件供订阅端静音。
- `静音` = 客户端本地对远端音轨静音；`禁言` = 服务端用户级发言限制。

## 关键约束

- `SFU_PROVIDER` 固定 `livekit`，其他值启动失败。
- HTTP 与 WS 两个通道共用 `internal/policy` 的封禁判决（`middleware.BanCheck` 与 `ws_policy.go`）。
- 改 `internal/protocol` 后必须 `pnpm gen:protocol`。
- 插件运行层的职责、协议单一事实源与「拉取即信任」安全约束见 `internal/plugin/AGENTS.md`；精确管理路由见 `internal/router/routes/plugin/routes.go`。
- `livekit/server-sdk-go` 停在 v2.19.0：v2.20.0 起依赖 LiveKit 自维护的 pion fork（`webrtc-pion -warp.3` / `ice -warp.4`），而 `replace` 只对被依赖模块本模块生效、不传递——直接升会在 `transport.go` 撞上 dtls v3/v4 类型不匹配。要跟进必须在 `go.mod` 里自行镜像那两条 `replace`，并接受 experimental 依赖进生产二进制（约 +385 KB）。
