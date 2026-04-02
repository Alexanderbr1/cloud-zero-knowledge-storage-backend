# cloud-backend

Backend для загрузки файлов в MinIO/S3: **JWT (email + пароль)** и presigned PUT/GET. Объекты в бакете лежат по пути **`blobs/<user_id>/<blob_id>`**; в БД дополнительно хранится человекочитаемое `file_name` для списка.

## Структура

| Путь | Назначение |
|------|------------|
| `cmd/app/main.go` | Точка входа: конфиг, логирование, `app.Run` |
| `config/` | Загрузка конфигурации из env (twelve-factor) |
| `internal/app/` | Composition root: DI, миграции, сборка HTTP |
| `internal/controller/restapi/` | REST delivery (middleware, JSON, validator) |
| `internal/controller/restapi/v1/` | Маршруты API v1 |
| `internal/controller/restapi/v1/dto/` | JSON-модели запросов/ответов (`json` + `validate`) |
| `internal/entity/` | Доменные сущности |
| `internal/usecase/auth/` | Регистрация, вход, refresh/logout |
| `internal/usecase/storage/` | Presign PUT/GET в объектное хранилище |
| `internal/repo/persistent/postgres/` | Реализация репозиториев + миграции (pgx) |
| `internal/repo/storage/minio/` | MinIO (S3 API) |
| `pkg/jwt/` | Подпись и проверка access JWT (HS256) |
| `migrations/` | SQL-миграции (golang-migrate, embed) |

## Запуск

```bash
export DATABASE_URL=postgres://...
export JWT_SECRET=...   # обязательно

# опционально: DB_INIT=true для применения миграций при старте

# MinIO (если задан MINIO_ENDPOINT — поднимаются эндпоинты /v1/storage/*)
export MINIO_ENDPOINT=127.0.0.1:9000
export MINIO_PUBLIC_ENDPOINT=http://localhost:9000   # хост в presigned URL для браузера
export MINIO_ACCESS_KEY=minioadmin
export MINIO_SECRET_KEY=minioadmin
export MINIO_BUCKET=blobs

go run ./cmd/app
```

### Аутентификация (JWT)

| Метод | Путь | Описание |
|--------|------|----------|
| `POST` | `/v1/auth/register` | Тело: `{"email":"...","password":"..."}` (пароль min 8 символов). **201** + JSON с access и **Set-Cookie** с refresh (см. ниже). |
| `POST` | `/v1/auth/login` | Тело: `{"email":"...","password":"..."}`. **200** + JSON с access и **Set-Cookie** с refresh. |
| `POST` | `/v1/auth/refresh` | Тело не нужно: refresh берётся из **HttpOnly**-куки (имя по умолчанию `refresh_token`). **200** + новый access и новая кука (ротация). **401** — нет/невалидный refresh (кука сбрасывается). |
| `POST` | `/v1/auth/logout` | Refresh из куки; **204** — кука очищается, сессия отзывается при наличии валидного токена (идемпотентно). |

Ответ register/login/refresh в JSON: `access_token`, `expires_in` (секунды access), `refresh_expires_in` (секунды жизни refresh), `token_type`: `Bearer`. Сам **refresh-токен в JSON не отдаётся** — только в куке `HttpOnly`; при HTTPS включайте `REFRESH_COOKIE_SECURE=true`.

Переменные: `JWT_ACCESS_TTL` (например `15m`), `JWT_REFRESH_TTL` (например `720h`), опционально кука: `REFRESH_COOKIE_NAME`, `REFRESH_COOKIE_PATH`, `REFRESH_COOKIE_DOMAIN`, `REFRESH_COOKIE_SECURE`, `REFRESH_COOKIE_SAMESITE` (`lax` \| `strict` \| `none`; для `none` обязателен `REFRESH_COOKIE_SECURE=true`). Для SPA на **другом origin** нужны `fetch`/`axios` с `credentials: 'include'` и CORS с конкретным `Access-Control-Allow-Origin` и `Access-Control-Allow-Credentials: true`.

Дальше для `/v1/storage/*` нужен заголовок:

`Authorization: Bearer <access_token>`

### API хранилища (только с JWT)

| Метод | Путь | Описание |
|--------|------|----------|
| `POST` | `/v1/storage/presign` | Тело: `{"file_name":"report.pdf"}`. Сервер **не хранит MIME-тип**; ответ: `upload_url`, `blob_id`, `object_key` (`blobs/<user_id>/<blob_id>`). Клиент делает **PUT** на `upload_url` с телом (часто `application/octet-stream` для ciphertext). |
| `GET` | `/v1/storage/blobs` | Список файлов **текущего пользователя**. |
| `POST` | `/v1/storage/blobs/{blob_id}/presign-get` | Временная ссылка на скачивание из MinIO/S3. |
| `DELETE` | `/v1/storage/blobs/{blob_id}` | Удаление объекта и метаданных (**только свой** blob). **404**, если чужой или нет записи. |

Переменные MinIO: `MINIO_USE_SSL`, `MINIO_REGION`, `MINIO_PRESIGN_TTL` (например `1h`).

Пример MinIO в Docker:

```bash
docker run -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data --console-address ":9001"
```

Создайте bucket `blobs` (или имя из `MINIO_BUCKET`) в консоли `http://127.0.0.1:9001`, либо включите `DB_INIT=true` — приложение создаст bucket при старте, если его ещё нет.

## Миграции

По одной паре файлов на таблицу (порядок важен из‑за FK):

| Версия | Файлы | Таблица |
|--------|--------|---------|
| `1` | `000001_users.*` | `users` |
| `2` | `000002_stored_blobs.*` | `stored_blobs` (без `content_type`) |
| `3` | `000003_refresh_sessions.*` | `refresh_sessions` |

Если база уже создавалась **другой** историей миграций, пересоздайте volume Postgres или очистите БД, иначе `migrate` может конфликтовать с `schema_migrations`.

## Сборка

```bash
go build -o bin/app ./cmd/app
```
