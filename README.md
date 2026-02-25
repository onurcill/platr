# Platr — gRPC Inspector

Ekip workspace'leri, environment yönetimi ve Kubernetes desteği olan gRPC inspector.

## Hızlı Başlangıç

**Gereksinim:** Docker Desktop

```bash
curl -sL https://raw.githubusercontent.com/onurcill/platr/main/docker-compose.local.yml -o docker-compose.yml
docker compose up -d
```

Tarayıcıda aç: **http://localhost:3000**

## Güncelleme

```bash
docker compose pull && docker compose up -d
```

## Geliştirme

```bash
# Backend
cd backend && go run .

# Frontend
cd frontend && npm install && npm run dev
```

## Deploy (Sunucu)

```bash
cp deploy/.env.example .env
# .env dosyasını düzenle
docker compose -f deploy/docker-compose.yml --env-file .env up -d
```
