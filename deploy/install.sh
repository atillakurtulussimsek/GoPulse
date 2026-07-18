#!/usr/bin/env bash
#
# GoPulse — kurulum ve güncelleme script'i
#
# Ne yapar:
#   - Gerekli araçları (git, curl) ve Go'yu (yoksa/eskiyse) hazırlar
#   - Projeyi GitHub'dan çeker (veya mevcut kopyayı günceller)
#   - CGO'suz statik binary'yi derler
#   - Port çakışmasını kontrol eder; doluysa terminalde bir menü sunar
#     (otomatik boş port / manuel port / iptal). Terminal yoksa durur ve
#     öneri verir; GOPULSE_AUTO_PORT=true ile otomatik seçer
#   - Tercihleri kalıcı bir dosyaya (/etc/gopulse/gopulse.env) kaydeder ve
#     sonraki çalıştırmalarda hatırlar
#   - Bir systemd servisi olarak kurar ve başlatır
#
# Zaten kuruluysa ne yapılacağını sorar (terminalde menü, ya da GOPULSE_ACTION):
#   update    → yalnızca kodu günceller; veri ve ayarlar korunur (varsayılan)
#   reinstall → SIFIRDAN kurar; veri/ayar/kod tümüyle silinir (onay ister)
#   uninstall → servisi ve dosyaları kaldırır (veritabanı isteğe bağlı silinir)
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

SERVICE_NAME="gopulse"
UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

# Kalıcı yapılandırma dosyası. Tercihler burada saklanır ve systemd bu dosyayı
# EnvironmentFile ile okur. Sonraki çalıştırmalarda buradan hatırlanır.
ENV_FILE="${GOPULSE_ENV_FILE:-/etc/gopulse/gopulse.env}"

# Komut satırından AÇIKÇA verilen değerleri yakala (kayıtlı .env'den önceliklidir).
CLI_LISTEN_ADDR="${GOPULSE_LISTEN_ADDR:-}"
CLI_COOKIE_SECURE="${GOPULSE_COOKIE_SECURE:-}"
CLI_DB_PATH="${GOPULSE_DB_PATH:-}"

# Daha önce kaydedilmiş tercihleri yükle ("hatırla").
if [ -f "$ENV_FILE" ]; then
	# shellcheck disable=SC1090
	. "$ENV_FILE"
fi

# Öncelik: komut satırı > kayıtlı .env > varsayılan.
LISTEN_ADDR="${CLI_LISTEN_ADDR:-${GOPULSE_LISTEN_ADDR:-127.0.0.1:8080}}"
COOKIE_SECURE="${CLI_COOKIE_SECURE:-${GOPULSE_COOKIE_SECURE:-true}}"
DB_PATH="${CLI_DB_PATH:-${GOPULSE_DB_PATH:-${DATA_DIR}/gopulse.db}}"

# Port doluysa ve terminal yoksa (cron/CI) otomatik boş port seçimi.
# İnteraktif terminalde bunun yerine bir menü sunulur.
AUTO_PORT="${GOPULSE_AUTO_PORT:-false}"

# İnteraktif menü tamamen kapatılmak istenirse: GOPULSE_NONINTERACTIVE=true
NONINTERACTIVE="${GOPULSE_NONINTERACTIVE:-false}"

# Kuruluysa yapılacak işlem: update | reinstall | uninstall.
# Boş bırakılırsa (ve terminal varsa) kullanıcıya sorulur.
ACTION="${GOPULSE_ACTION:-}"

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

