#!/usr/bin/env bash
#
# GoPulse — kurulum ve güncelleme script'i
#
# Ne yapar:
#   - Gerekli araçları (git, curl) ve Go'yu (yoksa/eskiyse) hazırlar
#   - Projeyi GitHub'dan çeker (veya mevcut kopyayı günceller)
#   - CGO'suz statik binary'yi derler
#   - Bir systemd servisi olarak kurar ve başlatır
#
# İdempotent: zaten kuruluysa YALNIZCA kodu günceller; veritabanına
# (/var/lib/gopulse) asla dokunmaz.
#
# Kullanım (root/sudo gerekir):
#   sudo bash install.sh
# veya doğrudan:
#   curl -fsSL https://raw.githubusercontent.com/atillakurtulussimsek/GoPulse/main/deploy/install.sh | sudo bash

set -euo pipefail

# ----------------------------------------------------------------------------
# Yapılandırma (gerektiğinde düzenleyin veya ortam değişkeniyle geçersiz kılın)
# ----------------------------------------------------------------------------
REPO_URL="${GOPULSE_REPO_URL:-https://github.com/atillakurtulussimsek/GoPulse.git}"
BRANCH="${GOPULSE_BRANCH:-main}"

APP_USER="${GOPULSE_USER:-gopulse}"
APP_GROUP="${GOPULSE_GROUP:-gopulse}"

INSTALL_DIR="${GOPULSE_INSTALL_DIR:-/opt/gopulse}"   # kaynak + binary
SRC_DIR="${INSTALL_DIR}/src"                         # git deposu
BIN_PATH="${INSTALL_DIR}/gopulse"                    # derlenen binary
DATA_DIR="${GOPULSE_DATA_DIR:-/var/lib/gopulse}"     # veritabanı (KORUNUR)
DB_PATH="${DATA_DIR}/gopulse.db"

SERVICE_NAME="gopulse"
UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

# Çalışma ayarları (HTTPS reverse proxy arkasında).
LISTEN_ADDR="${GOPULSE_LISTEN_ADDR:-127.0.0.1:8080}"
COOKIE_SECURE="${GOPULSE_COOKIE_SECURE:-true}"

# Go yoksa/eskiyse kurulacak sürüm.
GO_VERSION="${GOPULSE_GO_VERSION:-1.23.12}"
GO_MIN_MINOR=23

# /usr/local/go varsa PATH'e ekle (önceki kurulumlar için).
export PATH="/usr/local/go/bin:${PATH}"

# ----------------------------------------------------------------------------
# Yardımcılar
# ----------------------------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m  !\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31mHATA:\033[0m %s\n' "$*" >&2; exit 1; }

require_root() {
	[ "$(id -u)" -eq 0 ] || die "Bu script root gerektirir. 'sudo bash install.sh' ile çalıştırın."
}

detect_arch() {
	case "$(uname -m)" in
		x86_64|amd64)  echo "amd64" ;;
		aarch64|arm64) echo "arm64" ;;
		*) die "Desteklenmeyen mimari: $(uname -m)" ;;
	esac
}

# Paket yöneticisiyle bir paket kurar (git/curl için).
install_pkg() {
	local pkg="$1"
	if   command -v apt-get >/dev/null 2>&1; then apt-get update -qq && apt-get install -y -qq "$pkg"
	elif command -v dnf     >/dev/null 2>&1; then dnf install -y -q "$pkg"
	elif command -v yum     >/dev/null 2>&1; then yum install -y -q "$pkg"
	elif command -v apk     >/dev/null 2>&1; then apk add --no-cache "$pkg"
	elif command -v pacman  >/dev/null 2>&1; then pacman -Sy --noconfirm "$pkg"
	else die "'$pkg' bulunamadı ve otomatik kurulamadı. Lütfen elle kurun."
	fi
}

ensure_tool() {
	local tool="$1" pkg="${2:-$1}"
	command -v "$tool" >/dev/null 2>&1 && return
	log "'$tool' kuruluyor..."
	install_pkg "$pkg"
	command -v "$tool" >/dev/null 2>&1 || die "'$tool' kurulamadı."
}

# Kurulu Go'nun minör sürümü GO_MIN_MINOR'dan büyük/eşit mi?
go_is_recent() {
	command -v go >/dev/null 2>&1 || return 1
	local v major minor
	v="$(go env GOVERSION 2>/dev/null | sed 's/^go//')" || return 1
	major="${v%%.*}"; minor="${v#*.}"; minor="${minor%%.*}"
	[ "${major:-0}" -gt 1 ] || { [ "${major:-0}" -eq 1 ] && [ "${minor:-0}" -ge "$GO_MIN_MINOR" ]; }
}

ensure_go() {
	if go_is_recent; then
		ok "Go mevcut: $(go env GOVERSION)"
		return
	fi
	local arch tarball url
	arch="$(detect_arch)"
	tarball="go${GO_VERSION}.linux-${arch}.tar.gz"
	url="https://go.dev/dl/${tarball}"

	log "Go ${GO_VERSION} kuruluyor (${url})..."
	rm -rf /usr/local/go
	curl -fsSL "$url" -o "/tmp/${tarball}"
	tar -C /usr/local -xzf "/tmp/${tarball}"
	rm -f "/tmp/${tarball}"
	export PATH="/usr/local/go/bin:${PATH}"
	go_is_recent || die "Go kurulumu başarısız."
	ok "Go kuruldu: $(go env GOVERSION)"
}

