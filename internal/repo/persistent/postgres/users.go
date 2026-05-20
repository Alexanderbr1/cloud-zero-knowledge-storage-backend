package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

var _ authuc.UserRepository = (*Storage)(nil)

func (s *Storage) CreateUser(ctx context.Context, p authuc.NewUserParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, srp_salt, srp_verifier, bcrypt_salt, crypto_salt, public_key, encrypted_private_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.Email, p.SRPSalt, p.SRPVerifier, p.BcryptSalt, p.CryptoSalt,
		nullableBytes(p.PublicKey), nullableBytes(p.EncryptedPrivateKey),
	)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == pgerrcode.UniqueViolation {
			return authuc.ErrUserExists
		}
	}
	return err
}

func (s *Storage) GetByEmail(ctx context.Context, email string) (entity.User, bool, error) {
	var u entity.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, srp_salt, srp_verifier, bcrypt_salt, crypto_salt, public_key, encrypted_private_key
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.SRPSalt, &u.SRPVerifier, &u.BcryptSalt, &u.CryptoSalt,
		&u.PublicKey, &u.EncryptedPrivateKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.User{}, false, nil
	}
	if err != nil {
		return entity.User{}, false, err
	}
	return u, true, nil
}

func (s *Storage) GetCryptoSaltByUserID(ctx context.Context, userID uuid.UUID) ([]byte, error) {
	var salt []byte
	err := s.pool.QueryRow(ctx,
		`SELECT crypto_salt FROM users WHERE id = $1`, userID,
	).Scan(&salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

// nullableBytes returns nil for empty slices so nullable BYTEA columns store NULL
// instead of an empty byte array.
func nullableBytes(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}
