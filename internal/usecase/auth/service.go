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
	"github.com/rs/zerolog"

	"cloud-backend/internal/entity"
	srppkg "cloud-backend/pkg/srp"
)

const dbTimeout = 5 * time.Second

// dbCtx возвращает дочерний контекст с таймаутом для одного DB-запроса.
func dbCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dbTimeout)
}

// ─── Репозитории ──────────────────────────────────────────────────────────

// ─── Параметры сервиса ────────────────────────────────────────────────────

type RegisterParams struct {
	Email       string
	SRPSalt     string
	SRPVerifier string
	BcryptSalt  string
	CryptoSalt  []byte
	Device      DeviceInfo
}

type LoginFinalizeParams struct {
	SessionID string
	M1        string
	Device    DeviceInfo
}

// ─── Параметры репозиториев ───────────────────────────────────────────────

type NewUserParams struct {
	ID          uuid.UUID
	Email       string
	SRPSalt     string
	SRPVerifier string
	BcryptSalt  string
	CryptoSalt  []byte
}

type RefreshSessionParams struct {
	SessionID       uuid.UUID
	UserID          uuid.UUID
	DeviceSessionID uuid.UUID
	TokenHash       []byte
	ExpiresAt       time.Time
}

type ConsumedSession struct {
	SessionID       uuid.UUID
	UserID          uuid.UUID
	DeviceSessionID uuid.UUID
}

// ─── Репозитории ──────────────────────────────────────────────────────────

type UserRepository interface {
	CreateUser(ctx context.Context, p NewUserParams) error
	GetByEmail(ctx context.Context, email string) (entity.User, bool, error)
}

type SessionRepository interface {
	CreateRefreshSession(ctx context.Context, p RefreshSessionParams) error
	ConsumeRefreshSession(ctx context.Context, tokenHash []byte) (ConsumedSession, bool, error)
	ConsumeAndGetSession(ctx context.Context, tokenHash []byte) (userID, deviceSessionID uuid.UUID, err error)
}

type DeviceSessionRepository interface {
	CreateDeviceSession(ctx context.Context, id, userID uuid.UUID, device DeviceInfo) error
	UpdateLastActive(ctx context.Context, id uuid.UUID) error
	ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error)
	RevokeSession(ctx context.Context, id, userID uuid.UUID) error
	// RevokeOtherSessions revokes all sessions except exceptID and returns their IDs
	// so the caller can add them to the blocklist.
	RevokeOtherSessions(ctx context.Context, userID, exceptID uuid.UUID) ([]uuid.UUID, error)
	// RevokeOrphanedSessions revokes sessions with no active refresh tokens.
	// Pass uuid.Nil to revoke across all users (used by the background job).
	RevokeOrphanedSessions(ctx context.Context, userID uuid.UUID) error
}

// SessionBlocklist records revoked session IDs for the remaining lifetime of
// their access tokens so the auth middleware can reject them immediately.
type SessionBlocklist interface {
	Block(ctx context.Context, id uuid.UUID, ttl time.Duration) error
	BlockBatch(ctx context.Context, ids []uuid.UUID, ttl time.Duration) error
}

type TokenIssuer interface {
	IssueAccess(userID, deviceSessionID uuid.UUID) (token string, expiresInSec int64, err error)
}

// ─── Сервис ───────────────────────────────────────────────────────────────

type Service struct {
	Users          UserRepository
	Sessions       SessionRepository
	DeviceSessions DeviceSessionRepository
	Tokens         TokenIssuer
	Blocklist      SessionBlocklist
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	SRPSessions    srpSessionManager
	Logger         zerolog.Logger
}

// DeviceInfo — данные об устройстве, передаваемые из транспортного слоя.
type DeviceInfo struct {
	UserAgent  string
	IPAddress  string
	DeviceName string
}

// ─── Типы результатов ─────────────────────────────────────────────────────

type TokenPair struct {
	AccessToken      string
	AccessExpiresIn  int64
	RefreshToken     string
	RefreshExpiresIn int64
	DeviceSessionID  uuid.UUID
}

