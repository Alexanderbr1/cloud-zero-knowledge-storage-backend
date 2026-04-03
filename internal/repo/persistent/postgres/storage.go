package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// Storage — Postgres-репозиторий (реализует UserRepository, SessionRepository, BlobRegistry).
type Storage struct {
	pool *pgxpool.Pool
}

func NewStorage(pool *pgxpool.Pool) *Storage {
	return &Storage{pool: pool}
}
