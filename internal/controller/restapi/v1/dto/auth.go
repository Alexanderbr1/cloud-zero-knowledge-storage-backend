package dto

// RegisterRequest — SRP-6a registration.
// The client performs all bcrypt + SRP computation and sends only the derived values.
type RegisterRequest struct {
	Email       string `json:"email"        validate:"required,email,max=320"`
	SRPSalt     string `json:"srp_salt"     validate:"required"`
	SRPVerifier string `json:"srp_verifier" validate:"required"`
	BcryptSalt  string `json:"bcrypt_salt"  validate:"required"`
	CryptoSalt  string `json:"crypto_salt"  validate:"required"` // base64-encoded PBKDF2 salt
}

// LoginInitRequest — first leg of SRP-6a login.
type LoginInitRequest struct {
	Email string `json:"email" validate:"required,email,max=320"`
	A     string `json:"A"     validate:"required"` // client public ephemeral (hex)
}

// LoginInitResponse — server responds with SRP parameters and its public ephemeral.
type LoginInitResponse struct {
	SessionID  string `json:"session_id"`
	SRPSalt    string `json:"srp_salt"`    // hex-encoded SRP salt
	BcryptSalt string `json:"bcrypt_salt"` // bcrypt salt string ($2b$10$...)
	B          string `json:"B"`           // server public ephemeral (hex)
	CryptoSalt string `json:"crypto_salt"` // base64-encoded PBKDF2 salt
}

// LoginFinalizeRequest — second leg of SRP-6a login.
type LoginFinalizeRequest struct {
	SessionID string `json:"session_id" validate:"required"`
	M1        string `json:"M1"         validate:"required"` // client proof (hex)
}

// TokenResponse — issued after register / login-finalize / refresh.
type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	// M2 is present only in login-finalize; client must verify it.
	M2 string `json:"M2,omitempty"`
}
