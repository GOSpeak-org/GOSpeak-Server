#!/usr/bin/env bash
# 单二进制部署：自动下载当前平台最新的 GOSpeak 与 LiveKit 发行包并落地为可运行服务。
#
#   curl -fsSL https://raw.githubusercontent.com/GOSpeak-org/GOSpeak-Server/release/deploy/install-binary.sh | bash
#   curl -fsSL <上面的地址> | bash -s -- --dir /opt/gospeak --without-systemd
#
# 默认安装到 /opt/gospeak，已存在的 .env 不会被覆盖（重复运行即升级）。
set -euo pipefail

GOSPEAK_REPO="GOSpeak-org/GOSpeak-Server"
LIVEKIT_REPO="livekit/livekit"
INSTALL_DIR="${GOSPEAK_DIR:-/opt/gospeak}"
RUN_USER=""
GOSPEAK_VERSION="latest"
LIVEKIT_VERSION="latest"
TARGET=""
WITH_LIVEKIT=1
WITH_SYSTEMD=1
DRY_RUN=0
WORK=""

usage() {
	cat <<EOF
Usage: install-binary.sh [options]

通过 GitHub Releases 下载当前平台最新的 GOSpeak 单二进制与 LiveKit Server，
校验 SHA256 后安装为 systemd 服务。可重复运行用于升级，不会覆盖已有 .env。

Options:
  --dir DIR              安装目录（默认 /opt/gospeak，或环境变量 GOSPEAK_DIR）
  --user USER            systemd 服务运行用户（默认当前用户；sudo 时用 SUDO_USER）
  --target OS-ARCH       目标平台，如 linux-amd64（默认自动检测）
  --version VER          GOSpeak 版本，如 1.1.0-rc.3（默认 latest）
  --livekit-version VER  LiveKit 版本，如 v1.13.11（默认 latest）
  --without-livekit      不下载 LiveKit
  --without-systemd      不生成/启动 systemd 服务
  --dry-run              只解析版本与 URL，不做任何下载和写入
  -h, --help             显示帮助

curl 直接运行示例：
  curl -fsSL https://raw.githubusercontent.com/$GOSPEAK_REPO/release/deploy/install-binary.sh | bash
EOF
}

fail() {
	echo "install-binary.sh: $*" >&2
	exit 1
}

log() {
	printf '==> %s\n' "$*"
}

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 3 --retry-delay 1 --connect-timeout 15 --max-time 600 "$@"
	elif command -v wget >/dev/null 2>&1; then
		local url="" quiet="-q" arg
		for arg in "$@"; do
			case "$arg" in
			-*) ;;
			*) url="$arg" ;;
			esac
		done
		wget $quiet --timeout=30 -O- "$url"
	else
		fail "需要 curl 或 wget"
	fi
}

api_fetch() {
	if [ -n "${GITHUB_TOKEN:-}" ] && command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 3 -H "Accept: application/vnd.github+json" \
			-H "Authorization: Bearer $GITHUB_TOKEN" "$1"
	else
		fetch "$1"
	fi
}

# sed 解析 GitHub API 的 pretty JSON；asset 名与 tag 均为简单 ASCII。
json_strings() {
	sed -n 's/.*"'"$1"'": *"\([^"]*\)".*/\1/p'
}

detect_target() {
	local os arch
	os="$(uname -s)"
	arch="$(uname -m)"
	case "$os" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	FreeBSD) os=freebsd ;;
	OpenBSD) os=openbsd ;;
	*) fail "不支持的系统: $os" ;;
	esac
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	armv7l | armv7 | armhf) arch=armv7 ;;
	*) fail "不支持的架构: $arch" ;;
	esac
	printf '%s-%s' "$os" "$arch"
}

latest_server_version() {
	local raw tag
	raw="$(mktemp)"
	api_fetch "https://api.github.com/repos/$GOSPEAK_REPO/releases?per_page=30" >"$raw"
	tag="$(json_strings tag_name <"$raw" | grep '^gospeak-server-v' | head -1 || true)"
	rm -f "$raw"
	[ -n "$tag" ] || fail "未找到 $GOSPEAK_REPO 的 gospeak-server-v* 发行版"
	printf '%s' "${tag#gospeak-server-v}"
}

