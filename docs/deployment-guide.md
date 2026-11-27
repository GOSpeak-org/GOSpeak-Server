# GOSpeak 部署/运维手册

> 更新: 2026-11-24（随代码版本，当前 `VERSION` = 1.3.0-rc.1）
>
> **统一部署入口已迁到 [`deploy/DEPLOY.md`](../deploy/DEPLOY.md)。** 编排文件二选一：精简起步 `deploy/docker-compose.yml`、全档位 `deploy/docker-compose.standard.yml`；启动必须显式带 `-f`，不要用裸 `docker compose up -d`（会选中精简版，`COMPOSE_PROFILES` 不生效）。
> 本文保留概念说明；一键命令与公网清单以 `deploy/DEPLOY.md` 为准。

GOSpeak 实时音视频平台的部署文档。覆盖本地开发、Docker 部署、生产上线全流程。

---

## 1. 环境要求

| 组件 | 版本 | 用途 |
|------|------|------|
| Go | 1.27.4+ | 后端编译 (需要 CGO, 因 SQLite) |
| Node.js | 22+ | 前端构建 (Vite) |
| pnpm | 10+ | monorepo 包管理 |
| Docker | 24+ | 容器化部署 |
| Docker Compose | v2 | 多服务编排 |

可选:
- PostgreSQL / MySQL — 生产数据库（默认 SQLite）

> NATS 事件总线固定为进程内嵌运行（含 JetStream），随 GOSpeak 启动，无需单独部署。

---

## 2. 快速开始（本地开发）

### 2.1 安装依赖

```bash
pnpm install
```

### 2.2 启动依赖服务

本地开发用精简版起步（只有应用 + LiveKit，无 profiles）：

```bash
docker compose -f deploy/docker-compose.yml up -d
```

需要 PostgreSQL / RustFS / 可观测栈 / `edge` 自动 HTTPS 时，改用全档位版并显式指定它——`COMPOSE_PROFILES` 只对 `docker-compose.standard.yml` 生效：

```bash
docker compose -f deploy/docker-compose.standard.yml up -d   # 档位见 deploy/.env 的 COMPOSE_PROFILES
```

### 2.3 启动后端

```bash
pnpm dev:server
# 等价: cd server && air
# air 热重载；未指定 -e 时默认 dev；LoadEnvFiles 依次读 .env.dev/.env（工作目录）与 deploy/.env
```

### 2.4 启动前端

```bash
pnpm dev:web
# 等价: cd client/web && vite --force
```

或一键同时启:
```bash
pnpm start:dev
```

访问: http://localhost:3000 (Vite)

### 2.5 默认管理员账号

| 字段 | 值 |
|------|-----|
| 用户名 | `admin` |
| 默认密码 | `admin123` |

首次启动自动 seed；登录后若 `need_change_password=true`，请立即修改默认密码。


---

## 3. 环境变量说明

配置完全通过环境变量驱动，唯一配置文件是仓库根的 **`deploy/.env`**（由 `deploy/.env.example` 复制或 `deploy/init.sh` 生成）。

启动时 `config.LoadEnvFiles()` 按顺序查找：`GOSPEAK_ENV_FILE` 显式指定的文件 → 当前工作目录的 `.env.<env>` / `.env` → 逐级向上找到的 `deploy/.env`。已存在的进程环境变量不会被覆盖，因此容器内以 `env_file` 注入的值优先。

### 3.1 数据库

| 变量 | 说明 | 示例 |
|------|------|------|
| `DB_TYPE` | `PostgreSQL` / `MySQL` / `SQLite` | `SQLite` |
| `DB_HOST` | DB 主机 (非 SQLite) | `localhost` |
| `DB_PORT` | DB 端口 | `5432` |
| `DB_USER` | DB 用户 | `gospeak` |
| `DB_PASSWORD` | DB 密码 | `***` |
| `DB_PATH` | SQLite 文件路径 | `db/app.db` |

### 3.2 NATS（内嵌事件总线）

NATS 总线固定为进程内嵌运行（含 JetStream），承担内部事件、KV 状态（JWT 黑名单/密钥轮换/禁言规则）与异步任务队列，无需也无法配置外部 NATS（不支持 `NATS_URL`）。

