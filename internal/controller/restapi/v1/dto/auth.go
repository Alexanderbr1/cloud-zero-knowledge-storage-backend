package dto

type RegisterRequest struct {
	Email                string `json:"email"                   validate:"required,email,max=320"`
	SRPSalt              string `json:"srp_salt"                validate:"required"`
	SRPVerifier          string `json:"srp_verifier"            validate:"required"`
	BcryptSalt           string `json:"bcrypt_salt"             validate:"required"`
	CryptoSalt           string `json:"crypto_salt"             validate:"required"` // base64 PBKDF2 salt
	PublicKey            string `json:"public_key"              validate:"required"` // base64 SPKI P-256
	EncryptedPrivateKey  string `json:"encrypted_private_key"   validate:"required"` // base64 wrapped private key
	KEKEncryptedMaster   string `json:"kek_encrypted_master"    validate:"required"` // base64 AES-KW(masterKey, KEK)
	KEKEncryptedRecovery string `json:"kek_encrypted_recovery"  validate:"required"` // base64 AES-KW(recoveryKey, KEK)
	RecoverySalt         string `json:"recovery_salt"           validate:"required"` // base64 PBKDF2 salt for recovery key
}

type LoginInitRequest struct {
	Email string `json:"email" validate:"required,email,max=320"`
	A     string `json:"A"     validate:"required"` // client public ephemeral (hex)
}

type LoginInitResponse struct {
	SessionID  string `json:"session_id"`
	SRPSalt    string `json:"srp_salt"`    // hex
	BcryptSalt string `json:"bcrypt_salt"` // $2b$10$...
	B          string `json:"B"`           // server public ephemeral (hex)
	CryptoSalt string `json:"crypto_salt"` // base64 PBKDF2 salt
}

type LoginFinalizeRequest struct {
	SessionID string `json:"session_id" validate:"required"`
	M1        string `json:"M1"         validate:"required"` // client proof (hex)
}

type CryptoSaltResponse struct {
	CryptoSalt         string `json:"crypto_salt"`
	KEKEncryptedMaster string `json:"kek_encrypted_master"`
}

type TokenResponse struct {
	AccessToken         string `json:"access_token"`
	ExpiresIn           int64  `json:"expires_in"`
	RefreshExpiresIn    int64  `json:"refresh_expires_in"`
	TokenType           string `json:"token_type"`
	M2                  string `json:"M2,omitempty"`                     // login-finalize only; client must verify
	EncryptedPrivateKey string `json:"encrypted_private_key,omitempty"`  // login-finalize only
	KEKEncryptedMaster  string `json:"kek_encrypted_master,omitempty"`   // login-finalize only
	ClientKey           string `json:"client_key,omitempty"`             // base64 AES-GCM key for password blob persistence
}

type ResetPasswordRequestRequest struct {
	Email string `json:"email" validate:"required,email,max=320"`
}

type RecoveryDataRequest struct {
	Token string `json:"token" validate:"required"`
}

type RecoveryDataResponse struct {
	KEKEncryptedRecovery string `json:"kek_encrypted_recovery"` // base64
	RecoverySalt         string `json:"recovery_salt"`          // base64
}

type ResetPasswordConfirmRequest struct {
	Token                string `json:"token"                  validate:"required"`
	SRPSalt              string `json:"srp_salt"               validate:"required"`
	SRPVerifier          string `json:"srp_verifier"           validate:"required"`
	BcryptSalt           string `json:"bcrypt_salt"            validate:"required"`
	CryptoSalt           string `json:"crypto_salt"            validate:"required"`
	KEKEncryptedMaster   string `json:"kek_encrypted_master"   validate:"required"`
}

