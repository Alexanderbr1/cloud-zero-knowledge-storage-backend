#!/usr/bin/env bash
# Generates .env with random secrets for first-time setup.
# Run once before `docker compose up -d`.
set -euo pipefail

ENV_FILE=".env"

if [ -f "$ENV_FILE" ]; then
  echo "⚠️  $ENV_FILE already exists. Delete it first if you want to regenerate."
  exit 1
fi

gen() { openssl rand -base64 32 | tr -d '/+=\n' | head -c 40; }

JWT_SECRET=$(gen)
DB_PASSWORD=$(gen)
MINIO_ACCESS_KEY=$(gen | head -c 20)
MINIO_SECRET_KEY=$(gen)

cat > "$ENV_FILE" <<EOF
# Скопируй в .env и заполни значения.

# ── Postgres ──────────────────────────────────────────────────────────────────
DB_PASSWORD=${DB_PASSWORD}

# ── JWT ───────────────────────────────────────────────────────────────────────
JWT_SECRET=${JWT_SECRET}
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h

# ── Cookies ───────────────────────────────────────────────────────────────────
REFRESH_COOKIE_SECURE=true
REFRESH_COOKIE_SAMESITE=lax

# ── MinIO ─────────────────────────────────────────────────────────────────────
MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}
MINIO_SECRET_KEY=${MINIO_SECRET_KEY}
MINIO_BUCKET=blobs
MINIO_REGION=us-east-1
MINIO_PRESIGN_TTL=1h
MINIO_PUBLIC_ENDPOINT=https://s3.example.com
MINIO_SERVER_URL=https://s3.example.com
MINIO_API_CORS_ALLOW_ORIGIN=https://app.example.com

# ── Хранилище ─────────────────────────────────────────────────────────────────
MAX_UPLOAD_BYTES=0
STORAGE_QUOTA_BYTES=0
TRASH_TTL=720h

# ── Email (Resend) ─────────────────────────────────────────────────────────────
RESEND_API_KEY=
SMTP_FROM=noreply@example.com

# ── CORS ──────────────────────────────────────────────────────────────────────
FRONTEND_ORIGIN=https://app.example.com

# ── Прочее ────────────────────────────────────────────────────────────────────
LOG_LEVEL=info
EOF

chmod 600 "$ENV_FILE"

echo "✅  $ENV_FILE создан. Секреты сгенерированы автоматически."
echo ""
echo "Осталось заполнить вручную:"
echo "  MINIO_PUBLIC_ENDPOINT, MINIO_SERVER_URL — публичный URL MinIO"
echo "  MINIO_API_CORS_ALLOW_ORIGIN, FRONTEND_ORIGIN — домен фронтенда"
echo "  RESEND_API_KEY, SMTP_FROM — если нужны email-уведомления"
echo ""
echo "Затем: docker compose up -d"
