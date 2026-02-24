# Platr — Production Deployment Rehberi

Herhangi bir Linux sunucusunda **tek komutla** yayına alma rehberi.

---

## Gereksinimler

| | Minimum | Önerilen |
|---|---|---|
| CPU | 1 vCPU | 2 vCPU |
| RAM | 1 GB | 2 GB |
| Disk | 10 GB | 20 GB SSD |
| OS | Ubuntu 22.04 | Ubuntu 22.04 / Debian 12 |
| Domain | ✅ Zorunlu | — |

**Önerilen sunucular:** Hetzner CX21 (~€4/ay) · DigitalOcean Droplet · AWS EC2 t3.small

---

## Adım 1 — Sunucuya Docker Kurulumu

```bash
# Sunucuya SSH ile bağlanın
ssh root@SUNUCU_IP_ADRESI

# Docker otomatik kurulum scripti (Ubuntu / Debian)
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

---

## Adım 2 — Domain DNS Ayarı

Domain sağlayıcınızın panelinde **A kaydı** ekleyin:

| Tür | İsim | Değer |
|---|---|---|
| A | `@` | `SUNUCU_IP` |
| A | `platr` | `SUNUCU_IP` |

> DNS yayılması 5–30 dakika sürebilir.
> Kontrol: `dig +short platr.sirketiniz.com`

---

## Adım 3 — Projeyi Sunucuya Yükleme

```bash
# Sunucuda bir dizin oluşturun
mkdir -p /opt/platr && cd /opt/platr

# Bu zip'i sunucuya kopyalayın (yerel makinenizden):
scp platr-production.zip root@SUNUCU_IP:/opt/platr/
unzip platr-production.zip
```

Klasör yapısı şöyle olmalı:
```
/opt/platr/
├── grpc-inspector/
│   └── backend/         ← Go kaynak kodu
├── grpc-inspector-frontend/  ← React kaynak kodu
├── deploy/              ← Docker & nginx yapılandırması
├── deploy.sh            ← Deploy scripti
└── .env.example
```

---

## Adım 4 — Ortam Değişkenlerini Ayarlama

```bash
cp .env.example .env
nano .env
```

Doldurun:

```env
APP_DOMAIN=platr.sirketiniz.com
APP_URL=https://platr.sirketiniz.com
JWT_SECRET=           # ← openssl rand -base64 32 komutu ile üretin
```

### JWT_SECRET üretme:
```bash
openssl rand -base64 32
# Çıktıyı JWT_SECRET= alanına yapıştırın
```

---

## Adım 5 — Deploy! 🚀

```bash
chmod +x deploy.sh
./deploy.sh
```

Script otomatik olarak:
- Docker imajlarını derler (Go backend, React frontend)
- Tüm servisleri başlatır
- **Let's Encrypt SSL sertifikası** alır (domain ayarlıysa otomatik!)
- Sağlık kontrolü yapar

**İlk çalıştırma:** ~5-8 dakika (Go ve npm build süresi)

---

## Güncelleme

Yeni sürüm geldiğinde:
```bash
cd /opt/platr
./deploy.sh --update
```

---

## Servis Yönetimi

```bash
# Durum
docker compose -f deploy/docker-compose.yml ps

# Canlı loglar
docker compose -f deploy/docker-compose.yml logs -f

# Belirli servis logu
docker compose -f deploy/docker-compose.yml logs -f backend
docker compose -f deploy/docker-compose.yml logs -f caddy

# Yeniden başlatma
docker compose -f deploy/docker-compose.yml restart

# Durdurma
docker compose -f deploy/docker-compose.yml down
```

---

## Yedekleme

```bash
# Anlık veritabanı yedeği
docker compose -f deploy/docker-compose.yml exec backend \
  sqlite3 /data/platr.db .dump > backup_$(date +%Y%m%d_%H%M).sql
```

**Otomatik günlük yedek (cron):**
```bash
crontab -e
# Ekleyin:
0 3 * * * cd /opt/platr && docker compose -f deploy/docker-compose.yml exec -T backend sqlite3 /data/platr.db .dump > /opt/backups/platr_$(date +\%Y\%m\%d).sql 2>&1
```

---

## Firewall (Güvenlik Duvarı)

```bash
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP (HTTPS'e yönlendirilir)
ufw allow 443/tcp   # HTTPS
ufw allow 443/udp   # HTTP/3
ufw --force enable
```

---

## Mimari

```
İnternet (80/443)
       │
       ▼
  ┌─────────┐
  │  Caddy  │  ← Otomatik Let's Encrypt SSL
  └────┬────┘
       │ proxy
       ▼
  ┌──────────┐
  │ Frontend │  ← nginx: React SPA + /api proxy
  └────┬─────┘
       │ /api/*
       ▼
  ┌─────────┐
  │ Backend │  ← Go REST API (iç ağda, dışarıya kapalı)
  └────┬────┘
       │
       ▼
  ┌──────────┐
  │  SQLite  │  ← /data/platr.db (Docker volume)
  └──────────┘
```

---

## Sorun Giderme

**SSL sertifikası alınamıyor:**
```bash
# Domain'in sunucuya işaret ettiğini doğrulayın
dig +short platr.sirketiniz.com   # sunucu IP'nizi göstermeli

docker compose -f deploy/docker-compose.yml logs caddy
```

**Backend başlamıyor:**
```bash
docker compose -f deploy/docker-compose.yml logs backend
# Genellikle: JWT_SECRET eksik veya /data dizin izni sorunu
```

**Port 80/443 meşgul:**
```bash
ss -tlnp | grep -E '":80|":443'
# Eğer nginx/apache çalışıyorsa:
systemctl stop nginx apache2
```
