package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/entity"
	"cloud-backend/internal/repo/memory"
	authuc "cloud-backend/internal/usecase/auth"
	"cloud-backend/pkg/jwt"
)

// ─── Mock implementations ─────────────────────────────────────────────────────

type mockUserRepo struct {
	createErr     error
	user          entity.User
	found         bool
	getErr        error
	updateErr     error
	cryptoSalt    []byte
	kek           []byte
	cryptoErr     error
	recovery      authuc.RecoveryData
	recoveryFound bool
	recoveryErr   error
}

func (m *mockUserRepo) CreateUser(_ context.Context, _ authuc.NewUserParams) error {
	return m.createErr
}
func (m *mockUserRepo) GetByEmail(_ context.Context, _ string) (entity.User, bool, error) {
	return m.user, m.found, m.getErr
}
func (m *mockUserRepo) GetCryptoSaltAndKEKByUserID(_ context.Context, _ uuid.UUID) ([]byte, []byte, error) {
	return m.cryptoSalt, m.kek, m.cryptoErr
}
func (m *mockUserRepo) GetRecoveryDataByUserID(_ context.Context, _ uuid.UUID) (authuc.RecoveryData, bool, error) {
	return m.recovery, m.recoveryFound, m.recoveryErr
}
func (m *mockUserRepo) UpdateCredentialsAndKEK(_ context.Context, _ uuid.UUID, _ authuc.ResetPasswordParams) error {
	return m.updateErr
}

type mockSessionRepo struct {
	createErr  error
	consumed   authuc.ConsumedSession
	found      bool
	consumeErr error
}

func (m *mockSessionRepo) CreateRefreshSession(_ context.Context, _ authuc.RefreshSessionParams) error {
	return m.createErr
}
func (m *mockSessionRepo) ConsumeRefreshSession(_ context.Context, _ []byte) (authuc.ConsumedSession, bool, error) {
	return m.consumed, m.found, m.consumeErr
}

type mockDeviceRepo struct {
	createErr      error
	revokeOtherIDs []uuid.UUID
	revokeOtherErr error
}

func (m *mockDeviceRepo) CreateDeviceSession(_ context.Context, _, _ uuid.UUID, _ authuc.DeviceInfo) error {
	return m.createErr
}
func (m *mockDeviceRepo) UpdateLastActive(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockDeviceRepo) ListActiveSessions(_ context.Context, _ uuid.UUID) ([]entity.DeviceSession, error) {
	return nil, nil
}
func (m *mockDeviceRepo) RevokeSession(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockDeviceRepo) RevokeOtherSessions(_ context.Context, _, _ uuid.UUID) ([]uuid.UUID, error) {
	return m.revokeOtherIDs, m.revokeOtherErr
}
func (m *mockDeviceRepo) RevokeOrphanedSessions(_ context.Context) error { return nil }
func (m *mockDeviceRepo) RevokeUserOrphanedSessions(_ context.Context, _ uuid.UUID) error {
	return nil
}

type mockBlocklist struct {
	batchErr error
}

func (m *mockBlocklist) Block(_ context.Context, _ uuid.UUID, _ time.Duration) error { return nil }
func (m *mockBlocklist) BlockBatch(_ context.Context, _ []uuid.UUID, _ time.Duration) error {
	return m.batchErr
}

type mockNotifier struct {
	resetErr error
}

func (m *mockNotifier) NotifyNewLogin(_ context.Context, _, _, _ string) error { return nil }
func (m *mockNotifier) NotifyPasswordReset(_ context.Context, _, _ string) error {
	return m.resetErr
}

type mockAudit struct{}

func (m *mockAudit) LogAsync(_ context.Context, _ entity.AuditEvent) {}

type mockResetTokens struct {
	createErr     error
	userID        uuid.UUID
	found         bool
	consumeErr    error
	getErr        error
	invalidateErr error
}

func (m *mockResetTokens) CreateResetToken(_ context.Context, _ uuid.UUID, _ []byte, _ time.Time) error {
	return m.createErr
}
func (m *mockResetTokens) ConsumeResetToken(_ context.Context, _ []byte) (uuid.UUID, bool, error) {
	return m.userID, m.found, m.consumeErr
}
func (m *mockResetTokens) GetUserIDFromResetToken(_ context.Context, _ []byte) (uuid.UUID, bool, error) {
	return m.userID, m.found, m.getErr
}
func (m *mockResetTokens) InvalidateUserTokens(_ context.Context, _ uuid.UUID) error {
	return m.invalidateErr
}

