package dto

type RegisterRequest struct {
	Email      string `json:"email"       validate:"required,email,max=320"`
	Password   string `json:"password"    validate:"required,min=8,max=128"`
	CryptoSalt string `json:"crypto_salt" validate:"required"`
}

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email,max=320"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	// CryptoSalt — только в ответах register/login (base64); в refresh отсутствует.
	CryptoSalt string `json:"crypto_salt,omitempty"`
}
