package auth

import (
	"time"

	"github.com/google/uuid"

	srppkg "cloud-backend/pkg/srp"
)

// SRPSessionTTL is exported so the repo/memory implementation can use it for cleanup.
const SRPSessionTTL = 5 * time.Minute

type SRPSessEntry struct {
	UserID              uuid.UUID
	Email               string
	SRPSaltHex          string
	AHex                string
	Session             *srppkg.ServerSession
	CryptoSalt          []byte
	BcryptSalt          string
	EncryptedPrivateKey []byte
	KEKEncryptedMaster  []byte
	ExpiresAt           time.Time
}

// SRPSessionManager: implementation lives in repo/memory to keep infrastructure out of the usecase.
type SRPSessionManager interface {
	Store(id uuid.UUID, e *SRPSessEntry) bool
	Consume(id uuid.UUID) (*SRPSessEntry, bool)
}