// ─── Builder ──────────────────────────────────────────────────────────────────

type serviceFixture struct {
	users       *mockUserRepo
	sessions    *mockSessionRepo
	devices     *mockDeviceRepo
	blocklist   *mockBlocklist
	notifier    *mockNotifier
	resetTokens *mockResetTokens
	svc         *authuc.Service
}

func newFixture(t *testing.T) *serviceFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	f := &serviceFixture{
		users:       &mockUserRepo{},
		sessions:    &mockSessionRepo{},
		devices:     &mockDeviceRepo{},
		blocklist:   &mockBlocklist{},
		notifier:    &mockNotifier{},
		resetTokens: &mockResetTokens{},
	}
	f.svc = &authuc.Service{
		Users:          f.users,
		Sessions:       f.sessions,
		DeviceSessions: f.devices,
		Tokens:         jwt.NewService([]byte("test-secret-32-bytes-1234567890!"), 15*time.Minute),
		Blocklist:      f.blocklist,
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     720 * time.Hour,
		SRPSessions:    memory.NewSRPSessionStore(ctx),
		Notifier:       f.notifier,
		Audit:          &mockAudit{},
		ResetTokens:    f.resetTokens,
		Logger:         zerolog.Nop(),
		FrontendOrigin: "http://localhost:4200",
	}
	return f
}

// ─── Register ─────────────────────────────────────────────────────────────────

