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

func (s *Storage) CreateUser(ctx context.Context, p authuc.NewUserParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users
		 (id, email, srp_salt, srp_verifier, bcrypt_salt, crypto_salt, public_key, encrypted_private_key,
		  kek_encrypted_master, kek_encrypted_recovery, recovery_salt)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		p.ID, p.Email, p.SRPSalt, p.SRPVerifier, p.BcryptSalt, p.CryptoSalt,
		nullableBytes(p.PublicKey), nullableBytes(p.EncryptedPrivateKey),
		nullableBytes(p.KEKEncryptedMaster), nullableBytes(p.KEKEncryptedRecovery), nullableBytes(p.RecoverySalt),
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
		`SELECT id, email, srp_salt, srp_verifier, bcrypt_salt, crypto_salt, public_key, encrypted_private_key,
		        kek_encrypted_master, kek_encrypted_recovery, recovery_salt
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.SRPSalt, &u.SRPVerifier, &u.BcryptSalt, &u.CryptoSalt,
		&u.PublicKey, &u.EncryptedPrivateKey,
		&u.KEKEncryptedMaster, &u.KEKEncryptedRecovery, &u.RecoverySalt)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.User{}, false, nil
	}
	if err != nil {
		return entity.User{}, false, err
	}
	return u, true, nil
}

func (s *Storage) GetRecoveryDataByUserID(ctx context.Context, userID uuid.UUID) (authuc.RecoveryData, bool, error) {
	var d authuc.RecoveryData
	err := s.pool.QueryRow(ctx,
		`SELECT id, kek_encrypted_recovery, recovery_salt FROM users WHERE id = $1`,
		userID,
	).Scan(&d.UserID, &d.KEKEncryptedRecovery, &d.RecoverySalt)
	if errors.Is(err, pgx.ErrNoRows) {
		return authuc.RecoveryData{}, false, nil
	}
	if err != nil {
		return authuc.RecoveryData{}, false, err
	}
	return d, true, nil
}

func (s *Storage) UpdateCredentialsAndKEK(ctx context.Context, userID uuid.UUID, p authuc.ResetPasswordParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users
		 SET srp_salt = $2, srp_verifier = $3, bcrypt_salt = $4, crypto_salt = $5,
		     kek_encrypted_master = $6,
		     kek_encrypted_recovery = $7, recovery_salt = $8
		 WHERE id = $1`,
		userID, p.SRPSalt, p.SRPVerifier, p.BcryptSalt, p.CryptoSalt,
		nullableBytes(p.KEKEncryptedMaster),
		nullableBytes(p.KEKEncryptedRecovery),
		nullableBytes(p.RecoverySalt),
	)
	return err
}

func (s *Storage) GetCryptoSaltAndKEKByUserID(ctx context.Context, userID uuid.UUID) (cryptoSalt []byte, kekEncMaster []byte, encryptedPrivateKey []byte, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT crypto_salt, kek_encrypted_master, encrypted_private_key FROM users WHERE id = $1`, userID,
	).Scan(&cryptoSalt, &kekEncMaster, &encryptedPrivateKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return cryptoSalt, kekEncMaster, encryptedPrivateKey, nil
}

// nullableBytes returns nil for empty slices so nullable BYTEA columns store NULL
// instead of an empty byte array.
func nullableBytes(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}
