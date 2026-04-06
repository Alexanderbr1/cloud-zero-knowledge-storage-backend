package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"cloud-backend/internal/entity"
)

// UserRepository — доступ к пользователям в БД.
type UserRepository interface {
	CreateUser(ctx context.Context, id uuid.UUID, email, passwordHash string, cryptoSalt []byte) error
	GetByEmail(ctx context.Context, email string) (entity.User, bool, error)
}

// SessionRepository — refresh-сессии (ротация, отзыв).
type SessionRepository interface {
	CreateRefreshSession(ctx context.Context, sessionID, userID uuid.UUID, refreshTokenHash []byte, expiresAt time.Time) error
	ConsumeRefreshSession(ctx context.Context, refreshTokenHash []byte) (sessionID, userID uuid.UUID, ok bool, err error)
	RevokeRefreshSessionByHash(ctx context.Context, refreshTokenHash []byte) error
}

// TokenIssuer — выдача access JWT.
type TokenIssuer interface {
	IssueAccess(userID uuid.UUID) (token string, expiresInSec int64, err error)
}

// Service — регистрация, вход и обновление токенов.
type Service struct {
	Users      UserRepository
	Sessions   SessionRepository
	Tokens     TokenIssuer
	RefreshTTL time.Duration
}

// TokenPair — access + refresh после login/register/refresh.
// CryptoSalt — только для login/register: клиент деривирует мастер-ключ после успешной аутентификации.
// После refresh поле пустое (мастер-ключ уже в памяти клиента).
type TokenPair struct {
	AccessToken      string
	AccessExpiresIn  int64
	RefreshToken     string
	RefreshExpiresIn int64
	CryptoSalt       []byte
}

func (s *Service) Register(ctx context.Context, email, password string, cryptoSalt []byte) (TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return TokenPair{}, ErrInvalidInput
	}
	if len(password) < 8 {
		return TokenPair{}, ErrInvalidInput
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return TokenPair{}, fmt.Errorf("hash password: %w", err)
	}
	id := uuid.New()
	userCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.Users.CreateUser(userCtx, id, email, string(hash), cryptoSalt); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == pgerrcode.UniqueViolation {
			return TokenPair{}, ErrUserExists
		}
		return TokenPair{}, err
	}
	pair, err := s.issueTokenPair(ctx, id)
	if err != nil {
		return TokenPair{}, err
	}
	pair.CryptoSalt = append([]byte(nil), cryptoSalt...)
	return pair, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return TokenPair{}, ErrInvalidInput
	}

	userCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	u, ok, err := s.Users.GetByEmail(userCtx, email)
	if err != nil {
		return TokenPair{}, err
	}
	if !ok {
		return TokenPair{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return TokenPair{}, ErrInvalidCredentials
	}
	pair, err := s.issueTokenPair(ctx, u.ID)
	if err != nil {
		return TokenPair{}, err
	}
	pair.CryptoSalt = append([]byte(nil), u.CryptoSalt...)
	return pair, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return TokenPair{}, ErrInvalidRefresh
	}

	hash := refreshTokenHash(refreshToken)
	sessionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, userID, ok, err := s.Sessions.ConsumeRefreshSession(sessionCtx, hash)
	if err != nil {
		return TokenPair{}, err
	}
	if !ok {
		return TokenPair{}, ErrInvalidRefresh
	}
	return s.issueTokenPair(ctx, userID)
}

// Logout отзывает refresh-сессию по токену (идемпотентно: пустой токен — без ошибки).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil
	}
	hash := refreshTokenHash(refreshToken)
	sessionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return s.Sessions.RevokeRefreshSessionByHash(sessionCtx, hash)
}

func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID) (TokenPair, error) {
	accessRaw, accessExp, err := s.Tokens.IssueAccess(userID)
	if err != nil {
		return TokenPair{}, err
	}
	refreshRaw, refreshHash, err := newRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	sessID := uuid.New()
	expiresAt := time.Now().Add(s.RefreshTTL)

	sessCtx, sessCancel := context.WithTimeout(ctx, 5*time.Second)
	defer sessCancel()

	if err := s.Sessions.CreateRefreshSession(sessCtx, sessID, userID, refreshHash, expiresAt); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:      accessRaw,
		AccessExpiresIn:  accessExp,
		RefreshToken:     refreshRaw,
		RefreshExpiresIn: int64(s.RefreshTTL.Seconds()),
	}, nil
}

func newRefreshToken() (raw string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}

func refreshTokenHash(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
