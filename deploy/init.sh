#!/usr/bin/env bash
# =============================================================================
# GOSpeak 部署引导脚本
# =============================================================================
# 职责：
#   1. 以 deploy/.env.example 为模板生成 deploy/.env（含随机密钥）
#   2. 归一 deploy/livekit/livekit.yaml，使其不含任何密钥
#   3. 校验 .env / livekit.yaml / Caddyfile 的一致性，不一致就报错退出
#
# 用法:
#   ./init.sh                          # 生成 .env 并归一 livekit.yaml
#   ./init.sh --force                  # 覆盖已存在的 .env（保留已填的 GOSPEAK_DOMAIN）
#   ./init.sh --check                  # 只做一致性校验，不写任何文件（无 .env 也能通过，CI 用）
#   ./init.sh --domain voice.example.com
#                                      # 写入 GOSPEAK_DOMAIN（启用 Caddy 自动 HTTPS）
#
# 环境变量覆盖（等价于上面的参数）：
#   GOSPEAK_DOMAIN=voice.example.com ./init.sh
# =============================================================================
set -euo pipefail

cd "$(dirname "$0")"

FORCE=0
CHECK_ONLY=0
DOMAIN_ARG=""

usage() { sed -n '10,18p' "$0"; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    livekit) shift ;;            # 兼容旧用法：仅 LiveKit 一种档位
    --force) FORCE=1; shift ;;
    --check) CHECK_ONLY=1; shift ;;
    --domain)
      [[ $# -ge 2 ]] || { echo "--domain 需要一个值（如 voice.example.com）" >&2; exit 1; }
      DOMAIN_ARG="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "未知参数: $1" >&2; usage ;;
  esac
done

ENV_FILE=".env"
ENV_TPL=".env.example"
LIVEKIT_YAML="livekit/livekit.yaml"
CADDYFILE="Caddyfile"

[[ -f "$ENV_TPL" ]] || { echo "缺少模板 $ENV_TPL" >&2; exit 1; }
[[ -f "$LIVEKIT_YAML" ]] || { echo "缺少 $LIVEKIT_YAML" >&2; exit 1; }

# ── 读取 .env 中某个键的值（去掉引号与行尾空白）─────────────────────────────
env_get() {
  local key="$1" file="$2"
  [[ -f "$file" ]] || return 0
  sed -n "s/^${key}=//p" "$file" | tail -1 | sed -e 's/[[:space:]]*$//' -e 's/^["'\'']//' -e 's/["'\'']$//'
}

# ── 域名归一：容忍用户填 https://xxx/ 或带路径，只保留主机名 ────────────────
normalize_domain() {
  local d="$1"
  d="${d#http://}"
  d="${d#https://}"
  d="${d%%/*}"
  printf '%s' "$d"
}

# ── 档位判定 ────────────────────────────────────────────────────────────────
# COMPOSE_PROFILES 只对全档位编排（docker-compose.standard.yml）生效；
# 精简编排（docker-compose.yml）忽略它，等价于 app,livekit 两档。
has_profile() {
  local name="$1" profiles
  profiles="$(env_get COMPOSE_PROFILES "$ENV_FILE")"
  [[ ",${profiles}," == *",${name},"* ]]
}

# ── 一致性校验 ──────────────────────────────────────────────────────────────
# 校验分两类：
#   A. 仓库结构合规（CI 也跑，不依赖 .env 存在）
#      - livekit.yaml 的 keys 必须为空（{}），不得硬编码密钥
#      - livekit.yaml 的 webhook 不得含明文 api_key
#      - Caddyfile 必须存在且引用 {$GOSPEAK_DOMAIN（edge 档位依赖）
#      - compose 的注入表达式与 .env 键名匹配
#   B. 本地部署就绪（仅当 .env 存在时校验）
#      - .env 的 LIVEKIT_KEY / LIVEKIT_SECRET 非空且非占位值
#      - LIVEKIT_PUBLIC_URL 可留空（同源自动推导）；显式填写时只校验 ws/wss 前缀
check_consistency() {
  local rc=0

  # ── A. 仓库结构合规 ──
  if grep -qE '^\s+API[0-9a-zA-Z]+:\s*\S+' "$LIVEKIT_YAML"; then
    echo "  [x] $LIVEKIT_YAML 仍含硬编码 keys，请改为 keys: {}" >&2; rc=1
  else
    echo "  [ok] $LIVEKIT_YAML 不含硬编码密钥"
  fi

  if grep -qE '^keys:\s*\{\}\s*$' "$LIVEKIT_YAML"; then
    echo "  [ok] $LIVEKIT_YAML 的 keys 为空，由环境变量注入"
  else
    echo "  [x] $LIVEKIT_YAML 的 keys 应为 'keys: {}'" >&2; rc=1
  fi

  if grep -qE '^\s+api_key:\s*\S+' "$LIVEKIT_YAML"; then
    echo "  [x] $LIVEKIT_YAML 的 webhook 仍含明文 api_key，应删除" >&2; rc=1
  else
    echo "  [ok] $LIVEKIT_YAML 的 webhook 不含明文 api_key"
  fi

  # Caddyfile：edge 档位的唯一入口配置，必须存在且用环境变量拼站点地址，
  # 否则 GOSPEAK_DOMAIN 为空时会写死一个不存在的域名。
  if [[ ! -f "$CADDYFILE" ]]; then
    echo "  [x] 缺少 ${CADDYFILE}（edge 档位需要它）" >&2; rc=1
  elif ! grep -qF '{$GOSPEAK_DOMAIN' "$CADDYFILE"; then
    echo "  [x] $CADDYFILE 未引用 {\$GOSPEAK_DOMAIN，无法随 .env 切换域名" >&2; rc=1
  else
    echo "  [ok] $CADDYFILE 存在且按 {\$GOSPEAK_DOMAIN 生成站点地址"
  fi

  # 所有 compose 文件都必须按 .env 的键名注入 LiveKit 密钥，
  # 避免新增的部署变体漏掉注入。
  #
  # 注意：Dokploy 部署时会重写 compose 文件（剥离注释、去引号、展开数组、
  # 注入 labels），所以这里只做语义校验——检查键名与变量引用是否出现，
  # 不依赖引号、缩进或行内格式，否则在 Dokploy 托管的环境里会误报。
  local compose_files=(docker-compose*.yml)
  local compose_ok=1
  for cf in "${compose_files[@]}"; do
    [[ -f "$cf" ]] || continue
    local has_keys=0 has_webhook=0
    # LIVEKIT_KEYS 的值必须同时引用 LIVEKIT_KEY 与 LIVEKIT_SECRET
    if awk '
      /^[[:space:]]*LIVEKIT_KEYS:/ {
        if ($0 ~ /LIVEKIT_KEY/ && $0 ~ /LIVEKIT_SECRET/) { found=1 }
      }
      END { exit(found ? 0 : 1) }
    ' "$cf"; then has_keys=1; fi
    # LIVEKIT_WEBHOOK_API_KEY 的值必须引用 LIVEKIT_KEY
    if awk '
      /^[[:space:]]*LIVEKIT_WEBHOOK_API_KEY:/ {
        if ($0 ~ /LIVEKIT_KEY/) { found=1 }
      }
      END { exit(found ? 0 : 1) }
    ' "$cf"; then has_webhook=1; fi

    if [[ $has_keys -eq 1 && $has_webhook -eq 1 ]]; then
      echo "  [ok] $cf 已按 .env 的键名注入 LiveKit 密钥"
    else
      [[ $has_keys -eq 0 ]] && echo "  [x] $cf 缺少 LIVEKIT_KEYS 注入（需引用 LIVEKIT_KEY 与 LIVEKIT_SECRET）" >&2
      [[ $has_webhook -eq 0 ]] && echo "  [x] $cf 缺少 LIVEKIT_WEBHOOK_API_KEY 注入（需引用 LIVEKIT_KEY）" >&2
      compose_ok=0
    fi
  done
  [[ $compose_ok -eq 1 ]] || rc=1

  # ── B. 本地部署就绪（CI 无 .env，跳过）──
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "  [-] 未找到 ${ENV_FILE}，跳过部署就绪检查（CI 场景正常）"
    return $rc
  fi

  local key secret
  key="$(env_get LIVEKIT_KEY "$ENV_FILE")"
  secret="$(env_get LIVEKIT_SECRET "$ENV_FILE")"
  if [[ -z "$key" || -z "$secret" ]]; then
    echo "  [x] $ENV_FILE 缺少 LIVEKIT_KEY / LIVEKIT_SECRET" >&2; rc=1
  elif [[ "$key" == "change-me" || "$secret" == "change-me" ]]; then
    echo "  [x] $ENV_FILE 的 LiveKit 密钥仍是占位值 change-me" >&2; rc=1
  else
    echo "  [ok] $ENV_FILE 已配置 LiveKit 密钥（key=${key:0:12}...）"
  fi

  # 域名：留空是正常形态（Caddy 只听 :80 / 已有反代），非空时打印生效值。
  local domain
  domain="$(env_get GOSPEAK_DOMAIN "$ENV_FILE")"
  if [[ -z "$domain" ]]; then
    echo "  [-] GOSPEAK_DOMAIN 未设置：Caddy 只监听 80（局域网或已有反代）"
  else
    echo "  [ok] GOSPEAK_DOMAIN=${domain}（edge 档位将自动申请证书）"
  fi

  # 浏览器侧地址：默认留空 = 应用按请求 Host 推导 wss://<域名>/livekit。
  # 仅在显式填写时校验前缀，避免误填成 https:// 或容器内网地址。
  local public_url
  public_url="$(env_get LIVEKIT_PUBLIC_URL "$ENV_FILE")"
  if [[ -z "$public_url" || "$public_url" == "auto" ]]; then
    echo "  [ok] LIVEKIT_PUBLIC_URL 留空，使用同源 /livekit 自动推导"
  else
    case "$public_url" in
      ws://*|wss://*)
        echo "  [ok] LIVEKIT_PUBLIC_URL=${public_url}（独立域名 / LiveKit Cloud）" ;;
      *)
        echo "  [x] LIVEKIT_PUBLIC_URL 必须以 ws:// 或 wss:// 开头（当前: ${public_url}）" >&2
        rc=1 ;;
    esac
  fi

  # /metrics：生产配置校验强制要求 METRICS_TOKEN，空值会让进程启动即 panic。
  # 在这里提前拦下来，比等到容器反复重启再翻日志便宜得多。
  local app_env metrics_token
  app_env="$(env_get APP_ENV "$ENV_FILE")"
  metrics_token="$(env_get METRICS_TOKEN "$ENV_FILE")"
  if [[ -z "$metrics_token" ]]; then
    if [[ "$app_env" == "prod" || "$app_env" == "production" ]]; then
      echo "  [x] $ENV_FILE 是生产环境（APP_ENV=$app_env）但 METRICS_TOKEN 为空，应用启动会直接失败" >&2
      echo "      修复：./init.sh --force 重新生成，或在 $ENV_FILE 补一行 METRICS_TOKEN=<随机字符串>" >&2
      rc=1
    else
      echo "  [-] METRICS_TOKEN 未设置：dev 环境 /metrics 免鉴权（生产必填）"
    fi
  else
    echo "  [ok] $ENV_FILE 已设置 METRICS_TOKEN（APP_ENV=${app_env:-dev}）"
  fi

  # Prometheus 抓 /metrics 要带同一令牌，走文件注入免得密钥进配置文件。
  # 只有启用了 observability 档位才需要这个文件，精简档不生成也不校验。
  if has_profile observability; then
    local token_file="observability/prometheus/metrics_token"
    if [[ ! -f "$token_file" ]]; then
      echo "  [x] 已启用 observability 档位，但缺少 $token_file：prometheus 抓取会 401" >&2
      echo "      修复：./init.sh --force（会按 .env 的档位重新生成）" >&2
      rc=1
    elif [[ "$(tr -d '\r\n' < "$token_file")" != "$metrics_token" ]]; then
      echo "  [x] $token_file 与 .env 的 METRICS_TOKEN 不一致：prometheus 抓取会 401" >&2
      rc=1
    else
      echo "  [ok] $token_file 与 .env 的 METRICS_TOKEN 一致"
    fi
  else
    echo "  [-] 未启用 observability 档位，跳过 prometheus 抓取令牌文件"
  fi

  return $rc
}

