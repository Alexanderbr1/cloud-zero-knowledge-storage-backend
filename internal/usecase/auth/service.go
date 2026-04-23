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
	"cloud-backend/pkg/useragent"
)

// ─── Репозитории ──────────────────────────────────────────────────────────

type UserRepository interface {
	CreateUser(ctx context.Context, id uuid.UUID, email, srpSalt, srpVerifier, bcryptSalt string, cryptoSalt []byte) error
	GetByEmail(ctx context.Context, email string) (entity.User, bool, error)
}

type SessionRepository interface {
	CreateRefreshSession(ctx context.Context, sessionID, userID, deviceSessionID uuid.UUID, refreshTokenHash []byte, expiresAt time.Time) error
	ConsumeRefreshSession(ctx context.Context, refreshTokenHash []byte) (sessionID, userID, deviceSessionID uuid.UUID, ok bool, err error)
	RevokeRefreshSessionByHash(ctx context.Context, refreshTokenHash []byte) error
	ConsumeAndGetDeviceSession(ctx context.Context, refreshTokenHash []byte) (uuid.UUID, error)
}

type DeviceSessionRepository interface {
	CreateDeviceSession(ctx context.Context, id, userID uuid.UUID, deviceName, ipAddress, userAgent string) error
	UpdateLastActive(ctx context.Context, id uuid.UUID) error
	ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error)
	RevokeSession(ctx context.Context, id, userID uuid.UUID) error
	RevokeOtherSessions(ctx context.Context, userID, exceptID uuid.UUID) error
	RevokeDeviceSessionByID(ctx context.Context, id uuid.UUID) error
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
	RefreshTTL     time.Duration
	SRPSessions    *srpSessionStore
}

// DeviceInfo — данные об устройстве, извлечённые HTTP-обработчиком из запроса.
type DeviceInfo struct {
	UserAgent  string
	IPAddress  string
	DeviceName string // распарсенное человекочитаемое имя
}

const maxUserAgentLen = 512

// ParseDeviceInfo строит DeviceInfo из заголовков HTTP.
// User-Agent truncated to maxUserAgentLen to prevent oversized storage.
// IP is taken from X-Forwarded-For / X-Real-IP only for display — it can be spoofed by clients
// and must not be used for security decisions.
func ParseDeviceInfo(userAgent, remoteAddr, xForwardedFor, xRealIP string) DeviceInfo {
	ip := remoteAddr
	if xff := strings.TrimSpace(xForwardedFor); xff != "" {
		ip = strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	} else if xrip := strings.TrimSpace(xRealIP); xrip != "" {
		ip = xrip
	} else if idx := strings.LastIndex(remoteAddr, ":"); idx > 0 {
		ip = remoteAddr[:idx]
	}
	if len(userAgent) > maxUserAgentLen {
		userAgent = userAgent[:maxUserAgentLen]
	}
	return DeviceInfo{
		UserAgent:  userAgent,
		IPAddress:  ip,
		DeviceName: useragent.Parse(userAgent),
	}
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

func (s *Service) Register(
	ctx context.Context,
	email, srpSalt, srpVerifier, bcryptSalt string,
	cryptoSalt []byte,
	device DeviceInfo,
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
	return s.issueTokenPair(ctx, id, device)
}

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

func (s *Service) LoginFinalize(ctx context.Context, sessionID, m1Hex string, device DeviceInfo) (LoginFinalizeResult, error) {
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

	pair, err := s.issueTokenPair(ctx, entry.userID, device)
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

	_, userID, deviceSessionID, ok, err := s.Sessions.ConsumeRefreshSession(sessionCtx, hash)
	if err != nil {
		return TokenPair{}, err
	}
	if !ok {
		return TokenPair{}, ErrInvalidRefresh
	}

	// Обновляем last_active_at — пользователь активен.
	_ = s.DeviceSessions.UpdateLastActive(ctx, deviceSessionID)

	return s.issueTokenPairForDevice(ctx, userID, deviceSessionID)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil
	}
	hash := refreshTokenHash(refreshToken)

	sessionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Атомарно отзываем refresh-токен и узнаём device_session_id.
	deviceSessionID, err := s.Sessions.ConsumeAndGetDeviceSession(sessionCtx, hash)
	if err != nil {
		return err
	}
	if deviceSessionID == uuid.Nil {
		// Token not found or already revoked — treat as successful logout.
		return nil
	}

	// Отзываем само устройство.
	return s.DeviceSessions.RevokeDeviceSessionByID(ctx, deviceSessionID)
}

// ─── Управление устройствами ──────────────────────────────────────────────

func (s *Service) ListDeviceSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error) {
	return s.DeviceSessions.ListActiveSessions(ctx, userID)
}

func (s *Service) RevokeDeviceSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	return s.DeviceSessions.RevokeSession(ctx, sessionID, userID)
}

func (s *Service) RevokeOtherDeviceSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error {
	return s.DeviceSessions.RevokeOtherSessions(ctx, userID, currentSessionID)
}

// ─── Приватные методы ─────────────────────────────────────────────────────

// issueTokenPair создаёт новое device session и выдаёт токены.
func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID, device DeviceInfo) (TokenPair, error) {
	deviceSessionID := uuid.New()
	devCtx, devCancel := context.WithTimeout(ctx, 5*time.Second)
	defer devCancel()

	if err := s.DeviceSessions.CreateDeviceSession(
		devCtx, deviceSessionID, userID,
		device.DeviceName, device.IPAddress, device.UserAgent,
	); err != nil {
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

	sessCtx, sessCancel := context.WithTimeout(ctx, 5*time.Second)
	defer sessCancel()

	if err := s.Sessions.CreateRefreshSession(sessCtx, sessID, userID, deviceSessionID, refreshHashBytes, expiresAt); err != nil {
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
