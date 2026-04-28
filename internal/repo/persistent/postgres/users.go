package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

var _ authuc.UserRepository = (*Storage)(nil)

func (s *Storage) CreateUser(ctx context.Context, p authuc.NewUserParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, srp_salt, srp_verifier, bcrypt_salt, crypto_salt)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.Email, p.SRPSalt, p.SRPVerifier, p.BcryptSalt, p.CryptoSalt,
	)
	return err
}

func (s *Storage) GetByEmail(ctx context.Context, email string) (entity.User, bool, error) {
	var u entity.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, srp_salt, srp_verifier, bcrypt_salt, crypto_salt
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.SRPSalt, &u.SRPVerifier, &u.BcryptSalt, &u.CryptoSalt)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.User{}, false, nil
	}
	if err != nil {
		return entity.User{}, false, err
	}
	return u, true, nil
}