# ── 归一 livekit.yaml：剥离密钥，保留静态配置 ───────────────────────────────
normalize_livekit_yaml() {
  local tmp
  tmp="$(mktemp)"

  # 1. keys: 下所有缩进条目 → 替换为 keys: {}
  # 2. webhook 下的 api_key 行 → 删除
  awk '
    # keys 块：把 "keys:" 之后的缩进条目全部吞掉，输出 keys: {}
    /^keys:[[:space:]]*$/ { print "keys: {}"; in_keys=1; next }
    in_keys && /^[[:space:]]+/ { next }
    in_keys && !/^[[:space:]]+/ { in_keys=0 }

    # webhook 块内的 api_key 行删除
    /^webhook:[[:space:]]*$/ { in_webhook=1; print; next }
    in_webhook && /^[[:space:]]+api_key:/ { next }
    in_webhook && /^[^[:space:]]/ { in_webhook=0 }

    { print }
  ' "$LIVEKIT_YAML" > "$tmp"

  if cmp -s "$tmp" "$LIVEKIT_YAML"; then
    echo "  [ok] $LIVEKIT_YAML 已是无密钥形态"
    rm -f "$tmp"
    return 0
  fi

  mv "$tmp" "$LIVEKIT_YAML"
  echo "  [ok] 已归一 ${LIVEKIT_YAML}（移除硬编码 keys / webhook.api_key）"
}