# Bir TCP portunun dinlenip dinlenmediğini kontrol eder (ss → lsof → /dev/tcp).
port_in_use() {
	local p="$1"
	if command -v ss >/dev/null 2>&1; then
		ss -Htln "sport = :$p" 2>/dev/null | grep -q . && return 0 || return 1
	elif command -v lsof >/dev/null 2>&1; then
		lsof -iTCP:"$p" -sTCP:LISTEN -t >/dev/null 2>&1 && return 0 || return 1
	else
		(exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null && { exec 3>&- 3<&-; return 0; } || return 1
	fi
}

# Verilen porttan başlayarak ilk boş portu döndürür.
find_free_port() {
	local start="$1" p
	for p in $(seq "$start" $((start + 50))); do
		port_in_use "$p" || { echo "$p"; return 0; }
	done
	return 1
}

# İnteraktif bir terminale (kontrol eden tty) erişilebilir mi? curl|bash ile
# çalıştırılsa bile gerçek bir terminal varsa /dev/tty üzerinden soru sorulabilir.
tty_available() {
	[ "$NONINTERACTIVE" != "true" ] || return 1
	[ -e /dev/tty ] || return 1
	( : >/dev/tty ) 2>/dev/null || return 1
	return 0
}

# LISTEN_ADDR'i host'u koruyarak yeni portla günceller.
apply_port() {
	local p="$1"
	if [ -z "$PORT_HOST" ]; then LISTEN_ADDR=":$p"; else LISTEN_ADDR="${PORT_HOST}:${p}"; fi
	ok "Port ayarlandı: ${LISTEN_ADDR}"
}

# Port dolu iken kullanıcıya menü sunar (otomatik / manuel / iptal).
choose_port_interactive() {
	local port="$1" suggestion="$2" choice np
	{
		echo
		echo "Port ${port} kullanımda. Ne yapmak istersiniz?"
		if [ -n "$suggestion" ]; then
			echo "  1) Otomatik boş port kullan (${suggestion})"
		else
			echo "  1) Otomatik boş port kullan (yakında boş port yok)"
		fi
		echo "  2) Manuel port gir"
		echo "  3) İptal"
		printf "Seçiminiz [1]: "
	} >/dev/tty
	read -r choice </dev/tty || choice=""
	choice="${choice:-1}"

	case "$choice" in
		1)
			[ -n "$suggestion" ] || die "Boş port bulunamadı; manuel port deneyin."
			apply_port "$suggestion"
			;;
		2)
			while true; do
				printf "Port numarası (1-65535): " >/dev/tty
				read -r np </dev/tty || np=""
				if ! printf '%s' "$np" | grep -qE '^[0-9]+$' || [ "$np" -lt 1 ] || [ "$np" -gt 65535 ]; then
					echo "  Geçersiz port numarası." >/dev/tty; continue
				fi
				if port_in_use "$np"; then
					echo "  ${np} de kullanımda, başka bir port deneyin." >/dev/tty; continue
				fi
				apply_port "$np"; break
			done
			;;
		3) die "Kurulum iptal edildi." ;;
		*) echo "  Geçersiz seçim." >/dev/tty; choose_port_interactive "$port" "$suggestion" ;;
	esac
}

# Yapılandırılan portun uygunluğunu doğrular. Kendi servisimiz güncelleniyorsa
# (portu zaten biz tutuyoruz) çakışma sayılmaz. Doluysa: env ile otomatik,
# terminalde interaktif menü, aksi halde durup öneri sunar.
check_port() {
	PORT_PORT="${LISTEN_ADDR##*:}"
	if [ "$LISTEN_ADDR" = ":$PORT_PORT" ]; then PORT_HOST=""; else PORT_HOST="${LISTEN_ADDR%:*}"; fi

	# Güncelleme: gopulse servisi zaten çalışıyorsa portu biz tutuyoruzdur.
	if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
		ok "Port kontrolü atlandı (gopulse servisi zaten çalışıyor — güncelleme)."
		return
	fi

	if ! port_in_use "$PORT_PORT"; then
		ok "Port uygun: ${LISTEN_ADDR}"
		return
	fi

	local suggestion
	suggestion="$(find_free_port "$PORT_PORT" || true)"

	# 1) Açık env tercihi: otomatik boş port.
	if [ "$AUTO_PORT" = "true" ] && [ -n "$suggestion" ]; then
		apply_port "$suggestion"
		warn "Port $PORT_PORT dolu; GOPULSE_AUTO_PORT=true → otomatik seçildi."
		return
	fi

	# 2) İnteraktif terminal: kullanıcıya sor.
	if tty_available; then
		choose_port_interactive "$PORT_PORT" "$suggestion"
		return
	fi

	# 3) Non-interactive (cron/CI, tty yok): dur ve öner.
	printf '\033[1;31mHATA:\033[0m %s\n' "Port $PORT_PORT kullanımda (başka bir process tutuyor)." >&2
	if [ -n "$suggestion" ]; then
		echo "  Boş bir port: $suggestion" >&2
		echo "  Bu portla tekrar çalıştırın:" >&2
		echo "    sudo GOPULSE_LISTEN_ADDR=${PORT_HOST:-127.0.0.1}:${suggestion} bash install.sh" >&2
		echo "  veya otomatik seçim için:" >&2
		echo "    sudo GOPULSE_AUTO_PORT=true bash install.sh" >&2
	else
		echo "  Yakında boş port bulunamadı; farklı bir port belirtin." >&2
	fi
	exit 1
}

