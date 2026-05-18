package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/entity"
	srppkg "cloud-backend/pkg/srp"
)

type UserRepository interface {
	CreateUser(ctx context.Context, p NewUserParams) error
	GetByEmail(ctx context.Context, email string) (entity.User, bool, error)
}

type SessionRepository interface {
	CreateRefreshSession(ctx context.Context, p RefreshSessionParams) error
	ConsumeRefreshSession(ctx context.Context, tokenHash []byte) (ConsumedSession, bool, error)
}

type DeviceSessionRepository interface {
	CreateDeviceSession(ctx context.Context, id, userID uuid.UUID, device DeviceInfo) error
	UpdateLastActive(ctx context.Context, id uuid.UUID) error
	ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error)
	RevokeSession(ctx context.Context, id, userID uuid.UUID) error
	// returns IDs so the caller can add them to the blocklist.
	RevokeOtherSessions(ctx context.Context, userID, exceptID uuid.UUID) ([]uuid.UUID, error)
	RevokeOrphanedSessions(ctx context.Context) error
	RevokeUserOrphanedSessions(ctx context.Context, userID uuid.UUID) error
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

// Implementations must be safe for concurrent use and return quickly;
// the caller fires notifications in a goroutine.
type Notifier interface {
	NotifyNewLogin(ctx context.Context, toEmail, deviceName, ipAddress string) error
}

type AuditLogger interface {
	LogAsync(ctx context.Context, e entity.AuditEvent)
}

const dbTimeout = 5 * time.Second

func dbCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dbTimeout)
}

type Service struct {
	Users          UserRepository
	Sessions       SessionRepository
	DeviceSessions DeviceSessionRepository
	Tokens         TokenIssuer
	Blocklist      SessionBlocklist
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	SRPSessions    SRPSessionManager
	Logger         zerolog.Logger
	Notifier       Notifier
	Audit          AuditLogger
}

func (s *Service) Register(ctx context.Context, p RegisterParams) (TokenPair, error) {
	p.Email = strings.TrimSpace(strings.ToLower(p.Email))

	id := uuid.New()
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	if err := s.Users.CreateUser(tctx, NewUserParams{
		ID:                  id,
		Email:               p.Email,
		SRPSalt:             p.SRPSalt,
		SRPVerifier:         p.SRPVerifier,
		BcryptSalt:          p.BcryptSalt,
		CryptoSalt:          p.CryptoSalt,
		PublicKey:           p.PublicKey,
		EncryptedPrivateKey: p.EncryptedPrivateKey,
	}); err != nil {
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
	if !s.SRPSessions.Store(sessionID, &SRPSessEntry{
		UserID:              u.ID,
		Email:               email,
		SRPSaltHex:          u.SRPSalt,
		AHex:                aHex,
		Session:             sess,
		CryptoSalt:          slices.Clone(u.CryptoSalt),
		BcryptSalt:          u.BcryptSalt,
		EncryptedPrivateKey: slices.Clone(u.EncryptedPrivateKey),
		ExpiresAt:           time.Now().Add(SRPSessionTTL),
	}) {
		return LoginInitResult{}, fmt.Errorf("srp session store at capacity")
	}

	return LoginInitResult{
		SessionID:  sessionID.String(),
		SRPSalt:    u.SRPSalt,
		BcryptSalt: u.BcryptSalt,
		B:          sess.PublicEphemeralHex(),
		CryptoSalt: slices.Clone(u.CryptoSalt),
	}, nil
}

func (s *Service) LoginFinalize(ctx context.Context, p LoginFinalizeParams) (LoginFinalizeResult, error) {
	sid, err := uuid.Parse(p.SessionID)
	if err != nil {
		return LoginFinalizeResult{}, ErrInvalidInput
	}

	entry, ok := s.SRPSessions.Consume(sid)
	if !ok {
		return LoginFinalizeResult{}, ErrInvalidCredentials
	}

	m2Hex, err := entry.Session.VerifyClientProof(entry.AHex, p.M1, entry.Email, entry.SRPSaltHex)
	if err != nil {
		if s.Audit != nil {
			s.Audit.LogAsync(ctx, entity.AuditEvent{
				UserID:    entry.UserID,
				EventType: entity.AuditLoginFailed,
				IPAddress: p.Device.IPAddress,
				UserAgent: p.Device.UserAgent,
			})
		}
		return LoginFinalizeResult{}, ErrInvalidCredentials
	}

	// Best-effort: cleanup failure must not block login.
	cleanCtx, cleanCancel := context.WithTimeout(ctx, 3*time.Second)
	defer cleanCancel()
	if err := s.DeviceSessions.RevokeUserOrphanedSessions(cleanCtx, entry.UserID); err != nil {
		s.Logger.Warn().Err(err).Msg("orphaned session cleanup failed")
	}

	pair, err := s.issueTokenPair(ctx, entry.UserID, p.Device)
	if err != nil {
		return LoginFinalizeResult{}, err
	}

	go func() {
		nctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Notifier.NotifyNewLogin(nctx, entry.Email, p.Device.DeviceName, p.Device.IPAddress); err != nil {
			s.Logger.Warn().Err(err).Msg("login notification failed")
		}
	}()

	return LoginFinalizeResult{
		M2:                  m2Hex,
		Pair:                pair,
		EncryptedPrivateKey: entry.EncryptedPrivateKey,
		UserID:              entry.UserID,
	}, nil
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

	if err := s.DeviceSessions.UpdateLastActive(ctx, consumed.DeviceSessionID); err != nil {
		s.Logger.Debug().Err(err).Msg("update last_active_at failed")
	}

	return s.issueTokenPairForDevice(ctx, consumed.UserID, consumed.DeviceSessionID)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	hash := refreshTokenHash(refreshToken)

	tctx, cancel := dbCtx(ctx)
	defer cancel()

	consumed, ok, err := s.Sessions.ConsumeRefreshSession(tctx, hash)
	if err != nil {
		return err
	}
	if !ok {
		// Token not found or already revoked — treat as successful logout.
		return nil
	}

	if err := s.DeviceSessions.RevokeSession(ctx, consumed.DeviceSessionID, consumed.UserID); err != nil {
		return err
	}
	if s.Audit != nil {
		s.Audit.LogAsync(ctx, entity.AuditEvent{
			UserID:    consumed.UserID,
			EventType: entity.AuditLogout,
		})
	}
	return nil
}

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

func (s *Service) CleanOrphanedSessions(ctx context.Context) error {
	return s.DeviceSessions.RevokeOrphanedSessions(ctx)
}

func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID, device DeviceInfo) (TokenPair, error) {
	deviceSessionID := uuid.New()
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	if err := s.DeviceSessions.CreateDeviceSession(tctx, deviceSessionID, userID, device); err != nil {
		return TokenPair{}, fmt.Errorf("create device session: %w", err)
	}

	return s.issueTokenPairForDevice(ctx, userID, deviceSessionID)
}

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
		SessionID:       sessID,
		UserID:          userID,
		DeviceSessionID: deviceSessionID,
		TokenHash:       refreshHashBytes,
		ExpiresAt:       expiresAt,
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
