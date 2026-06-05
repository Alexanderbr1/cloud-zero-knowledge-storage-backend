package entity

import "github.com/google/uuid"

type User struct {
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
