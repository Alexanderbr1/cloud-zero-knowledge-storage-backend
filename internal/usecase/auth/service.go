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
	GetCryptoSaltAndKEKByUserID(ctx context.Context, userID uuid.UUID) (cryptoSalt []byte, kekEncMaster []byte, err error)
	GetRecoveryDataByUserID(ctx context.Context, userID uuid.UUID) (RecoveryData, bool, error)
	UpdateCredentialsAndKEK(ctx context.Context, userID uuid.UUID, p ResetPasswordParams) error
}

type PasswordResetRepository interface {
	CreateResetToken(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error
	ConsumeResetToken(ctx context.Context, tokenHash []byte) (uuid.UUID, bool, error)
	GetUserIDFromResetToken(ctx context.Context, tokenHash []byte) (uuid.UUID, bool, error)
	InvalidateUserTokens(ctx context.Context, userID uuid.UUID) error
}

type SessionRepository interface {
	CreateRefreshSession(ctx context.Context, p RefreshSessionParams) error
	ConsumeRefreshSession(ctx context.Context, tokenHash []byte) (ConsumedSession, bool, error)
	// FindRevokedSession looks up a refresh session that has already been consumed.
	// Used for reuse detection: if a token that was previously rotated is presented again,
	// an attacker may have gotten a new token before the legitimate user — revoke the device session.
	FindRevokedSession(ctx context.Context, tokenHash []byte) (ConsumedSession, bool, error)
}

type DeviceSessionRepository interface {
	CreateDeviceSession(ctx context.Context, id, userID uuid.UUID, device DeviceInfo) (uuid.UUID, error)
	UpdateLastActive(ctx context.Context, id uuid.UUID) error
	ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error)
	RevokeSession(ctx context.Context, id, userID uuid.UUID) error
	RevokeOtherSessions(ctx context.Context, userID, exceptID uuid.UUID) ([]uuid.UUID, error)
	RevokeOrphanedSessions(ctx context.Context) error
	RevokeUserOrphanedSessions(ctx context.Context, userID uuid.UUID) error
}

type SessionBlocklist interface {
	Block(ctx context.Context, id uuid.UUID, ttl time.Duration) error
	BlockBatch(ctx context.Context, ids []uuid.UUID, ttl time.Duration) error
}

type TokenIssuer interface {
	IssueAccess(userID, deviceSessionID uuid.UUID) (token string, expiresInSec int64, err error)
}

type Notifier interface {
	NotifyNewLogin(ctx context.Context, toEmail, deviceName, ipAddress string) error
	NotifyPasswordReset(ctx context.Context, toEmail, resetURL string) error
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
	ResetTokens    PasswordResetRepository
	FrontendOrigin string
}

func (s *Service) Register(ctx context.Context, p RegisterParams) (TokenPair, error) {
	p.Email = strings.TrimSpace(strings.ToLower(p.Email))

	id := uuid.New()
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	if err := s.Users.CreateUser(tctx, NewUserParams{
		ID:                   id,
		Email:                p.Email,
		SRPSalt:              p.SRPSalt,
		SRPVerifier:          p.SRPVerifier,
		BcryptSalt:           p.BcryptSalt,
		CryptoSalt:           p.CryptoSalt,
		PublicKey:            p.PublicKey,
		EncryptedPrivateKey:  p.EncryptedPrivateKey,
		KEKEncryptedMaster:   p.KEKEncryptedMaster,
		KEKEncryptedRecovery: p.KEKEncryptedRecovery,
		RecoverySalt:         p.RecoverySalt,
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
		KEKEncryptedMaster:  slices.Clone(u.KEKEncryptedMaster),
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
		KEKEncryptedMaster:  entry.KEKEncryptedMaster,
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
		// Token not found or already consumed. If it was previously consumed (rotated),
		// this is a reuse — someone else may have the newer token. Revoke the device session.
		s.handleTokenReuse(ctx, hash)
		return TokenPair{}, ErrInvalidRefresh
	}

	if err := s.DeviceSessions.UpdateLastActive(ctx, consumed.DeviceSessionID); err != nil {
		s.Logger.Debug().Err(err).Msg("update last_active_at failed")
	}

	return s.issueTokenPairForDevice(ctx, consumed.UserID, consumed.DeviceSessionID, consumed.ClientKey)
}

// handleTokenReuse detects refresh token reuse: if the presented token was already rotated
// (revoked_at IS NOT NULL), the token family is compromised. The device session and any
// access tokens it issued are invalidated immediately.
func (s *Service) handleTokenReuse(ctx context.Context, tokenHash []byte) {
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	session, found, err := s.Sessions.FindRevokedSession(tctx, tokenHash)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("reuse detection: db lookup failed")
		return
	}
	if !found {
		return // token never existed or expired — not a reuse, just invalid
	}

	s.Logger.Warn().
		Str("user_id", session.UserID.String()).
		Str("device_session_id", session.DeviceSessionID.String()).
		Msg("refresh token reuse detected — revoking device session")

	if err := s.DeviceSessions.RevokeSession(ctx, session.DeviceSessionID, session.UserID); err != nil {
		s.Logger.Warn().Err(err).Msg("reuse detection: revoke device session failed")
		return
	}
	if err := s.Blocklist.Block(ctx, session.DeviceSessionID, s.AccessTTL); err != nil {
		s.Logger.Warn().Err(err).Msg("reuse detection: blocklist failed")
	}
	if s.Audit != nil {
		s.Audit.LogAsync(ctx, entity.AuditEvent{
			UserID:    session.UserID,
			EventType: entity.AuditRefreshTokenReuseDetected,
		})
	}
}