# Yönetilen bir anahtarı .env dosyasına yazar/günceller (diğer satırlar korunur).
set_env_kv() {
	local key="$1" val="$2"
	if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
		sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
	else
		printf '%s=%s\n' "$key" "$val" >> "$ENV_FILE"
	fi
}

# Seçilen tercihleri kalıcı .env dosyasına kaydeder ("hatırla").
save_config() {
	mkdir -p "$(dirname "$ENV_FILE")"
	touch "$ENV_FILE"
	set_env_kv GOPULSE_LISTEN_ADDR   "$LISTEN_ADDR"
	set_env_kv GOPULSE_DB_PATH       "$DB_PATH"
	set_env_kv GOPULSE_COOKIE_SECURE "$COOKIE_SECURE"
	chmod 600 "$ENV_FILE"
	ok "Yapılandırma kaydedildi: $ENV_FILE"
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

# Ayarlar kalıcı yapılandırma dosyasından okunur (script tarafından yönetilir).
EnvironmentFile=${ENV_FILE}

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

# GoPulse sistemde kurulu mu? (kaynak, birim veya binary'den biri yeterli)
is_installed() {
	[ -d "${SRC_DIR}/.git" ] || [ -f "$UNIT_PATH" ] || [ -x "$BIN_PATH" ]
}

# Kurulu bir sistemde yapılacak işlemi belirler: update | reinstall | uninstall.
# Öncelik: GOPULSE_ACTION env > interaktif menü > (tty yoksa) güvenli 'update'.
choose_action() {
	if ! is_installed; then
		ACTION="install"
		return
	fi
	if [ -n "${ACTION:-}" ]; then
		case "$ACTION" in
			update|reinstall|uninstall) return ;;
			*) die "Geçersiz GOPULSE_ACTION: '$ACTION' (update|reinstall|uninstall)" ;;
		esac
	fi
	if tty_available; then
		{
			echo
			echo "GoPulse zaten kurulu. Ne yapmak istersiniz?"
			echo "  1) Güncelle    (kodu yenile, verileri koru)"
			echo "  2) Yeniden kur (SIFIRDAN — tüm veriler ve ayarlar silinir)"
			echo "  3) Kaldır      (servisi ve dosyaları kaldır)"
			echo "  4) İptal"
			printf "Seçiminiz [1]: "
		} >/dev/tty
		local c; read -r c </dev/tty || c=""
		case "${c:-1}" in
			1) ACTION="update" ;;
			2) ACTION="reinstall" ;;
			3) ACTION="uninstall" ;;
			4) die "İşlem iptal edildi." ;;
			*) die "Geçersiz seçim." ;;
		esac
	else
		ACTION="update"
		warn "Etkileşimsiz mod: varsayılan işlem 'güncelle'."
		warn "Değiştirmek için: GOPULSE_ACTION=update|reinstall|uninstall"
	fi
}

# Yıkıcı (geri alınamaz) işlemler için açık onay ister.
confirm_destructive() {
	local what="$1"
	[ "${GOPULSE_ASSUME_YES:-false}" = "true" ] && return
	if tty_available; then
		{
			echo
			echo "!!! DİKKAT: Bu işlem ${what} KALICI olarak siler (geri alınamaz)."
			printf "Devam etmek için 'evet' yazın: "
		} >/dev/tty
		local a; read -r a </dev/tty || a=""
		[ "$a" = "evet" ] || die "Onaylanmadı — iptal edildi."
	else
		die "Yıkıcı işlem için onay gerekli. Etkileşimsiz modda GOPULSE_ASSUME_YES=true verin."
	fi
}

