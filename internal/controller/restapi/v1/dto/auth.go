package dto

// RegisterRequest — POST /v1/auth/register
type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email,max=320"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

// LoginRequest — POST /v1/auth/login
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=320"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

// RefreshRequest — POST /v1/auth/refresh
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// LogoutRequest — POST /v1/auth/logout
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"omitempty"`
}

// TokenResponse — ответ после register/login/refresh
type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
}
