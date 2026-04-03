package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

var _ authuc.UserRepository = (*Storage)(nil)

func (s *Storage) CreateUser(ctx context.Context, id uuid.UUID, email, passwordHash string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		id, email, passwordHash,
	)
	return err
}

func (s *Storage) GetByEmail(ctx context.Context, email string) (entity.User, bool, error) {
	var u entity.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.User{}, false, nil
	}
	if err != nil {
		return entity.User{}, false, err
	}
	return u, true, nil
}
