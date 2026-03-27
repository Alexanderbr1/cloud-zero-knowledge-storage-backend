# cloud-backend

MVP backend для **загрузки файлов в MinIO/S3**. На этом этапе API работает **без аутентификации и авторизации**: все пользователи видят общий список blob'ов и могут получать/удалять их по `blob_id`.

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
| `internal/usecase/storage/` | Presign PUT/GET в объектное хранилище |
| `internal/repo/persistent/postgres/` | Реализация репозиториев + миграции (pgx) |
| `internal/repo/storage/minio/` | MinIO (S3 API) |
| `migrations/` | SQL-миграции (golang-migrate, embed) |

Зависимости направлены внутрь: **controller → usecase → entity**; репозитории и `pkg` подключаются из `app`.

**Слои:** формат и ограничения входа (UUID, длина `Content-Type` и т.д.) — в **контроллере** (`dto` + `ValidateStruct` + хендлеры). В **use case** остаются правила предметной области: presign URL, операции с метаданными blob'ов и удаление без повторной «транспортной» валидации.

**Интерфейсы** объявляются у **потребителя**: `ObjectStore` / `BlobRegistry` находятся в `internal/usecase/storage/service.go`.

## Запуск

```bash
export DATABASE_URL=postgres://...
# опционально: DB_INIT=true для применения миграций при старте

# MinIO (если задан MINIO_ENDPOINT — поднимаются эндпоинты /v1/storage/*)
export MINIO_ENDPOINT=127.0.0.1:9000
export MINIO_ACCESS_KEY=minioadmin
export MINIO_SECRET_KEY=minioadmin
export MINIO_BUCKET=blobs

go run ./cmd/app
```

### API хранилища (без авторизации)

| Метод | Путь | Описание |
|--------|------|----------|
| `POST` | `/v1/storage/presign` | Тело: `{"content_type":"application/octet-stream"}` (можно опустить). Ответ: `upload_url`, `blob_id`, `object_key`. Клиент шлёт **PUT** на `upload_url` с телом файла. |
| `GET` | `/v1/storage/blobs` | Получение списка всех blob'ов в системе (общий для всех пользователей). |
| `POST` | `/v1/storage/blobs/{blob_id}/presign-get` | Подписанный **GET** для скачивания файла из MinIO/S3. Ответ: `download_url`, `expires_in`, `content_type`. |
| `DELETE` | `/v1/storage/blobs/{blob_id}` | Удаление объекта в MinIO и записи в БД. **204** при успехе, **404** если blob не существует. |

Переменные MinIO (все опциональны, кроме ключа/секрета при непустом endpoint):  
`MINIO_USE_SSL`, `MINIO_REGION`, `MINIO_PRESIGN_TTL` (например `1h`).

Пример MinIO в Docker:

```bash
docker run -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data --console-address ":9001"
```

Создайте bucket `blobs` (или имя из `MINIO_BUCKET`) в консоли `http://127.0.0.1:9001`, либо включите `DB_INIT=true` — приложение создаст bucket при старте, если его ещё нет.

## Сборка

```bash
go build -o bin/app ./cmd/app
```