# GoPulse'u tümüyle kaldırır. Veritabanı yalnızca açıkça istenirse silinir.
do_uninstall() {
	log "GoPulse KALDIRILIYOR"

	if command -v systemctl >/dev/null 2>&1; then
		systemctl stop "$SERVICE_NAME" 2>/dev/null || true
		systemctl disable "$SERVICE_NAME" 2>/dev/null || true
		rm -f "$UNIT_PATH"
		systemctl daemon-reload 2>/dev/null || true
	fi
	rm -rf "$INSTALL_DIR"
	rm -f "$ENV_FILE"
	rmdir "$(dirname "$ENV_FILE")" 2>/dev/null || true
	ok "Servis, binary, kaynak ve ayar dosyaları kaldırıldı."

	# Veritabanı: varsayılan olarak KORUNUR; istenirse silinir.
	local purge="${GOPULSE_PURGE_DATA:-}"
	if [ -z "$purge" ]; then
		if tty_available; then
			printf "Veritabanı da silinsin mi? (%s) [e/H]: " "$DATA_DIR" >/dev/tty
			local a; read -r a </dev/tty || a=""
			case "$a" in [eE]*) purge="true" ;; *) purge="false" ;; esac
		else
			purge="false"
		fi
	fi
	if [ "$purge" = "true" ]; then
		rm -rf "$DATA_DIR"
		ok "Veritabanı silindi: $DATA_DIR"
	else
		ok "Veritabanı korundu: $DATA_DIR"
	fi

	echo
	ok "GoPulse kaldırıldı."
	echo "   Not: '$APP_USER' sistem kullanıcısı bırakıldı."
	echo "   Silmek için: sudo userdel $APP_USER"
	exit 0
}

# Sıfırdan yeniden kurulum: TÜM veri, ayar ve kod silinir, sonra taze kurulur.
do_reinstall() {
	log "GoPulse YENİDEN KURULUYOR (sıfırdan)"
	confirm_destructive "veritabanı (${DATA_DIR}) ve tüm ayarlar dâhil mevcut kurulumu"

	command -v systemctl >/dev/null 2>&1 && { systemctl stop "$SERVICE_NAME" 2>/dev/null || true; }
	rm -rf "$INSTALL_DIR" "$DATA_DIR"
	rm -f "$ENV_FILE"
	ok "Eski kurulum tümüyle temizlendi."

	# "Sıfırdan": kayıtlı ayarları yok say, varsayılanlara dön (CLI hâlâ önceliklidir).
	LISTEN_ADDR="${CLI_LISTEN_ADDR:-127.0.0.1:8080}"
	COOKIE_SECURE="${CLI_COOKIE_SECURE:-true}"
	DB_PATH="${CLI_DB_PATH:-${DATA_DIR}/gopulse.db}"
}

# ----------------------------------------------------------------------------
# Akış
# ----------------------------------------------------------------------------
main() {
	require_root
	choose_action

	case "$ACTION" in
		uninstall) do_uninstall ;;   # kendi içinde çıkar
		reinstall) do_reinstall ;;   # temizler, ardından taze kuruluma düşer
		update)    log "GoPulse GÜNCELLENİYOR (veri ve ayarlar korunur)" ;;
		install)   log "GoPulse KURULUYOR (ilk kurulum)" ;;
	esac

	ensure_tool git
	ensure_tool curl
	ensure_go
	ensure_user
	fetch_source
	build_binary
	check_port
	save_config
	write_unit
	set_permissions
	start_service

	echo
	ok "Tamamlandı."
	echo "   İşlem          : ${ACTION}"
	echo "   Dinleme adresi : ${LISTEN_ADDR}"
	if [ "$ACTION" = "update" ]; then
		echo "   Veritabanı     : ${DB_PATH} (korundu)"
	else
		echo "   Veritabanı     : ${DB_PATH}"
	fi
	echo "   Ayar dosyası   : ${ENV_FILE}"
	echo "   Servis         : systemctl status ${SERVICE_NAME}"
	echo "   Günlükler      : journalctl -u ${SERVICE_NAME} -f"
	if [ "$COOKIE_SECURE" = "true" ]; then
		echo
		echo "   Not: GOPULSE_COOKIE_SECURE=true. GoPulse'u bir HTTPS reverse proxy"
		echo "   (nginx/Caddy) arkasında yayınlayın; aksi halde panele giriş yapılamaz."
	fi
	if [ "$ACTION" = "install" ] || [ "$ACTION" = "reinstall" ]; then
		echo
		echo "   İlk kez: tarayıcıdan panele girip /setup ekranından yönetici"
		echo "   hesabını oluşturun."
	fi
}

main "$@"