ensure_user() {
	if id "$APP_USER" >/dev/null 2>&1; then
		ok "Kullanıcı mevcut: $APP_USER"
		return
	fi
	log "Servis kullanıcısı oluşturuluyor: $APP_USER"
	useradd --system --no-create-home --shell /usr/sbin/nologin "$APP_USER"
}

fetch_source() {
	mkdir -p "$INSTALL_DIR"
	if [ -d "${SRC_DIR}/.git" ]; then
		log "Mevcut kaynak güncelleniyor (${BRANCH})..."
		git -C "$SRC_DIR" fetch --depth 1 origin "$BRANCH"
		git -C "$SRC_DIR" reset --hard FETCH_HEAD
		ok "Kaynak güncel: $(git -C "$SRC_DIR" rev-parse --short HEAD)"
	elif [ -e "$SRC_DIR" ] && [ -n "$(ls -A "$SRC_DIR" 2>/dev/null)" ]; then
		die "$SRC_DIR bir git deposu değil ama boş da değil. Elle temizleyip tekrar deneyin."
	else
		log "Depo klonlanıyor: $REPO_URL ($BRANCH)"
		git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$SRC_DIR"
		ok "Klonlandı: $(git -C "$SRC_DIR" rev-parse --short HEAD)"
	fi
}

build_binary() {
	log "Statik binary derleniyor..."
	# CGO'suz (modernc.org/sqlite saf Go) → gcc gerekmez, tek dosya.
	# Önce .new'e derleyip atomik olarak taşırız (çalışan servisi bozmadan).
	( cd "$SRC_DIR" && \
	  GOTOOLCHAIN=local CGO_ENABLED=0 \
	  go build -trimpath -ldflags="-s -w" -o "${BIN_PATH}.new" ./cmd/gopulse )
	mv -f "${BIN_PATH}.new" "$BIN_PATH"
	ok "Derlendi: $BIN_PATH"
}

write_unit() {
	log "systemd birimi yazılıyor: $UNIT_PATH"
	cat > "$UNIT_PATH" <<UNIT
[Unit]
Description=GoPulse uptime izleme botu
Documentation=${REPO_URL%.git}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_GROUP}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${BIN_PATH}
Restart=on-failure
RestartSec=5

Environment=GOPULSE_LISTEN_ADDR=${LISTEN_ADDR}
Environment=GOPULSE_DB_PATH=${DB_PATH}
Environment=GOPULSE_COOKIE_SECURE=${COOKIE_SECURE}

# Güvenlik sıkılaştırma
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
ReadWritePaths=${DATA_DIR}

[Install]
WantedBy=multi-user.target
UNIT
}

set_permissions() {
	# Binary ve kaynak dizini + KORUNAN veri dizini sahiplik/izinleri.
	mkdir -p "$DATA_DIR"
	chown -R "${APP_USER}:${APP_GROUP}" "$INSTALL_DIR" "$DATA_DIR"
	chmod 750 "$DATA_DIR"
}

start_service() {
	command -v systemctl >/dev/null 2>&1 || die "systemctl bulunamadı (systemd gerekli)."
	log "Servis etkinleştiriliyor ve (yeniden) başlatılıyor..."
	systemctl daemon-reload
	systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
	systemctl restart "$SERVICE_NAME"
	sleep 1
	if systemctl is-active --quiet "$SERVICE_NAME"; then
		ok "Servis çalışıyor: $SERVICE_NAME"
	else
		warn "Servis başlatılamadı. Günlükler: journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
		exit 1
	fi
}

# ----------------------------------------------------------------------------
# Akış
# ----------------------------------------------------------------------------
main() {
	require_root

	local first_install="no"
	[ -d "${SRC_DIR}/.git" ] || first_install="yes"
	if [ "$first_install" = "yes" ]; then
		log "GoPulse KURULUYOR (ilk kurulum)"
	else
		log "GoPulse GÜNCELLENİYOR (veriye dokunulmaz)"
	fi

	ensure_tool git
	ensure_tool curl
	ensure_go
	ensure_user
	fetch_source
	build_binary
	write_unit
	set_permissions
	start_service

	echo
	ok "Tamamlandı."
	echo "   Dinleme adresi : ${LISTEN_ADDR}"
	echo "   Veritabanı     : ${DB_PATH} (korundu)"
	echo "   Servis         : systemctl status ${SERVICE_NAME}"
	echo "   Günlükler      : journalctl -u ${SERVICE_NAME} -f"
	if [ "$COOKIE_SECURE" = "true" ]; then
		echo
		echo "   Not: GOPULSE_COOKIE_SECURE=true. GoPulse'u bir HTTPS reverse proxy"
		echo "   (nginx/Caddy) arkasında yayınlayın; aksi halde panele giriş yapılamaz."
	fi
	if [ "$first_install" = "yes" ]; then
		echo
		echo "   İlk kez: tarayıcıdan panele girip /setup ekranından yönetici"
		echo "   hesabını oluşturun."
	fi
}

main "$@"
