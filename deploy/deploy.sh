#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════════════════
# Platr Production Deploy Script
#
# KULLANIM — proje root'undan (grpc-inspector ve grpc-inspector-frontend
#            klasörlerinin bulunduğu dizin):
#
#   ./deploy/deploy.sh           → ilk kurulum / yeniden build
#   ./deploy/deploy.sh --update  → git pull + yeniden build
# ═══════════════════════════════════════════════════════════════════════════════
set -euo pipefail

# Script nerede olursa olsun proje root'una git (deploy/'ın bir üst dizini)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$ROOT"

GRN='\033[0;32m'; YLW='\033[1;33m'; RED='\033[0;31m'; BLD='\033[1m'; NC='\033[0m'
ok()   { echo -e "${GRN}✔  $*${NC}"; }
info() { echo -e "${BLD}▶  $*${NC}"; }
warn() { echo -e "${YLW}⚠   $*${NC}"; }
die()  { echo -e "${RED}✗  $*${NC}" >&2; exit 1; }

UPDATE="${1:-}"
COMPOSE="docker compose -f deploy/docker-compose.yml --env-file .env"

# ── 1. Bağımlılık kontrolü ───────────────────────────────────────────────────
info "Gereksinimler kontrol ediliyor…"
command -v docker >/dev/null 2>&1      || die "Docker bulunamadı → https://docs.docker.com/get-docker"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 gerekli"
ok "Docker $(docker --version | awk '{print $3}' | tr -d ',')"

# ── 2. Kaynak dizin kontrolü ─────────────────────────────────────────────────
[ -d "grpc-inspector/backend" ]  || die "'grpc-inspector/backend' bulunamadı (bu scripti proje root'undan çalıştırın)"
[ -d "grpc-inspector-frontend" ] || die "'grpc-inspector-frontend' bulunamadı"
ok "Kaynak dizinler mevcut"

# ── 3. .env yükleme ve doğrulama ─────────────────────────────────────────────
if [ ! -f ".env" ]; then
    warn ".env bulunamadı — örnek dosyadan kopyalanıyor…"
    cp deploy/.env.example .env
    echo ""
    echo -e "${BLD}📝  .env dosyasını düzenleyin, sonra tekrar çalıştırın:${NC}"
    echo ""
    echo "    nano .env"
    echo ""
    echo "    Zorunlu alanlar:"
    echo "      APP_DOMAIN  →  platr.sirketiniz.com"
    echo "      APP_URL     →  https://platr.sirketiniz.com"
    echo "      JWT_SECRET  →  openssl rand -base64 32"
    echo ""
    echo "    ./deploy/deploy.sh"
    echo ""
    exit 0
fi

set -a; source .env; set +a

[[ "${JWT_SECRET:-BURAYA}" == "BURAYA"* ]] && \
    die "JWT_SECRET değiştirilmemiş! Çalıştırın: openssl rand -base64 32"
[[ -z "${APP_DOMAIN:-}" ]] && die "APP_DOMAIN boş! .env dosyasına domain adınızı girin."
[[ -z "${APP_URL:-}"    ]] && die "APP_URL boş! Örn: APP_URL=https://${APP_DOMAIN}"
ok ".env geçerli (${APP_DOMAIN})"

# ── 4. Güncelleme modu ───────────────────────────────────────────────────────
if [ "$UPDATE" = "--update" ]; then
    info "Kaynak kod güncelleniyor (git pull)…"
    git pull --ff-only 2>/dev/null || warn "git pull başarısız — mevcut kodla devam"
fi

# ── 5. Build ─────────────────────────────────────────────────────────────────
info "Docker imajları derleniyor (ilk seferde 5-10 dk sürebilir)…"
$COMPOSE build

# ── 6. Başlatma ──────────────────────────────────────────────────────────────
info "Servisler başlatılıyor…"
$COMPOSE up -d --remove-orphans

# ── 7. Sağlık kontrolü ───────────────────────────────────────────────────────
info "Backend yanıt vermesi bekleniyor…"
for i in $(seq 1 30); do
    if $COMPOSE exec -T backend wget -qO- http://localhost:8080/api/billing/plans >/dev/null 2>&1; then
        ok "Backend hazır"
        break
    fi
    [ $i -eq 30 ] && die "Backend 60s içinde yanıt vermedi.\n  Loglar: $COMPOSE logs backend"
    sleep 2
done

# ── 8. Başarı ────────────────────────────────────────────────────────────────
echo ""
echo -e "${GRN}${BLD}╔═════════════════════════════════════════════════╗${NC}"
echo -e "${GRN}${BLD}║  🚀  Platr başarıyla yayınlandı!                ║${NC}"
echo -e "${GRN}${BLD}╚═════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  🌐  ${BLD}https://${APP_DOMAIN}${NC}"
echo ""
echo "  Durum:      $COMPOSE ps"
echo "  Loglar:     $COMPOSE logs -f"
echo "  Güncelle:   ./deploy/deploy.sh --update"
echo "  Durdur:     $COMPOSE down"
echo ""
echo "  💾 Veritabanı yedeği:"
echo "     $COMPOSE exec backend sqlite3 /data/platr.db .dump > backup_\$(date +%Y%m%d).sql"
echo ""
