package v1_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi/v1"
	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

// ─── mocks ───────────────────────────────────────────────────────────────────

type mockAuthService struct {
	registerPair    authuc.TokenPair
	registerErr     error
	loginInitResult authuc.LoginInitResult
	loginInitErr    error
	finalizeResult  authuc.LoginFinalizeResult
	finalizeErr     error
	refreshPair     authuc.TokenPair
	refreshErr      error
	logoutErr       error
	cryptoSaltB64   string
	cryptoSaltKEK   []byte
	cryptoSaltErr   error
	resetReqErr     error
	recoveryData    authuc.RecoveryData
	recoveryFound   bool
	recoveryErr     error
	resetPwErr      error
}

func (m *mockAuthService) Register(_ context.Context, _ authuc.RegisterParams) (authuc.TokenPair, error) {
	return m.registerPair, m.registerErr
}
func (m *mockAuthService) LoginInit(_ context.Context, _, _ string) (authuc.LoginInitResult, error) {
	return m.loginInitResult, m.loginInitErr
}
func (m *mockAuthService) LoginFinalize(_ context.Context, _ authuc.LoginFinalizeParams) (authuc.LoginFinalizeResult, error) {
	return m.finalizeResult, m.finalizeErr
}
func (m *mockAuthService) Refresh(_ context.Context, _ string) (authuc.TokenPair, error) {
	return m.refreshPair, m.refreshErr
}
func (m *mockAuthService) Logout(_ context.Context, _ string) error { return m.logoutErr }
func (m *mockAuthService) GetCryptoSalt(_ context.Context, _ uuid.UUID) (string, []byte, error) {
	return m.cryptoSaltB64, m.cryptoSaltKEK, m.cryptoSaltErr
}
func (m *mockAuthService) RequestPasswordReset(_ context.Context, _ string) error {
	return m.resetReqErr
}
func (m *mockAuthService) GetRecoveryData(_ context.Context, _ string) (authuc.RecoveryData, bool, error) {
	return m.recoveryData, m.recoveryFound, m.recoveryErr
}
func (m *mockAuthService) ResetPassword(_ context.Context, _ string, _ authuc.ResetPasswordParams) error {
	return m.resetPwErr
}

type mockParseJWT struct {
	userID    uuid.UUID
	sessionID uuid.UUID
	err       error
}

func (m *mockParseJWT) ParseAccessToken(_ string) (uuid.UUID, uuid.UUID, error) {
	return m.userID, m.sessionID, m.err
}

type mockBlocklistHandler struct{ blocked bool }

func (m *mockBlocklistHandler) IsBlocked(_ context.Context, _ uuid.UUID) (bool, error) {
	return m.blocked, nil
}

type mockRateLimiterAllow struct{}

func (m *mockRateLimiterAllow) Allow(_ context.Context, _ string) (bool, error) {
	return true, nil
}

type mockAuditService struct{}

func (m *mockAuditService) LogAsync(_ context.Context, _ entity.AuditEvent) {}
func (m *mockAuditService) ListEvents(_ context.Context, _ uuid.UUID, _ int, _ time.Time) ([]entity.AuditEvent, error) {
	return nil, nil
}

// ─── fixture ─────────────────────────────────────────────────────────────────

var testCookieCfg = config.RefreshCookieConfig{
	Name: "refresh_token",
	Path: "/",
}

func newRouter(auth *mockAuthService) http.Handler {
	return v1.NewRouter(v1.Deps{
		Auth:                 auth,
		Tokens:               &mockParseJWT{userID: uuid.New(), sessionID: uuid.New()},
		Blocklist:            &mockBlocklistHandler{},
		LoginRateLimiter:     &mockRateLimiterAllow{},
		PublicKeyRateLimiter: &mockRateLimiterAllow{},
		Audit:                &mockAuditService{},
		Logger:               zerolog.Nop(),
		RefreshCookie:        testCookieCfg,
	})
}

// newRouterWithJWT builds a router with a custom JWT parser — needed for
// protected-route tests where we control the injected user identity.
func newRouterWithJWT(auth *mockAuthService, jwt *mockParseJWT) http.Handler {
	return v1.NewRouter(v1.Deps{
		Auth:                 auth,
		Tokens:               jwt,
		Blocklist:            &mockBlocklistHandler{},
		LoginRateLimiter:     &mockRateLimiterAllow{},
		PublicKeyRateLimiter: &mockRateLimiterAllow{},
		Audit:                &mockAuditService{},
		Logger:               zerolog.Nop(),
		RefreshCookie:        testCookieCfg,
	})
}

func do(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func doWithAuth(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test.token.value")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// decodeBody deserialises the response body into dst.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(dst); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, w.Body.String())
	}
}

// ─── POST /auth/login/init ────────────────────────────────────────────────────