# ── --check：只校验 ─────────────────────────────────────────────────────────
if [[ $CHECK_ONLY -eq 1 ]]; then
  echo "==> 校验 .env / livekit.yaml / Caddyfile 一致性"
  if check_consistency; then
    echo "一致性校验通过。"
    exit 0
  fi
  echo "一致性校验失败。" >&2
  exit 1
fi

if [[ -f "$ENV_FILE" && $FORCE -ne 1 ]]; then
  echo "deploy/.env 已存在，如需重新生成: ./init.sh --force" >&2
  echo "（只做一致性校验: ./init.sh --check）" >&2
  exit 1
fi

rand() { openssl rand -hex 32; }

echo "==> 生成随机密钥"
JWT_KEY=$(rand)
STORAGE_ENCRYPT_KEY=$(rand)
# 生产配置校验强制要求 METRICS_TOKEN，缺了会启动即 panic，必须一并生成。
METRICS_TOKEN=$(rand)
POSTGRES_PASSWORD=$(rand)
LIVEKIT_KEY="API$(openssl rand -hex 6)"
LIVEKIT_SECRET=$(openssl rand -base64 48 | tr -d '/+=' | cut -c1-43)

# --force 重新生成时保留已填的域名与档位，避免每次重置都要重填。
PREV_DOMAIN=""
PREV_PROFILES=""
if [[ -f "$ENV_FILE" ]]; then
  PREV_DOMAIN="$(env_get GOSPEAK_DOMAIN "$ENV_FILE")"
  PREV_PROFILES="$(env_get COMPOSE_PROFILES "$ENV_FILE")"
