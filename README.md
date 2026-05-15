# EncryptDrive

End-to-end encrypted file storage. Files are encrypted in the browser before upload — the server never sees plaintext.

## Self-Hosted Setup

### Requirements

- Docker + Docker Compose
- A domain pointing to your server (A record → server IP)
- Ports 80 and 443 open

### Quick Start

```bash
# 1. Generate .env with random secrets
cd cloud-backend
./setup.sh

# 2. Set your public URL in .env
#    MINIO_PUBLIC_ENDPOINT=https://drive.example.com
#    FRONTEND_ORIGIN=https://drive.example.com
#    MINIO_API_CORS_ALLOW_ORIGIN=https://drive.example.com
#    MINIO_SERVER_URL=https://drive.example.com

# 3. Set your domain in cloud-frontend/.env
echo "DOMAIN=drive.example.com" > ../cloud-frontend/.env

# 4. Start the backend
docker compose up -d

# 5. Start the frontend (with Caddy for automatic HTTPS)
cd ../cloud-frontend
docker compose up -d
```

Caddy automatically obtains a Let's Encrypt certificate on first start.

### Key Environment Variables

| Variable | Default | Description |
|---|---|---|
| `MINIO_PUBLIC_ENDPOINT` | — | **Required.** Public HTTPS URL, e.g. `https://drive.example.com` |
| `FRONTEND_ORIGIN` | — | **Required.** Same as above — used for CORS |
| `REGISTRATION_ENABLED` | `true` | Set to `false` to prevent new signups |
| `MAX_UPLOAD_BYTES` | `0` | Max file size in bytes (0 = unlimited) |
| `STORAGE_QUOTA_BYTES` | `0` | Storage limit per user in bytes (0 = unlimited) |
| `RESEND_API_KEY` | — | Optional. Resend key for email notifications |
| `REFRESH_COOKIE_SECURE` | `false` | Set to `true` when serving over HTTPS |

### Disabling Registration

After creating your accounts:

```bash
# In cloud-backend/.env
REGISTRATION_ENABLED=false
```

Then restart: `docker compose up -d`

### Health Check

```
GET /v1/health  →  {"status":"ok"}
```

### Updating

```bash
# Backend
cd cloud-backend
git pull
docker compose up -d --build

# Frontend
cd cloud-frontend
git pull
docker compose up -d --build
```

Migrations run automatically on startup.

### Backup

```bash
cd cloud-backend
./backup.sh           # saves to ./backups/
./backup.sh /mnt/nas  # or specify a custom path
```

This creates two timestamped files:
- `db_YYYYMMDD_HHMMSS.sql.gz` — PostgreSQL dump
- `minio_YYYYMMDD_HHMMSS.tar.gz` — file storage

### Limits

Set in `cloud-backend/.env`:

```bash
# 2 GB per file
MAX_UPLOAD_BYTES=2147483648

# 50 GB per user
STORAGE_QUOTA_BYTES=53687091200
```

Errors returned: `413 Request Entity Too Large` for file limit, `402 Payment Required` for quota.

---

## Architecture Overview

Files are encrypted client-side (AES-256-GCM) before upload. The server stores only ciphertext; encryption keys are derived from the user's password via SRP and never leave the browser.

```
Browser → nginx/Caddy → Backend API → PostgreSQL (metadata)
                      ↘ MinIO (encrypted blobs, via presigned URLs)
```

### Backend Structure

| Path | Purpose |
|---|---|
| `cmd/app/main.go` | Entry point |
| `config/` | Environment-based config (twelve-factor) |
| `internal/app/` | Dependency wiring |
| `internal/controller/restapi/v1/` | HTTP handlers |
| `internal/usecase/` | Business logic (auth, storage, sharing) |
| `internal/repo/` | PostgreSQL and MinIO adapters |
| `migrations/` | SQL migrations (auto-applied on start) |

### API

Auth: `POST /v1/auth/register`, `/v1/auth/login/init`, `/v1/auth/login/finalize`, `/v1/auth/refresh`, `/v1/auth/logout`

Storage (requires `Authorization: Bearer <token>`):
- `POST /v1/storage/presign` — get upload URL
- `GET /v1/storage/blobs` — list files
- `POST /v1/storage/blobs/{id}/presign-get` — get download URL
- `DELETE /v1/storage/blobs/{id}` — move to trash
- `GET /v1/trash/` — list trash
- `POST /v1/trash/blobs/{id}/restore` — restore from trash

### Database

| Migration | Table |
|---|---|
| 1 | `users` |
| 2 | `stored_blobs` |
| 3 | `refresh_sessions` |
| 4 | Constraints and indexes |
| 5 | `stored_blobs.content_type` |
| 6 | `folders` |
| 7 | `file_shares` |
| 8+ | Trash, search, sharing |
