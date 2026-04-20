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

	"cloud-backend/internal/entity"
	srppkg "cloud-backend/pkg/srp"
)

// UserRepository — доступ к пользователям в БД.
type UserRepository interface {
	CreateUser(ctx context.Context, id uuid.UUID, email, srpSalt, srpVerifier, bcryptSalt string, cryptoSalt []byte) error
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
	Users       UserRepository
	Sessions    SessionRepository
	Tokens      TokenIssuer
	RefreshTTL  time.Duration
	SRPSessions *srpSessionStore
}

// TokenPair — access + refresh после login/register/refresh.
type TokenPair struct {
	AccessToken      string
	AccessExpiresIn  int64
	RefreshToken     string
	RefreshExpiresIn int64
}

// LoginInitResult — данные для клиента после первого шага SRP-логина.
type LoginInitResult struct {
	SessionID  string
	SRPSalt    string // hex-encoded SRP salt
	BcryptSalt string // bcrypt salt string ($2b$10$...)
	B          string // hex-encoded server public ephemeral
	CryptoSalt []byte // for PBKDF2 master-key derivation on client
}

// LoginFinalizeResult — данные после успешной проверки M1.
type LoginFinalizeResult struct {
	M2   string
	Pair TokenPair
	// CryptoSalt is empty here: already returned in LoginInitResult.
}

// Register stores SRP credentials and issues tokens.
// The client is responsible for all bcrypt and SRP computation.
func (s *Service) Register(
	ctx context.Context,
	email, srpSalt, srpVerifier, bcryptSalt string,
	cryptoSalt []byte,
) (TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || srpSalt == "" || srpVerifier == "" || bcryptSalt == "" || len(cryptoSalt) == 0 {
		return TokenPair{}, ErrInvalidInput
	}

	id := uuid.New()
	userCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.Users.CreateUser(userCtx, id, email, srpSalt, srpVerifier, bcryptSalt, cryptoSalt); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == pgerrcode.UniqueViolation {
			return TokenPair{}, ErrUserExists
		}
		return TokenPair{}, err
	}
	return s.issueTokenPair(ctx, id)
}

// LoginInit — первый шаг SRP-логина.
// Клиент присылает email и свой публичный эфемерный ключ A.
// Сервер возвращает SRP-параметры и свой B, не проверяя ничего криптографически.
func (s *Service) LoginInit(ctx context.Context, email, aHex string) (LoginInitResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || aHex == "" {
		return LoginInitResult{}, ErrInvalidInput
	}

	userCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	u, ok, err := s.Users.GetByEmail(userCtx, email)
	if err != nil {
		return LoginInitResult{}, err
	}
	if !ok {
		// Don't reveal that user doesn't exist; return a dummy delay target
		return LoginInitResult{}, ErrInvalidCredentials
	}

	sess, err := srppkg.NewServerSession(u.SRPVerifier)
	if err != nil {
		return LoginInitResult{}, fmt.Errorf("srp new session: %w", err)
	}

	sessionID := uuid.New()
	s.SRPSessions.store(sessionID, &srpSessEntry{
		userID:     u.ID,
		email:      email,
		srpSaltHex: u.SRPSalt,
		aHex:       aHex,
		session:    sess,
		cryptoSalt: append([]byte(nil), u.CryptoSalt...),
		bcryptSalt: u.BcryptSalt,
		expiresAt:  time.Now().Add(srpSessionTTL),
	})

	return LoginInitResult{
		SessionID:  sessionID.String(),
		SRPSalt:    u.SRPSalt,
		BcryptSalt: u.BcryptSalt,
		B:          sess.PublicEphemeralHex(),
		CryptoSalt: append([]byte(nil), u.CryptoSalt...),
	}, nil
}

// LoginFinalize — второй шаг SRP-логина.
// Клиент присылает идентификатор сессии и свой proof M1.
// Сервер проверяет M1 и возвращает M2 вместе с токенами.
func (s *Service) LoginFinalize(ctx context.Context, sessionID, m1Hex string) (LoginFinalizeResult, error) {
	sid, err := uuid.Parse(sessionID)
	if err != nil {
		return LoginFinalizeResult{}, ErrInvalidInput
	}

	entry, ok := s.SRPSessions.consume(sid)
	if !ok {
		return LoginFinalizeResult{}, ErrInvalidCredentials
	}

	m2Hex, err := entry.session.VerifyClientProof(entry.aHex, m1Hex, entry.email, entry.srpSaltHex)
	if err != nil {
		return LoginFinalizeResult{}, ErrInvalidCredentials
	}

	pair, err := s.issueTokenPair(ctx, entry.userID)
	if err != nil {
		return LoginFinalizeResult{}, err
	}

	return LoginFinalizeResult{M2: m2Hex, Pair: pair}, nil
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