| 变量 | 说明 | 示例 |
|------|------|------|
| `NATS_SUBJECT_PREFIX` | NATS subject / KV bucket 前缀 | `gospeak` |
| `NATS_NAME` | 实例名（空 = 自动生成） | `node-1` |
| `NATS_CONNECT_TIMEOUT` | 连接超时 | `2s` |
| `NATS_EMBEDDED_PORT` | 内嵌 NATS 端口（空 = 随机端口） | `4222` |
| `NATS_WAL_PATH` | 断线事件磁盘 WAL 路径（空 = 禁用） | `/data/nats-wal` |
| `JWT_KEY_TTL` | JWT 密钥轮换周期，经 NATS KV 管理 | `24h` |

### 3.3 SFU（LiveKit）

`SFU_PROVIDER` 固定为 `livekit`（唯一支持的音视频后端）。

| 变量 | 说明 |
|------|------|
| `SFU_PROVIDER` | `livekit`（固定值） |
| `LIVEKIT_HOST` | 后端访问 LiveKit 的地址（admin API）。容器部署填 `ws://livekit:7880`，Cloud 填 `wss://<id>.livekit.cloud` |
| `LIVEKIT_PUBLIC_URL` | **默认留空**。留空时按请求 Host 自动推导 `wss://<域名>/livekit`（应用内置同源反代，LiveKit 无需独立域名/证书）；仅独立域名或 Cloud 时显式填 |
| `LIVEKIT_KEY` | API Key |
| `LIVEKIT_SECRET` | API Secret |

### 3.4 对象存储

| 变量 | 说明 | 示例 |
|------|------|------|
| `STORAGE_TYPE` | `local` / `s3` | `local` |
| `STORAGE_ENDPOINT` | S3 端点 (RustFS/R2) | `http://localhost:9000` |
| `STORAGE_BUCKET` | 桶名 | `gospeak` |
| `STORAGE_REGION` | 区域 | `us-east-1` |
| `STORAGE_ACCESS_KEY` | 访问密钥 | `rustfsadmin` |
| `STORAGE_SECRET_KEY` | 密钥 | `rustfsadmin` |
| `STORAGE_PATH_PREFIX` | S3 对象键前缀（本地存储忽略） | `uploads/` |
| `STORAGE_LOCAL_DIR` | 本地存储落盘目录，与 `/uploads` 静态服务同源 | `uploads` |
| `STORAGE_PUBLIC_BASE_URL` | 公开域名 (CDN) | `https://cdn.example.com` |
| `STORAGE_ENCRYPT_KEY` | 凭证加密密钥，64 位 hex（生产必设） | `a1b2...` |

⚠️ `STORAGE_ENCRYPT_KEY` 生产环境必须设置，否则凭证明文存储。

---

## 4. Docker 部署

### 4.1 依赖服务 — 渐进式数据库三档

首次部署推荐 `cd deploy && ./init.sh` 自动生成配置。`deploy/docker-compose.standard.yml` 用 Compose `profiles` 提供档位组合，档位写在 `deploy/.env` 的 `COMPOSE_PROFILES`，启动命令必须显式指向该文件：`docker compose -f deploy/docker-compose.standard.yml up -d`（裸 `docker compose up -d` 会选中精简版，档位不生效）：

| 档 | `deploy/.env` 关键项 |
|----|----------------------|
| A 单 SQLite | `DB_TYPE=SQLite`（默认） |
| B 单 PostgreSQL | `COMPOSE_PROFILES` 追加 `postgres`；`DB_TYPE=PostgreSQL`, `DB_HOST=postgres`, `DB_PASSWORD=...` |
| C PostgreSQL + 密钥轮换 | 同 B 档；`JWT_KEY_TTL` 默认 `24h`（三档均启用轮换，编排与 B 档相同） |

- **A 档**：零外部 DB，SQLite 内嵌，最小起步
- **B 档**：单一 PostgreSQL，中等规模
- **C 档**：在 B 档基础上开启密钥轮换；JWT 轮换与 Token 黑名单在所有档位均由内嵌 NATS 提供，并非 C 档独有。

