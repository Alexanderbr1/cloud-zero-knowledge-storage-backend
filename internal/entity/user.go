package entity

import "github.com/google/uuid"

// User — учётная запись для JWT-аутентификации.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CryptoSalt   []byte // соль для PBKDF2 key derivation; nil у старых аккаунтов
}