func TestLoginInit_HappyPath(t *testing.T) {
	auth := &mockAuthService{
		loginInitResult: authuc.LoginInitResult{
			SessionID:  uuid.New().String(),
			SRPSalt:    "aabbccdd",
			BcryptSalt: "$2a$12$salt",
			B:          "bbccdd",
			CryptoSalt: []byte{1, 2, 3},
		},
	}

	w := do(newRouter(auth), http.MethodPost, "/auth/login/init",
		`{"email":"alice@example.com","A":"aabb"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["session_id"] == "" {
		t.Error("session_id must be present in response")
	}
	if body["B"] == "" {
		t.Error("B must be present in response")
	}
}

func TestLoginInit_MissingEmail(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/login/init",
		`{"A":"aabb"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLoginInit_InvalidEmail(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/login/init",
		`{"email":"not-an-email","A":"aabb"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLoginInit_MissingA(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/login/init",
		`{"email":"alice@example.com"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLoginInit_InvalidJSON(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/login/init",
		`{not valid json}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLoginInit_UserNotFound_Returns401(t *testing.T) {
	auth := &mockAuthService{loginInitErr: authuc.ErrInvalidCredentials}

	w := do(newRouter(auth), http.MethodPost, "/auth/login/init",
		`{"email":"ghost@example.com","A":"aabb"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestLoginInit_InternalError_Returns500(t *testing.T) {
	auth := &mockAuthService{loginInitErr: errors.New("db down")}

	w := do(newRouter(auth), http.MethodPost, "/auth/login/init",
		`{"email":"alice@example.com","A":"aabb"}`)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── POST /auth/login/finalize ────────────────────────────────────────────────

func TestLoginFinalize_HappyPath(t *testing.T) {
	auth := &mockAuthService{
		finalizeResult: authuc.LoginFinalizeResult{
			M2:                  "m2hex",
			EncryptedPrivateKey: []byte("epk"),
			KEKEncryptedMaster:  []byte("kek"),
			Pair: authuc.TokenPair{
				AccessToken:     "access.token",
				AccessExpiresIn: 900,
			},
		},
	}

	w := do(newRouter(auth), http.MethodPost, "/auth/login/finalize",
		`{"session_id":"`+uuid.New().String()+`","M1":"aabbcc"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["access_token"] == "" {
		t.Error("access_token must be present")
	}
	if body["M2"] == "" {
		t.Error("M2 must be present")
	}
	if body["kek_encrypted_master"] == "" {
		t.Error("kek_encrypted_master must be present in finalize response")
	}
}

func TestLoginFinalize_MissingSessionID(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/login/finalize",
		`{"M1":"aabb"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLoginFinalize_InvalidCredentials_Returns401(t *testing.T) {
	auth := &mockAuthService{finalizeErr: authuc.ErrInvalidCredentials}

	w := do(newRouter(auth), http.MethodPost, "/auth/login/finalize",
		`{"session_id":"`+uuid.New().String()+`","M1":"aabb"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestLoginFinalize_InvalidInput_Returns400(t *testing.T) {
	auth := &mockAuthService{finalizeErr: authuc.ErrInvalidInput}

	w := do(newRouter(auth), http.MethodPost, "/auth/login/finalize",
		`{"session_id":"not-a-uuid","M1":"aabb"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── POST /auth/register — error mapping ─────────────────────────────────────

func validRegisterBody() string {
	kek40 := base64.StdEncoding.EncodeToString(make([]byte, 40))
	spki91 := base64.StdEncoding.EncodeToString(make([]byte, 91))
	epk69 := base64.StdEncoding.EncodeToString(make([]byte, 69))
	salt := base64.StdEncoding.EncodeToString(make([]byte, 16))
	return `{
		"email":"alice@example.com",
		"srp_salt":"aabb","srp_verifier":"ccdd","bcrypt_salt":"$2a$12$salt",
		"crypto_salt":"` + salt + `",
		"public_key":"` + spki91 + `",
		"encrypted_private_key":"` + epk69 + `",
		"kek_encrypted_master":"` + kek40 + `",
		"kek_encrypted_recovery":"` + kek40 + `",
		"recovery_salt":"` + salt + `"
	}`
}

func TestRegister_UserAlreadyExists_Returns409(t *testing.T) {
	auth := &mockAuthService{registerErr: authuc.ErrUserExists}

	w := do(newRouter(auth), http.MethodPost, "/auth/register", validRegisterBody())

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", w.Code, w.Body)
	}
}

func TestRegister_InvalidKEKLength_Returns400(t *testing.T) {
	// kek_encrypted_master must be exactly 40 bytes; 39 bytes must be rejected.
	shortKEK := base64.StdEncoding.EncodeToString(make([]byte, 39))
	kek40 := base64.StdEncoding.EncodeToString(make([]byte, 40))
	spki91 := base64.StdEncoding.EncodeToString(make([]byte, 91))
	epk69 := base64.StdEncoding.EncodeToString(make([]byte, 69))
	salt := base64.StdEncoding.EncodeToString(make([]byte, 16))

	body := `{
		"email":"alice@example.com",
		"srp_salt":"aabb","srp_verifier":"ccdd","bcrypt_salt":"$2a$12$salt",
		"crypto_salt":"` + salt + `",
		"public_key":"` + spki91 + `",
		"encrypted_private_key":"` + epk69 + `",
		"kek_encrypted_master":"` + shortKEK + `",
		"kek_encrypted_recovery":"` + kek40 + `",
		"recovery_salt":"` + salt + `"
	}`

	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/register", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for short KEK, got %d: %s", w.Code, w.Body)
	}
}

func TestRegister_MissingRequiredField_Returns400(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/register",
		`{"email":"alice@example.com"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── POST /auth/logout ────────────────────────────────────────────────────────

func TestLogout_NoCookie_Returns204(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/logout", "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 even without cookie, got %d", w.Code)
	}
}

func TestLogout_WithCookie_ClearsIt(t *testing.T) {
	router := newRouter(&mockAuthService{})
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: testCookieCfg.Name, Value: "some-refresh-token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	// The Set-Cookie header must clear the cookie (MaxAge < 0 or value empty).
	setCookie := w.Header().Get("Set-Cookie")
	if setCookie == "" {
		t.Error("expected Set-Cookie header to clear refresh token")
	}
}

// ─── POST /auth/reset-password/request ───────────────────────────────────────

func TestRequestPasswordReset_ValidEmail_Returns204(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/reset-password/request",
		`{"email":"alice@example.com"}`)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
}

func TestRequestPasswordReset_InvalidEmail_Returns400(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/reset-password/request",
		`{"email":"not-an-email"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestRequestPasswordReset_ServiceError_Returns500(t *testing.T) {
	auth := &mockAuthService{resetReqErr: errors.New("smtp failure")}

	w := do(newRouter(auth), http.MethodPost, "/auth/reset-password/request",
		`{"email":"alice@example.com"}`)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── POST /auth/reset-password/confirm ───────────────────────────────────────

func validResetConfirmBody() string {
	kek40 := base64.StdEncoding.EncodeToString(make([]byte, 40))
	salt := base64.StdEncoding.EncodeToString(make([]byte, 16))
	return `{
		"token":"reset-token-value",
		"srp_salt":"aabb","srp_verifier":"ccdd","bcrypt_salt":"$2a$12$salt",
		"crypto_salt":"` + salt + `",
		"kek_encrypted_master":"` + kek40 + `",
		"kek_encrypted_recovery":"` + kek40 + `",
		"recovery_salt":"` + salt + `"
	}`
}

func TestResetPasswordConfirm_HappyPath(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodPost, "/auth/reset-password/confirm",
		validResetConfirmBody())

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestResetPasswordConfirm_InvalidToken_Returns422(t *testing.T) {
	auth := &mockAuthService{resetPwErr: authuc.ErrResetTokenInvalid}

	w := do(newRouter(auth), http.MethodPost, "/auth/reset-password/confirm",
		validResetConfirmBody())

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}
}

func TestResetPasswordConfirm_ServiceError_Returns500(t *testing.T) {
	auth := &mockAuthService{resetPwErr: errors.New("db error")}

	w := do(newRouter(auth), http.MethodPost, "/auth/reset-password/confirm",
		validResetConfirmBody())

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── GET /auth/crypto-salt (protected) ───────────────────────────────────────

func TestGetCryptoSalt_NoAuth_Returns401(t *testing.T) {
	w := do(newRouter(&mockAuthService{}), http.MethodGet, "/auth/crypto-salt", "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestGetCryptoSalt_HappyPath(t *testing.T) {
	userID := uuid.New()
	auth := &mockAuthService{
		cryptoSaltB64: base64.StdEncoding.EncodeToString([]byte("salt-bytes")),
		cryptoSaltKEK: []byte("kek-bytes"),
	}
	router := newRouterWithJWT(auth, &mockParseJWT{userID: userID, sessionID: uuid.New()})

	w := doWithAuth(router, http.MethodGet, "/auth/crypto-salt", "")

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]string
	decodeBody(t, w, &body)
	if body["crypto_salt"] == "" {
		t.Error("crypto_salt must be present")
	}
	if body["kek_encrypted_master"] == "" {
		t.Error("kek_encrypted_master must be present")
	}
}

func TestGetCryptoSalt_ServiceError_Returns500(t *testing.T) {
	auth := &mockAuthService{cryptoSaltErr: errors.New("db error")}
	router := newRouterWithJWT(auth, &mockParseJWT{userID: uuid.New(), sessionID: uuid.New()})

	w := doWithAuth(router, http.MethodGet, "/auth/crypto-salt", "")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
