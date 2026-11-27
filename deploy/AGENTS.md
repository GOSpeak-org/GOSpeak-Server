# deploy — 部署编排

`deploy/` 是部署的唯一入口。**权威文档是 [DEPLOY.md](DEPLOY.md)**，本文件只写约定。

## 文件

| 文件 | 用途 |
|------|------|
| `docker-compose.yml` | 精简档位（app + LiveKit，无 profiles，开箱即用） |
| `docker-compose.standard.yml` | 全档位编排（app/livekit/edge/postgres/rustfs/observability） |
| `.env.example` | 统一配置模板（生成 `deploy/.env`） |
| `init.sh` | 生成 `.env`（含随机密钥）、归一 `livekit.yaml`、一致性校验 |
| `install-binary.sh` | curl 直跑的单二进制安装脚本：下载当前平台最新的 GOSpeak + LiveKit 发行包、校验、生成配置与 systemd 单元 |
| `Caddyfile` | 内置边缘（profile `edge`）：自动 HTTPS |
| `livekit/*.yaml` | LiveKit 配置（`livekit.yaml` / `livekit.docker.yaml` / `livekit.local.yaml`） |
| `observability/` | prometheus / alertmanager / grafana / loki / promtail |

## 档位（`COMPOSE_PROFILES`）

`app`（默认）、`livekit`（默认）、`edge`（内置 Caddy）、`postgres`、`rustfs`、`observability`。改档位就是改 `.env` 里的 `COMPOSE_PROFILES`。

**`COMPOSE_PROFILES` 只对 `docker-compose.standard.yml` 生效。** `docker-compose.yml` 是精简版（无 profiles、无 Caddy），裸跑 `docker compose up -d` 会选中它，不会启用 edge/HTTPS。启动一律显式带 `-f`：

- 精简起步：`docker compose -f deploy/docker-compose.yml up -d`
- 全档位：`docker compose -f deploy/docker-compose.standard.yml up -d`

## 关键约定

- **`deploy/.env` 是唯一配置文件**，compose 插值与应用环境变量共用同一份；应用加载优先级见 `server/internal/config/config.go`。**不要提交 `.env`**。
- **只需要一个域名、一个上游**：全档位版 `edge` profile 的 Caddy 反代只面对 `gospeak:8998`（精简版无 Caddy，也不读 `COMPOSE_PROFILES`），`/api` `/ws` `/uploads` `/livekit` 全在同一上游；LiveKit 不需要独立域名/证书，媒体走 UDP 直连。
- **密钥不进 `livekit.yaml`**：由 `init.sh` 归一后只留在 `.env`。
- `init.sh --check` 只做一致性校验、不写文件，CI 会跑；改 `.env.example` / `livekit.yaml` / `Caddyfile` 任一处都要保证校验通过。
- 改编排要同步 `DEPLOY.md` 与 Docs 仓的 `deploy/*`。

## 常用

```bash
cd deploy && ./init.sh --domain voice.example.com   # 生成 .env（含 edge 档位）
cd ..
# 按档位二选一，必须显式带 -f（裸 up -d 只会选中精简版，不启用 edge/HTTPS）：
docker compose -f deploy/docker-compose.yml up -d              # 精简起步：app + livekit
docker compose -f deploy/docker-compose.standard.yml up -d     # 全档位：COMPOSE_PROFILES 生效，含 edge 自动 HTTPS
cd deploy && ./init.sh --check                                  # 只校验
```