type LoginInitResult struct {
	SessionID  string
	SRPSalt    string
	BcryptSalt string
	B          string
	CryptoSalt []byte
}

type LoginFinalizeResult struct {
	M2   string
	Pair TokenPair
}

// ─── Методы аутентификации ────────────────────────────────────────────────

func (s *Service) Register(ctx context.Context, p RegisterParams) (TokenPair, error) {
	p.Email = strings.TrimSpace(strings.ToLower(p.Email))

	id := uuid.New()
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	if err := s.Users.CreateUser(tctx, NewUserParams{
		ID: id, Email: p.Email, SRPSalt: p.SRPSalt, SRPVerifier: p.SRPVerifier,
		BcryptSalt: p.BcryptSalt, CryptoSalt: p.CryptoSalt,
	}); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == pgerrcode.UniqueViolation {
			return TokenPair{}, ErrUserExists
		}
		return TokenPair{}, err
	}
	return s.issueTokenPair(ctx, id, p.Device)
}

func (s *Service) LoginInit(ctx context.Context, email, aHex string) (LoginInitResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	tctx, cancel := dbCtx(ctx)
	defer cancel()

	u, ok, err := s.Users.GetByEmail(tctx, email)
	if err != nil {
		return LoginInitResult{}, err
	}
	if !ok {
		return LoginInitResult{}, ErrInvalidCredentials
	}

	sess, err := srppkg.NewServerSession(u.SRPVerifier)
	if err != nil {
		return LoginInitResult{}, fmt.Errorf("srp new session: %w", err)
	}

	sessionID := uuid.New()
	if !s.SRPSessions.store(sessionID, &srpSessEntry{
		userID:     u.ID,
		email:      email,
		srpSaltHex: u.SRPSalt,
		aHex:       aHex,
		session:    sess,
		cryptoSalt: append([]byte(nil), u.CryptoSalt...),
		bcryptSalt: u.BcryptSalt,
		expiresAt:  time.Now().Add(srpSessionTTL),
	}) {
		return LoginInitResult{}, fmt.Errorf("srp session store at capacity")
	}

	return LoginInitResult{
		SessionID:  sessionID.String(),
		SRPSalt:    u.SRPSalt,
		BcryptSalt: u.BcryptSalt,
		B:          sess.PublicEphemeralHex(),
		CryptoSalt: append([]byte(nil), u.CryptoSalt...),
	}, nil
}

func (s *Service) LoginFinalize(ctx context.Context, p LoginFinalizeParams) (LoginFinalizeResult, error) {
	sid, err := uuid.Parse(p.SessionID)
	if err != nil {
		return LoginFinalizeResult{}, ErrInvalidInput
	}

	entry, ok := s.SRPSessions.consume(sid)
	if !ok {
		return LoginFinalizeResult{}, ErrInvalidCredentials
	}

	m2Hex, err := entry.session.VerifyClientProof(entry.aHex, p.M1, entry.email, entry.srpSaltHex)
	if err != nil {
		return LoginFinalizeResult{}, ErrInvalidCredentials
	}

	// Clean up this user's orphaned sessions before creating a new one.
	// Best-effort: a cleanup failure must not block login.
	cleanCtx, cleanCancel := context.WithTimeout(ctx, 3*time.Second)
	defer cleanCancel()
	if err := s.DeviceSessions.RevokeOrphanedSessions(cleanCtx, entry.userID); err != nil {
		s.Logger.Warn().Err(err).Msg("orphaned session cleanup failed")
	}

	pair, err := s.issueTokenPair(ctx, entry.userID, p.Device)
	if err != nil {
		return LoginFinalizeResult{}, err
	}

	return LoginFinalizeResult{M2: m2Hex, Pair: pair}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	hash := refreshTokenHash(refreshToken)
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	consumed, ok, err := s.Sessions.ConsumeRefreshSession(tctx, hash)
	if err != nil {
		return TokenPair{}, err
	}
	if !ok {
		return TokenPair{}, ErrInvalidRefresh
	}

	// Обновляем last_active_at — пользователь активен.
	if err := s.DeviceSessions.UpdateLastActive(ctx, consumed.DeviceSessionID); err != nil {
		s.Logger.Warn().Err(err).Msg("update last_active_at failed")
	}

	return s.issueTokenPairForDevice(ctx, consumed.UserID, consumed.DeviceSessionID)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	hash := refreshTokenHash(refreshToken)

	tctx, cancel := dbCtx(ctx)
	defer cancel()

	// Атомарно отзываем refresh-токен и узнаём device_session_id + userID.
	userID, deviceSessionID, err := s.Sessions.ConsumeAndGetSession(tctx, hash)
	if err != nil {
		return err
	}
	if deviceSessionID == uuid.Nil {
		// Token not found or already revoked — treat as successful logout.
		return nil
	}

	return s.DeviceSessions.RevokeSession(ctx, deviceSessionID, userID)
}