fi

echo "==> 写出 ${ENV_FILE}（以 ${ENV_TPL} 为模板，替换全部密钥）"
# 用模板生成而非内联 heredoc，保证 .env 与 .env.example 字段永远一致；
# 新增配置项只需改模板，不会出现 init.sh 漏字段。
sed \
  -e "s|^JWT_KEY=.*|JWT_KEY=$JWT_KEY|" \
  -e "s|^STORAGE_ENCRYPT_KEY=.*|STORAGE_ENCRYPT_KEY=$STORAGE_ENCRYPT_KEY|" \
  -e "s|^METRICS_TOKEN=.*|METRICS_TOKEN=$METRICS_TOKEN|" \
  -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$POSTGRES_PASSWORD|" \
  -e "s|^LIVEKIT_KEY=.*|LIVEKIT_KEY=$LIVEKIT_KEY|" \
  -e "s|^LIVEKIT_SECRET=.*|LIVEKIT_SECRET=$LIVEKIT_SECRET|" \
  "$ENV_TPL" > "$ENV_FILE"

# 沿用旧档位：模板只给 app,livekit，用户手动加的 observability/postgres 不应被重置。
if [[ -n "$PREV_PROFILES" ]]; then
  sed -i.bak "s|^COMPOSE_PROFILES=.*|COMPOSE_PROFILES=$PREV_PROFILES|" "$ENV_FILE"
  rm -f "$ENV_FILE.bak"
  echo "  [ok] 沿用已有 COMPOSE_PROFILES=$PREV_PROFILES"
fi