latest_livekit_version() {
	local raw tag
	raw="$(mktemp)"
	api_fetch "https://api.github.com/repos/$LIVEKIT_REPO/releases/latest" >"$raw"
	tag="$(json_strings tag_name <"$raw" | head -1 || true)"
	rm -f "$raw"
	[ -n "$tag" ] || fail "未找到 $LIVEKIT_REPO 的最新发行版"
	printf '%s' "$tag"
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		fail "需要 sha256sum 或 shasum 来校验下载文件"
	fi
}

verify_checksum() {
	local file="$1" sums="$2" name="$3" expected actual
	expected="$(awk -v n="$name" '$2 == n { print $1; exit }' "$sums")"
	[ -n "$expected" ] || fail "校验和文件中缺少 $name"
	actual="$(sha256_file "$file")"
	[ "$actual" = "$expected" ] || fail "$name 的 SHA256 校验失败"
	log "SHA256 校验通过: $name"
}

rand_hex() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex "$1"
	else
		od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'
	fi
}

write_env() {
	local env_file="$INSTALL_DIR/.env"
	if [ -f "$env_file" ]; then
		log "保留已有配置: $env_file"
		# 生产启动（-e prod）强制要求 METRICS_TOKEN；老安装补一行，避免升级后直接 panic。
		if ! grep -q '^METRICS_TOKEN=' "$env_file"; then
			printf 'METRICS_TOKEN=%s\n' "$(rand_hex 32)" >>"$env_file"
			log "已为已有配置补充 METRICS_TOKEN（生产必填）"
		fi
		return
	fi
	local jwt_key storage_key metrics_token lk_key lk_secret
	jwt_key="$(rand_hex 32)"
	storage_key="$(rand_hex 32)"
	metrics_token="$(rand_hex 32)"
	lk_key="API$(rand_hex 6)"
	lk_secret="$(rand_hex 24)"
	cat >"$env_file" <<EOF
# 由 install-binary.sh 生成于 $(date -u +%Y-%m-%dT%H:%M:%SZ)
SERVER_PORT=8998
DB_TYPE=SQLite
DB_PATH=$INSTALL_DIR/data/app.db
DB_WAL=true
STORAGE_TYPE=local
STORAGE_LOCAL_DIR=$INSTALL_DIR/data/uploads
STORAGE_PATH_PREFIX=uploads/
STORAGE_ENCRYPT_KEY=$storage_key
JWT_KEY=$jwt_key
JWT_KEY_TTL=24h
# /metrics 访问令牌；server -e prod 启动强制要求，留空会被配置校验拒绝
METRICS_TOKEN=$metrics_token
SFU_PROVIDER=livekit
LIVEKIT_HOST=ws://127.0.0.1:7880
LIVEKIT_PUBLIC_URL=
LIVEKIT_KEY=$lk_key
LIVEKIT_SECRET=$lk_secret
LIVEKIT_KEYS="$lk_key: $lk_secret"
LIVEKIT_WEBHOOK_API_KEY=$lk_key
EOF
	chmod 600 "$env_file"
	log "已生成配置: ${env_file}（权限 600）"
}

write_livekit_config() {
	local cfg="$INSTALL_DIR/livekit.yaml"
	if [ -f "$cfg" ]; then
		log "保留已有 LiveKit 配置: $cfg"
		return
	fi
	cat >"$cfg" <<'EOF'
# 由 install-binary.sh 生成。密钥不写入本文件，由 .env 注入。
port: 7880
bind_addresses:
  - ""

rtc:
  tcp_port: 7882
  udp_port: 7882
  use_external_ip: true
  stun_servers:
    - stun.l.google.com:19302

keys: {}

turn:
  enabled: true
  udp_port: 3478
  external_tls: false

webhook:
  urls:
    - http://127.0.0.1:8998/api/v1/signal/webhook
EOF
	log "已生成 LiveKit 配置: $cfg"
}