// ─── Управление устройствами ──────────────────────────────────────────────

func (s *Service) ListDeviceSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error) {
	return s.DeviceSessions.ListActiveSessions(ctx, userID)
}

func (s *Service) RevokeDeviceSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	if err := s.DeviceSessions.RevokeSession(ctx, sessionID, userID); err != nil {
		return err
	}
	if err := s.Blocklist.Block(ctx, sessionID, s.AccessTTL); err != nil {
		s.Logger.Warn().Err(err).Msg("blocklist block failed")
	}
	return nil
}

func (s *Service) RevokeOtherDeviceSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error {
	revokedIDs, err := s.DeviceSessions.RevokeOtherSessions(ctx, userID, currentSessionID)
	if err != nil {
		return err
	}
	if len(revokedIDs) > 0 {
		if err := s.Blocklist.BlockBatch(ctx, revokedIDs, s.AccessTTL); err != nil {
			s.Logger.Warn().Err(err).Msg("blocklist block_batch failed")
		}
	}
	return nil
}

// CleanOrphanedSessions удаляет все "мёртвые" device sessions по всем пользователям.
// Предназначен для вызова фоновой джобой.
func (s *Service) CleanOrphanedSessions(ctx context.Context) error {
	return s.DeviceSessions.RevokeOrphanedSessions(ctx, uuid.Nil)
}

// ─── Приватные методы ─────────────────────────────────────────────────────

// issueTokenPair создаёт новое device session и выдаёт токены.
func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID, device DeviceInfo) (TokenPair, error) {
	deviceSessionID := uuid.New()
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	if err := s.DeviceSessions.CreateDeviceSession(tctx, deviceSessionID, userID, device); err != nil {
		return TokenPair{}, fmt.Errorf("create device session: %w", err)
	}

	return s.issueTokenPairForDevice(ctx, userID, deviceSessionID)
}

// issueTokenPairForDevice выдаёт токены для существующей device session (используется при Refresh).
func (s *Service) issueTokenPairForDevice(ctx context.Context, userID, deviceSessionID uuid.UUID) (TokenPair, error) {
	accessRaw, accessExp, err := s.Tokens.IssueAccess(userID, deviceSessionID)
	if err != nil {
		return TokenPair{}, err
	}
	refreshRaw, refreshHashBytes, err := newRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	sessID := uuid.New()
	expiresAt := time.Now().Add(s.RefreshTTL)

	tctx, cancel := dbCtx(ctx)
	defer cancel()

	if err := s.Sessions.CreateRefreshSession(tctx, RefreshSessionParams{
		SessionID: sessID, UserID: userID, DeviceSessionID: deviceSessionID,
		TokenHash: refreshHashBytes, ExpiresAt: expiresAt,
	}); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:      accessRaw,
		AccessExpiresIn:  accessExp,
		RefreshToken:     refreshRaw,
		RefreshExpiresIn: int64(s.RefreshTTL.Seconds()),
		DeviceSessionID:  deviceSessionID,
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