func (s *Service) Logout(ctx context.Context, refreshToken string, device DeviceInfo) error {
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
			IPAddress: device.IPAddress,
			UserAgent: device.UserAgent,
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

func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	// Pad all code paths to a fixed minimum to prevent timing-based email enumeration.
	const minDelay = 300 * time.Millisecond
	start := time.Now()
	defer func() {
		if d := minDelay - time.Since(start); d > 0 {
			time.Sleep(d)
		}
	}()

	email = strings.TrimSpace(strings.ToLower(email))
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	u, ok, err := s.Users.GetByEmail(tctx, email)
	if err != nil {
		return err
	}
	if !ok || len(u.KEKEncryptedRecovery) == 0 {
		return nil
	}

	// Invalidate any previous active tokens before issuing a new one.
	itctx, icancel := dbCtx(ctx)
	defer icancel()
	if err := s.ResetTokens.InvalidateUserTokens(itctx, u.ID); err != nil {
		return err
	}

	raw, hash, err := newRefreshToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(time.Hour)

	rtctx, rcancel := dbCtx(ctx)
	defer rcancel()
	if err := s.ResetTokens.CreateResetToken(rtctx, u.ID, hash, expiresAt); err != nil {
		return err
	}

	origin := strings.TrimRight(s.FrontendOrigin, "/")
	resetURL := origin + "/auth/reset-password?token=" + raw

	nctx, ncancel := context.WithTimeout(ctx, 5*time.Second)
	defer ncancel()
	if err := s.Notifier.NotifyPasswordReset(nctx, email, resetURL); err != nil {
		s.Logger.Error().Err(err).Msg("password reset notification failed")
		return fmt.Errorf("send reset email: %w", err)
	}
	return nil
}

// GetRecoveryData looks up recovery material for a valid, unexpired reset token.
// The token is NOT consumed — it remains valid for the subsequent ResetPassword call.
func (s *Service) GetRecoveryData(ctx context.Context, token string) (RecoveryData, bool, error) {
	hash := refreshTokenHash(token)
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	userID, ok, err := s.ResetTokens.GetUserIDFromResetToken(tctx, hash)
	if err != nil {
		return RecoveryData{}, false, err
	}
	if !ok {
		return RecoveryData{}, false, nil
	}

	tctx2, cancel2 := dbCtx(ctx)
	defer cancel2()
	return s.Users.GetRecoveryDataByUserID(tctx2, userID)
}

func (s *Service) ResetPassword(ctx context.Context, token string, p ResetPasswordParams) error {
	hash := refreshTokenHash(token)
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	userID, ok, err := s.ResetTokens.ConsumeResetToken(tctx, hash)
	if err != nil {
		return err
	}
	if !ok {
		return ErrResetTokenInvalid
	}

	uctx, ucancel := dbCtx(ctx)
	defer ucancel()
	if err := s.Users.UpdateCredentialsAndKEK(uctx, userID, p); err != nil {
		return err
	}

	// Revoke all active sessions — attacker with a stolen session loses access immediately.
	sctx, scancel := dbCtx(ctx)
	defer scancel()
	revokedIDs, err := s.DeviceSessions.RevokeOtherSessions(sctx, userID, uuid.Nil)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("revoke sessions after password reset failed")
	} else if len(revokedIDs) > 0 {
		bctx, bcancel := dbCtx(ctx)
		defer bcancel()
		if err := s.Blocklist.BlockBatch(bctx, revokedIDs, s.AccessTTL); err != nil {
			s.Logger.Warn().Err(err).Msg("blocklist sessions after password reset failed")
		}
	}

	if s.Audit != nil {
		s.Audit.LogAsync(ctx, entity.AuditEvent{
			UserID:    userID,
			EventType: entity.AuditPasswordReset,
		})
	}
	return nil
}

func (s *Service) GetCryptoSalt(ctx context.Context, userID uuid.UUID) (cryptoSaltB64 string, kekEncMaster []byte, err error) {
	tctx, cancel := dbCtx(ctx)
	defer cancel()
	salt, kek, err := s.Users.GetCryptoSaltAndKEKByUserID(tctx, userID)
	if err != nil {
		return "", nil, err
	}
	return base64.StdEncoding.EncodeToString(salt), kek, nil
}

func (s *Service) CleanOrphanedSessions(ctx context.Context) error {
	return s.DeviceSessions.RevokeOrphanedSessions(ctx)
}

func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID, device DeviceInfo) (TokenPair, error) {
	proposedID := uuid.New()
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	sessionID, err := s.DeviceSessions.CreateDeviceSession(tctx, proposedID, userID, device)
	if err != nil {
		return TokenPair{}, fmt.Errorf("create device session: %w", err)
	}

	return s.issueTokenPairForDevice(ctx, userID, sessionID, nil)
}

// issueTokenPairForDevice creates a new refresh session. If clientKey is nil a
// fresh 32-byte random key is generated; otherwise the supplied key is reused
// (used on refresh so the password blob in the client's localStorage remains
// decryptable across the session chain).
func (s *Service) issueTokenPairForDevice(ctx context.Context, userID, deviceSessionID uuid.UUID, clientKey []byte) (TokenPair, error) {
	accessRaw, accessExp, err := s.Tokens.IssueAccess(userID, deviceSessionID)
	if err != nil {
		return TokenPair{}, err
	}
	refreshRaw, refreshHashBytes, err := newRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}

	if len(clientKey) == 0 {
		clientKey = make([]byte, 32)
		if _, err := rand.Read(clientKey); err != nil {
			return TokenPair{}, fmt.Errorf("generate client key: %w", err)
		}
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
		ClientKey:       clientKey,
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
		ClientKey:        clientKey,
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