write_systemd() {
	if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
		log "未检测到 systemd，跳过服务配置（手动执行: $INSTALL_DIR/gospeak server -e prod）"
		return
	fi

	local unit_dir=/etc/systemd/system systemctl_cmd="systemctl"
	local wanted_by=multi-user.target user_line="User=$RUN_USER"
	if [ "$(id -u)" -ne 0 ] || [ ! -w /etc/systemd/system ]; then
		unit_dir="${HOME}/.config/systemd/user"
		systemctl_cmd="systemctl --user"
		wanted_by=default.target
		user_line=""
	fi
	mkdir -p "$unit_dir"

	cat >"$unit_dir/gospeak.service" <<EOF
[Unit]
Description=GOSpeak Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
${user_line}
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$INSTALL_DIR/.env
ExecStart=$INSTALL_DIR/gospeak server -e prod
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=$wanted_by
EOF

	if [ "$WITH_LIVEKIT" -eq 1 ] && [ -x "$INSTALL_DIR/livekit-server" ]; then
		cat >"$unit_dir/livekit.service" <<EOF
[Unit]
Description=LiveKit Server (GOSpeak)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
${user_line}
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$INSTALL_DIR/.env
ExecStart=$INSTALL_DIR/livekit-server --config $INSTALL_DIR/livekit.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=$wanted_by
EOF
	fi

	$systemctl_cmd daemon-reload
	$systemctl_cmd enable --now gospeak.service
	if [ "$WITH_LIVEKIT" -eq 1 ] && [ -f "$unit_dir/livekit.service" ]; then
		$systemctl_cmd enable --now livekit.service
	fi
	log "systemd 服务已启用（${systemctl_cmd}）"
}

install_gospeak() {
	local version="$1" asset sums url tmp
	asset="gospeak-server-v${version}-${TARGET}"
	sums="SHA256SUMS-gospeak-server-v${version}.txt"
	url="https://github.com/$GOSPEAK_REPO/releases/download/gospeak-server-v${version}/${asset}"
	log "下载 GOSpeak ${version} (${TARGET})"
	fetch "$url" >"$WORK/$asset"
	fetch "https://github.com/$GOSPEAK_REPO/releases/download/gospeak-server-v${version}/${sums}" >"$WORK/$sums"
	verify_checksum "$WORK/$asset" "$WORK/$sums" "$asset"
	install -m 0755 "$WORK/$asset" "$INSTALL_DIR/gospeak"
}

install_livekit() {
	local tag="$1" version asset sums url tmp
	version="${tag#v}"
	asset="livekit_${version}_${TARGET/-/_}.tar.gz"
	case "$TARGET" in
	linux-*) ;;
	*) log "LiveKit 官方发行包目前仅提供 Linux，跳过（$(uname -s) 可用源码编译：scripts/dev-livekit.sh）"; return ;;
	esac
	sums="checksums.txt"
	url="https://github.com/$LIVEKIT_REPO/releases/download/${tag}/${asset}"
	log "下载 LiveKit ${tag} (${TARGET})"
	fetch "$url" >"$WORK/$asset"
	fetch "https://github.com/$LIVEKIT_REPO/releases/download/${tag}/${sums}" >"$WORK/$sums"
	verify_checksum "$WORK/$asset" "$WORK/$sums" "$asset"
	tmp="$(mktemp -d "$WORK/lk.XXXXXX")"
	tar -xzf "$WORK/$asset" -C "$tmp"
	[ -f "$tmp/livekit-server" ] || fail "$asset 中未找到 livekit-server"
	install -m 0755 "$tmp/livekit-server" "$INSTALL_DIR/livekit-server"
}