func TestRegister_HappyPath(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Register(context.Background(), authuc.RegisterParams{
		Email:       "alice@example.com",
		SRPSalt:     "aabbcc",
		SRPVerifier: "ddeeff",
		BcryptSalt:  "$2a$12$salt",
		CryptoSalt:  []byte("salt"),
		Device:      authuc.DeviceInfo{DeviceName: "Test"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	f := newFixture(t)
	f.users.createErr = authuc.ErrUserExists

	_, err := f.svc.Register(context.Background(), authuc.RegisterParams{
		Email: "alice@example.com",
	})
	if !errors.Is(err, authuc.ErrUserExists) {
		t.Fatalf("want ErrUserExists, got %v", err)
	}
}

func TestRegister_RepoError(t *testing.T) {
	f := newFixture(t)
	f.users.createErr = errors.New("db connection failed")

	_, err := f.svc.Register(context.Background(), authuc.RegisterParams{
		Email: "alice@example.com",
	})
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestRegister_EmailNormalized(t *testing.T) {
	f := newFixture(t)
	var createdEmail string

	f.svc.Users = &captureUserRepo{
		mockUserRepo: f.users,
		onCreateUser: func(p authuc.NewUserParams) { createdEmail = p.Email },
	}

	f.svc.Register(context.Background(), authuc.RegisterParams{
		Email: "  Alice@Example.COM  ",
	})
	if createdEmail != "alice@example.com" {
		t.Errorf("email not normalised: got %q", createdEmail)
	}
}

// captureUserRepo wraps mockUserRepo and calls a hook on CreateUser.
type captureUserRepo struct {
	*mockUserRepo
	onCreateUser func(authuc.NewUserParams)
}

func (r *captureUserRepo) CreateUser(ctx context.Context, p authuc.NewUserParams) error {
	if r.onCreateUser != nil {
		r.onCreateUser(p)
	}
	return r.mockUserRepo.CreateUser(ctx, p)
}

// ─── LoginInit ────────────────────────────────────────────────────────────────

func TestLoginInit_UserNotFound(t *testing.T) {
	f := newFixture(t)
	f.users.found = false

	_, err := f.svc.LoginInit(context.Background(), "nobody@example.com", "aabbcc")
	if !errors.Is(err, authuc.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginInit_RepoError(t *testing.T) {
	f := newFixture(t)
	f.users.getErr = errors.New("db error")

	_, err := f.svc.LoginInit(context.Background(), "alice@example.com", "aabbcc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoginInit_HappyPath_ReturnsSRPFields(t *testing.T) {
	f := newFixture(t)
	f.users.found = true
	f.users.user = entity.User{
		ID:                 uuid.New(),
		Email:              "alice@example.com",
		SRPSalt:            "aabbccdd",
		SRPVerifier:        "02", // v = 2, minimal valid
		BcryptSalt:         "$2a$12$salt",
		CryptoSalt:         []byte("cryptosalt"),
		KEKEncryptedMaster: []byte("kek"),
	}

	result, err := f.svc.LoginInit(context.Background(), "alice@example.com", "aabbcc")
	if err != nil {
		t.Fatalf("LoginInit: %v", err)
	}
	if result.SRPSalt != "aabbccdd" {
		t.Errorf("SRPSalt: want aabbccdd, got %s", result.SRPSalt)
	}
	if result.SessionID == "" {
		t.Error("SessionID must not be empty")
	}
	if result.B == "" {
		t.Error("B must not be empty")
	}
}

// ─── LoginFinalize ────────────────────────────────────────────────────────────

func TestLoginFinalize_InvalidSessionUUID(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.LoginFinalize(context.Background(), authuc.LoginFinalizeParams{
		SessionID: "not-a-uuid",
		M1:        "aabb",
	})
	if !errors.Is(err, authuc.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestLoginFinalize_SessionNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.LoginFinalize(context.Background(), authuc.LoginFinalizeParams{
		SessionID: uuid.New().String(),
		M1:        "aabb",
	})
	if !errors.Is(err, authuc.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

// ─── Refresh ──────────────────────────────────────────────────────────────────

func TestRefresh_InvalidToken(t *testing.T) {
	f := newFixture(t)
	f.sessions.found = false

	_, err := f.svc.Refresh(context.Background(), "invalid-refresh-token")
	if !errors.Is(err, authuc.ErrInvalidRefresh) {
		t.Fatalf("want ErrInvalidRefresh, got %v", err)
	}
}

func TestRefresh_HappyPath(t *testing.T) {
	f := newFixture(t)
	f.sessions.found = true
	f.sessions.consumed = authuc.ConsumedSession{
		UserID:          uuid.New(),
		DeviceSessionID: uuid.New(),
		ClientKey:       []byte("clientkey"),
	}

	pair, err := f.svc.Refresh(context.Background(), "any-token")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

// ─── RequestPasswordReset ─────────────────────────────────────────────────────

func TestRequestPasswordReset_UnknownEmail_NoError(t *testing.T) {
	f := newFixture(t)
	f.users.found = false

	// Must return nil even for non-existent email (anti-enumeration).
	if err := f.svc.RequestPasswordReset(context.Background(), "ghost@example.com"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestRequestPasswordReset_NoRecovery_NoError(t *testing.T) {
	f := newFixture(t)
	f.users.found = true
	f.users.user = entity.User{
		ID:                   uuid.New(),
		Email:                "alice@example.com",
		KEKEncryptedRecovery: nil, // no recovery set up
	}

	if err := f.svc.RequestPasswordReset(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("want nil for account without recovery, got %v", err)
	}
}

func TestRequestPasswordReset_NotifierError_ReturnsError(t *testing.T) {
	f := newFixture(t)
	f.users.found = true
	f.users.user = entity.User{
		ID:                   uuid.New(),
		Email:                "alice@example.com",
		KEKEncryptedRecovery: []byte("kek-recovery"),
	}
	f.notifier.resetErr = errors.New("smtp error")

	err := f.svc.RequestPasswordReset(context.Background(), "alice@example.com")
	if err == nil {
		t.Fatal("expected error when notifier fails")
	}
}

// ─── ResetPassword ────────────────────────────────────────────────────────────

func TestResetPassword_InvalidToken(t *testing.T) {
	f := newFixture(t)
	f.resetTokens.found = false

	err := f.svc.ResetPassword(context.Background(), "bad-token", authuc.ResetPasswordParams{})
	if !errors.Is(err, authuc.ErrResetTokenInvalid) {
		t.Fatalf("want ErrResetTokenInvalid, got %v", err)
	}
}

func TestResetPassword_HappyPath_RevokesSessionsAndBlocklists(t *testing.T) {
	f := newFixture(t)
	revokedID := uuid.New()
	f.resetTokens.found = true
	f.resetTokens.userID = uuid.New()
	f.devices.revokeOtherIDs = []uuid.UUID{revokedID}

	blocklisted := false
	f.blocklist = &mockBlocklist{}
	f.svc.Blocklist = &capturingBlocklist{
		onBatch: func(_ []uuid.UUID) { blocklisted = true },
	}

	err := f.svc.ResetPassword(context.Background(), "valid-token", authuc.ResetPasswordParams{
		SRPSalt:     "aabb",
		SRPVerifier: "ccdd",
		BcryptSalt:  "$2a$12$salt",
		CryptoSalt:  []byte("s"),
	})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if !blocklisted {
		t.Error("revoked sessions should be added to blocklist")
	}
}

// capturingBlocklist records calls to Block and BlockBatch.
type capturingBlocklist struct {
	onBlock func(uuid.UUID)
	onBatch func([]uuid.UUID)
}

func (c *capturingBlocklist) Block(_ context.Context, id uuid.UUID, _ time.Duration) error {
	if c.onBlock != nil {
		c.onBlock(id)
	}
	return nil
}
func (c *capturingBlocklist) BlockBatch(_ context.Context, ids []uuid.UUID, _ time.Duration) error {
	if c.onBatch != nil {
		c.onBatch(ids)
	}
	return nil
}

// ─── Refresh (additional) ─────────────────────────────────────────────────────

func TestRefresh_RepoError(t *testing.T) {
	f := newFixture(t)
	f.sessions.consumeErr = errors.New("db error")

	_, err := f.svc.Refresh(context.Background(), "any-token")
	if err == nil {
		t.Fatal("expected error when session repo fails")
	}
}

// ─── Logout ───────────────────────────────────────────────────────────────────

func TestLogout_HappyPath(t *testing.T) {
	f := newFixture(t)
	f.sessions.found = true
	f.sessions.consumed = authuc.ConsumedSession{
		UserID:          uuid.New(),
		DeviceSessionID: uuid.New(),
	}
	if err := f.svc.Logout(context.Background(), "any-token"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
}

func TestLogout_AlreadyRevoked_NoError(t *testing.T) {
	f := newFixture(t)
	f.sessions.found = false // token not found — treat as already logged out

	if err := f.svc.Logout(context.Background(), "stale-token"); err != nil {
		t.Fatalf("want nil for already-revoked token, got %v", err)
	}
}

// ─── GetCryptoSalt ────────────────────────────────────────────────────────────

func TestGetCryptoSalt_HappyPath(t *testing.T) {
	f := newFixture(t)
	f.users.cryptoSalt = []byte{1, 2, 3}
	f.users.kek = []byte("kek-bytes")

	saltB64, kek, err := f.svc.GetCryptoSalt(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetCryptoSalt: %v", err)
	}
	if saltB64 == "" {
		t.Error("expected non-empty base64 salt")
	}
	if string(kek) != "kek-bytes" {
		t.Errorf("kek mismatch: want %q, got %q", "kek-bytes", kek)
	}
}

// ─── RevokeDeviceSession ──────────────────────────────────────────────────────

func TestRevokeDeviceSession_BlocklistCalled(t *testing.T) {
	f := newFixture(t)
	targetID := uuid.New()
	blocked := false
	f.svc.Blocklist = &capturingBlocklist{
		onBlock: func(id uuid.UUID) {
			if id == targetID {
				blocked = true
			}
		},
	}

	if err := f.svc.RevokeDeviceSession(context.Background(), uuid.New(), targetID); err != nil {
		t.Fatalf("RevokeDeviceSession: %v", err)
	}
	if !blocked {
		t.Error("revoked session should be added to blocklist")
	}
}

// ─── RevokeOtherDeviceSessions ────────────────────────────────────────────────

func TestRevokeOtherDeviceSessions_BlocklistCalled(t *testing.T) {
	f := newFixture(t)
	f.devices.revokeOtherIDs = []uuid.UUID{uuid.New(), uuid.New()}
	blocklisted := false
	f.svc.Blocklist = &capturingBlocklist{
		onBatch: func(_ []uuid.UUID) { blocklisted = true },
	}

	if err := f.svc.RevokeOtherDeviceSessions(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("RevokeOtherDeviceSessions: %v", err)
	}
	if !blocklisted {
		t.Error("revoked sessions should be added to blocklist batch")
	}
}
