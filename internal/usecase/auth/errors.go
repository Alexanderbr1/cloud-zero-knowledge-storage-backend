package auth

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
	ErrSessionNotFound    = errors.New("session not found")
)
