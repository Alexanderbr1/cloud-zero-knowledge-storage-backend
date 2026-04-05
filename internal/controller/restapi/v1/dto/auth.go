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
}

// CryptoParamsResponse — публичный ответ на GET /auth/crypto-params?email=...
// Возвращает crypto_salt для деривации мастер-ключа на клиенте до логина.
type CryptoParamsResponse struct {
	CryptoSalt string `json:"crypto_salt"` // base64-encoded 16 байт
}