NATS 为进程内嵌运行，所有档位均有，无需外部部署，也不支持多副本共享外部 NATS。

### 4.1.1 SFU 配置

与 DB 档正交。`./init.sh` 一步完成档位与密钥配置，或直接改 `deploy/.env`（`COMPOSE_PROFILES` 只对 `docker-compose.standard.yml` 生效）：含 `app,livekit`，`SFU_PROVIDER=livekit`。

LiveKit 不需要独立域名或证书：应用内置同源反代 `/livekit/*` → 内部 LiveKit `7880`（WebSocket 升级自动处理），浏览器侧 `serverUrl` 由请求 Host 推导为 `wss://<域名>/livekit`。因此 compose 不向公网发布 7880 / 7881：7880 只绑宿主机回环（`127.0.0.1:7880`）供本机调试，公网只保留 `7882` tcp/udp（媒体）与 `3478/udp`（TURN）。

### 4.2 构建应用镜像

```bash
docker build -t gospeak:latest .
```

三阶段构建（见根 `Dockerfile`）:
1. `go-builder` — Go 后端 (CGO=1, 静态链接)
2. `web-builder` — 前端 SPA (pnpm + Vite)
3. 产出 Alpine 镜像，端口 8998，入口 `/app/gospeak`

### 4.3 运行应用

```bash
docker run -d \
  --name gospeak \
  --env-file deploy/.env \
  -p 8998:8998 \
  gospeak:latest
```

应用读 `deploy/.env`；未指定 `-e` 时默认 `dev`，生产需显式 `server -e prod`。

### 4.4 单二进制（不装 Docker）

与上面 4.1–4.3 的容器方案二选一。目标机器上没有 Docker 时，用 `deploy/install-binary.sh` 一条命令完成安装：

```bash
curl -fsSL https://raw.githubusercontent.com/GOSpeak-org/GOSpeak-Server/release/deploy/install-binary.sh | bash
```

- 自动识别系统与架构，从 Releases 取当前平台最新的 GOSpeak 与 LiveKit 发行包
- 校验 `SHA256SUMS-gospeak-server-*.txt` 与 LiveKit `checksums.txt`
- 安装到 `/opt/gospeak`，生成 `.env`（随机密钥，600）、`livekit.yaml`、systemd 单元
- 重复执行即升级，**不覆盖已有 `.env` / `livekit.yaml`**

常用参数：`--dry-run`（只解析不下载）、`--dir`、`--user`、`--version`、`--without-livekit`、`--without-systemd`。

该形态无内置 Caddy，HTTPS 需自备反代指向 `8998` 或配置 `TLS_CERT` / `TLS_KEY`；局域网自用可直接访问 `http://<host>:8998`（但语音需要 HTTPS 或 localhost）。

详见 [docs/deploy/binary.md](../../docs/deploy/binary.md)。

---

## 5. 生产部署步骤

### 5.1 发布清单 Checklist

- [ ] `deploy/.env` 已配置真实密钥（LiveKit、DB、Storage）
- [ ] `STORAGE_ENCRYPT_KEY` 已设（64 位 hex）
- [ ] `LIVEKIT_PUBLIC_URL` 保持留空（仅独立域名 / LiveKit Cloud 时显式填写）
- [ ] 有域名时 `GOSPEAK_DOMAIN` 已填、`COMPOSE_PROFILES` 含 `edge`，且用 `-f deploy/docker-compose.standard.yml` 启动；已有反代时不含 `edge`
- [ ] `DB_TYPE` 生产环境用 PostgreSQL/MySQL，非 SQLite
- [ ] `JWT_KEY` 已修改为强随机值（默认 `default-secret` 不可上生产）
- [ ] 端口防火墙开放: 80/443（Caddy 或自有反代）+ 7882/tcp + 7882/udp + 3478/udp（媒体）；不需要 7880/7881
- [ ] HTTPS 终结已配置（有域名用 `edge` 档位内置 Caddy，且必须使用全档位版；已有反代则指向 `8998`）
- [ ] 数据库备份策略已就位
- [ ] 日志收集已配置

### 5.2 完整上线流程

