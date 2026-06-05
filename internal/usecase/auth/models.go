package auth

import (
	"time"

	"github.com/google/uuid"
)

type RegisterParams struct {
	Email                string
	SRPSalt              string
	SRPVerifier          string
	BcryptSalt           string
	CryptoSalt           []byte
	PublicKey            []byte
	EncryptedPrivateKey  []byte
	KEKEncryptedMaster   []byte
	KEKEncryptedRecovery []byte
	RecoverySalt         []byte
	Device               DeviceInfo
}

type LoginFinalizeParams struct {
	SessionID string
	M1        string
	Device    DeviceInfo
}

type NewUserParams struct {
	ID                   uuid.UUID
	Email                string
	SRPSalt              string
	SRPVerifier          string
	BcryptSalt           string
	CryptoSalt           []byte
	PublicKey            []byte
	EncryptedPrivateKey  []byte
	KEKEncryptedMaster   []byte
	KEKEncryptedRecovery []byte
	RecoverySalt         []byte
}

type RecoveryData struct {
	UserID               uuid.UUID
	KEKEncryptedRecovery []byte
	RecoverySalt         []byte
}

type ResetPasswordParams struct {
	SRPSalt              string
	SRPVerifier          string
	BcryptSalt           string
	CryptoSalt           []byte
	KEKEncryptedMaster   []byte
	KEKEncryptedRecovery []byte
	RecoverySalt         []byte
}

type RefreshSessionParams struct {
	SessionID       uuid.UUID
	UserID          uuid.UUID
	DeviceSessionID uuid.UUID
	TokenHash       []byte
	ClientKey       []byte
	ExpiresAt       time.Time
}

type ConsumedSession struct {
	SessionID       uuid.UUID
	UserID          uuid.UUID
	DeviceSessionID uuid.UUID
	ClientKey       []byte
}

type DeviceInfo struct {
	UserAgent  string
	IPAddress  string
	DeviceName string
	DeviceID   string
}

type TokenPair struct {
	AccessToken      string
	AccessExpiresIn  int64
	RefreshToken     string
	RefreshExpiresIn int64
	DeviceSessionID  uuid.UUID
	ClientKey        []byte
}

type LoginInitResult struct {
	SessionID  string
	SRPSalt    string
	BcryptSalt string
	B          string
	CryptoSalt []byte
}

type LoginFinalizeResult struct {
	M2                  string
	Pair                TokenPair
	EncryptedPrivateKey []byte
	KEKEncryptedMaster  []byte
	UserID              uuid.UUID
}