main() {
	while [ $# -gt 0 ]; do
		case "$1" in
		--dir) INSTALL_DIR="${2:?--dir 需要参数}"; shift 2 ;;
		--user) RUN_USER="${2:?--user 需要参数}"; shift 2 ;;
		--target) TARGET="${2:?--target 需要参数}"; shift 2 ;;
		--version) GOSPEAK_VERSION="${2:?--version 需要参数}"; shift 2 ;;
		--livekit-version) LIVEKIT_VERSION="${2:?--livekit-version 需要参数}"; shift 2 ;;
		--without-livekit) WITH_LIVEKIT=0; shift ;;
		--without-systemd) WITH_SYSTEMD=0; shift ;;
		--dry-run) DRY_RUN=1; shift ;;
		-h | --help) usage; return ;;
		*) usage >&2; fail "未知参数: $1" ;;
		esac
	done

	[ -n "$TARGET" ] || TARGET="$(detect_target)"
	[ -n "$RUN_USER" ] || RUN_USER="${SUDO_USER:-$(id -un)}"

	if [ "$GOSPEAK_VERSION" = latest ]; then
		GOSPEAK_VERSION="$(latest_server_version)"
	fi
	if [ "$WITH_LIVEKIT" -eq 1 ] && [ "$LIVEKIT_VERSION" = latest ]; then
		LIVEKIT_VERSION="$(latest_livekit_version)"
	fi

	if [ "$DRY_RUN" -eq 1 ]; then
		local lk_state
		if [ "$WITH_LIVEKIT" -eq 0 ]; then
			lk_state=disabled
		elif [[ "$TARGET" == linux-* ]]; then
			lk_state="$LIVEKIT_VERSION"
		else
			lk_state="skipped (官方发行包仅提供 Linux)"
		fi
		cat <<EOF
目标平台:    $TARGET
安装目录:    $INSTALL_DIR
GOSpeak:     v$GOSPEAK_VERSION
  binary:    https://github.com/$GOSPEAK_REPO/releases/download/gospeak-server-v${GOSPEAK_VERSION}/gospeak-server-v${GOSPEAK_VERSION}-${TARGET}
  sums:      https://github.com/$GOSPEAK_REPO/releases/download/gospeak-server-v${GOSPEAK_VERSION}/SHA256SUMS-gospeak-server-v${GOSPEAK_VERSION}.txt
LiveKit:     $lk_state
systemd:     $([ "$WITH_SYSTEMD" -eq 1 ] && echo enabled || echo disabled)
运行用户:    $RUN_USER
EOF
		return
	fi

	if ! mkdir -p "$INSTALL_DIR/data/uploads" 2>/dev/null; then
		fail "无法创建 $INSTALL_DIR（权限不足）。请用 sudo 运行，或用 --dir ~/gospeak 安装到用户目录"
	fi
	[ -w "$INSTALL_DIR" ] || fail "$INSTALL_DIR 不可写，请改用 sudo 或 --dir 指定其它目录"
	WORK="$(mktemp -d)"
	trap 'rm -rf "$WORK"' EXIT
	trap 'exit 1' HUP INT TERM

	install_gospeak "$GOSPEAK_VERSION"
	if [ "$WITH_LIVEKIT" -eq 1 ]; then
		install_livekit "$LIVEKIT_VERSION"
	fi
	write_env
	if [ "$WITH_LIVEKIT" -eq 1 ]; then
		write_livekit_config
	fi
	if ! chown -R "$RUN_USER" "$INSTALL_DIR"; then
		echo "警告: 无法把 $INSTALL_DIR 的属主改为 $RUN_USER，服务可能因权限不足启动失败。" >&2
		echo "       可稍后手动执行: sudo chown -R $RUN_USER $INSTALL_DIR" >&2
	fi
	if [ "$WITH_SYSTEMD" -eq 1 ]; then
		write_systemd
	fi

	cat <<EOF

安装完成: $INSTALL_DIR
  gospeak          $("$INSTALL_DIR/gospeak" version 2>/dev/null || echo "v$GOSPEAK_VERSION")
  livekit-server   $([ -x "$INSTALL_DIR/livekit-server" ] && echo "$LIVEKIT_VERSION" || echo "未安装")
  配置             $INSTALL_DIR/.env
  数据             $INSTALL_DIR/data

放行端口: 8998/TCP（Web）、7882/TCP+UDP（媒体）、3478/UDP（TURN）
服务日志: journalctl -u gospeak -f
EOF
}

if [ -z "${INSTALL_BINARY_NO_MAIN:-}" ]; then
	main "$@"
fi
