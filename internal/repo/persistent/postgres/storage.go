package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// Storage — Postgres-репозиторий для storage MVP.
type Storage struct {
	Pool *pgxpool.Pool
}

func NewStorage(pool *pgxpool.Pool) *Storage {
	return &Storage{Pool: pool}
}