# 域名优先级：命令行参数 > 环境变量 > 已有 .env 的值
DOMAIN="$(normalize_domain "${DOMAIN_ARG:-${GOSPEAK_DOMAIN:-$PREV_DOMAIN}}")"
if [[ -n "$DOMAIN" ]]; then
  sed -i.bak "s|^GOSPEAK_DOMAIN=.*|GOSPEAK_DOMAIN=$DOMAIN|" "$ENV_FILE"
  rm -f "$ENV_FILE.bak"
  echo "  [ok] GOSPEAK_DOMAIN=$DOMAIN"

  # 有域名就顺手把 edge 档位加上：少了它，填了域名也不会有人来终结 TLS，
  # 用户还得再改一次 COMPOSE_PROFILES —— 这正是要消灭的手工步骤。
  profiles="$(env_get COMPOSE_PROFILES "$ENV_FILE")"
  if [[ ",${profiles}," != *,edge,* ]]; then
    profiles="${profiles:+$profiles,}edge"
    sed -i.bak "s|^COMPOSE_PROFILES=.*|COMPOSE_PROFILES=$profiles|" "$ENV_FILE"
    rm -f "$ENV_FILE.bak"
    echo "  [ok] COMPOSE_PROFILES=${profiles}（已自动启用内置 Caddy）"
  fi
else
  echo "  [ok] GOSPEAK_DOMAIN 留空（Caddy 只听 80；不开 edge 档位可忽略）"
fi

# Prometheus 抓 /metrics 要带 Bearer 令牌，用文件注入免得把密钥写进配置文件。
# 只有 observability 档位需要；644 是因为容器内 prometheus 以 nobody 运行。
if has_profile observability; then
  PROM_TOKEN_FILE="observability/prometheus/metrics_token"
  printf '%s' "$METRICS_TOKEN" > "$PROM_TOKEN_FILE"
  chmod 644 "$PROM_TOKEN_FILE"
  echo "  [ok] 已写出 ${PROM_TOKEN_FILE}（prometheus 抓取令牌）"
else
  echo "  [-] 未启用 observability 档位，不生成 prometheus 抓取令牌文件"
fi

echo "==> 归一 $LIVEKIT_YAML"
normalize_livekit_yaml

echo "==> 校验一致性"
if ! check_consistency; then
  echo "" >&2
  echo "一致性校验失败，请检查上面的 [x] 项。" >&2
  exit 1
fi

echo ""
if has_profile observability; then
  echo "完成。当前 .env 启用了 observability 档位，用全档位编排启动:"
  echo "  docker compose -f deploy/docker-compose.standard.yml up -d --build"
else
  echo "完成。当前是精简档位（app + livekit）:"
  echo "  docker compose -f deploy/docker-compose.yml up -d --build"
fi
echo "其余命令一律显式带 -f（裸 up -d 只会选中精简版，不启用 edge/HTTPS）:"
cat <<'EOF'

  # 有域名：填 GOSPEAK_DOMAIN，并启用内置 Caddy（自动 HTTPS）
  #   .env 中: COMPOSE_PROFILES=app,livekit,edge
  #            GOSPEAK_DOMAIN=voice.example.com
  #   然后:    cd .. && docker compose -f deploy/docker-compose.standard.yml up -d --build

  # 已有反代（Nginx / Traefik / Dokploy）：不要开 edge
  #   .env 中: COMPOSE_PROFILES=app,livekit
  #   把反代指向宿主机 8998 即可

  cd ..
  docker compose -f deploy/docker-compose.yml up -d --build     # 精简起步
  # docker compose -f deploy/docker-compose.standard.yml up -d --build  # 全档位（含 edge 自动 HTTPS）
  docker compose -f deploy/docker-compose.yml logs -f gospeak   # 看日志
  curl http://localhost:8998/ping  # 健康检查

提示:
  - 有域名时 Caddy 自动申请/续期证书，浏览器麦克风需要安全上下文（HTTPS）
  - 不需要为 LiveKit 配置第二个域名或证书：信令走应用同源 /livekit 反代，
    LIVEKIT_PUBLIC_URL 保持留空即可；仅独立域名或 LiveKit Cloud 时才填
  - WebRTC 媒体走 UDP 直连，防火墙只需放通 7882/udp、7882/tcp、3478/udp
  - 数据备份: docker run --rm -v gospeak_gospeak-db:/data -v "$PWD":/bak alpine \
      cp /data/app.db /bak/app-backup-$(date +%F).db
EOF
