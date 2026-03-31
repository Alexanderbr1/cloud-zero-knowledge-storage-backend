package entity

import "github.com/google/uuid"

// User — учётная запись для JWT-аутентификации.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
}
