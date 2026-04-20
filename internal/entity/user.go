package entity

import "github.com/google/uuid"

// User — учётная запись для JWT-аутентификации.
type User struct {
	ID          uuid.UUID
	Email       string
	SRPSalt     string // hex-encoded raw SRP salt bytes
	SRPVerifier string // hex-encoded SRP-6a verifier v = g^x mod N
	BcryptSalt  string // bcrypt salt string used to harden x = H(srpSalt || bcrypt(password, bcryptSalt))
	CryptoSalt  []byte // PBKDF2 salt for client-side master key derivation
}