```bash
# 1. 克隆 + 配置
git clone <repo> && cd GOSpeak
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env 填入生产密钥（或用 deploy/init.sh 自动生成）

# 2. 启动依赖服务（精简起步：应用 + LiveKit；要 edge/postgres 等档位请改用 -f deploy/docker-compose.standard.yml）
docker compose -f deploy/docker-compose.yml up -d

# 3. 构建应用
docker build -t gospeak:latest .

# 4. 运行
docker run -d \
  --name gospeak \
  --env-file deploy/.env \
  -p 8998:8998 \
  --restart unless-stopped \
  gospeak:latest

# 5. 验证
curl http://localhost:8998/ping      # 存活探针（另有 /readyz 就绪探针）
```

### 5.3 反向代理与 HTTPS

只有一个上游 `gospeak:8998`：`/api`、`/ws`、`/uploads`、`/livekit` 全部同源走它。

- **有域名**：用全档位版并设 `COMPOSE_PROFILES=app,livekit,edge` + `GOSPEAK_DOMAIN=voice.example.com`（即 `-f deploy/docker-compose.standard.yml`），内置 Caddy（`deploy/Caddyfile`）自动申请证书并把 HTTP 重定向到 HTTPS。
- **已有反代**（Nginx / Traefik / Dokploy / 宝塔）：不要开 `edge`，把反代指向 `gospeak:8998` 即可；保留 `Host` 与 `X-Forwarded-Proto` 头、放行 WebSocket 升级。**不需要为 LiveKit 写任何 location。**
- **局域网 / 无域名**：`GOSPEAK_DOMAIN` 留空，Caddy 只听 `:80`（浏览器麦克风需要 localhost 或 HTTPS）。

详见 [`deploy/DEPLOY.md`](../deploy/DEPLOY.md) 第 2 节。

### 5.4 停止

```bash
docker stop gospeak
docker compose -f deploy/docker-compose.yml down
```

---

## 6. 运维

### 6.1 日志

```bash
docker logs -f gospeak
```

应用日志由 `logrus` 输出，格式与目标由 `LOG_FORMAT`（prod 默认 json）、`LOG_OUTPUT`（默认 stdout）、`LOG_FILE` 控制，可通过部署档位调整。

### 6.2 健康检查

应用容器需配 `HEALTHCHECK`（见 Dockerfile 加固项）。依赖服务在 `docker-compose.yml` 已配 healthcheck。

### 6.3 数据备份

- **SQLite:** 备份 `server/db/*.db` 文件
- **PostgreSQL:** `pg_dump`
- **RustFS:** 桶同步至异地

### 6.4 升级

```bash
git pull
docker build -t gospeak:latest .
docker stop gospeak && docker rm gospeak
docker run -d ... gospeak:latest  # 同 5.2 step 4
```

> Compose 部署的升级更简单：`docker compose -f deploy/docker-compose.standard.yml up -d`（档位版）或 `-f deploy/docker-compose.yml`（精简版），见 `deploy/DEPLOY.md`。

---

## 7. 故障排查

| 症状 | 排查 |
|------|------|
| 前端无音视频 | 检查 `SFU_PROVIDER` + LiveKit 服务状态 + WebRTC UDP 端口开放 |
| 信令连不上 | 检查反代是否放行 WebSocket 升级；确认 `/api/v1/signal/token` 返回的 `serverUrl` 是 `wss://<域名>/livekit` |
| JWT 无法刷新 | 检查内嵌 NATS 启动日志（NATS 不可用会 fail-fast）与 `JWT_KEY` 一致性 |
| 上传失败 | 检查 `STORAGE_TYPE` + RustFS/S3 凭证 + `STORAGE_ENCRYPT_KEY` |
| DB 连接失败 | `DB_TYPE` 不匹配，PostgreSQL/MySQL 需 `DB_HOST` 等填全 |

---

## 8. 参考

- 架构总览: `ARCHITECTURE.md`
- Agent 指南: `AGENTS.md`
- 一键编排方案: `deploy/docker-compose.standard.yml`
- 权威部署入口: `deploy/DEPLOY.md`
- 反代与 HTTPS: `deploy/Caddyfile` + `docs/deploy/reverse-proxy.md`
- 服务成熟度: `docs/sfu-provider-maturity.md`
