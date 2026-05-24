package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	authuc "cloud-backend/internal/usecase/auth"
	srppkg "cloud-backend/pkg/srp"
)

const srpKeyPrefix = "srp_session:"

type srpSessionStore struct {
	client *redis.Client
}

func NewSRPSessionStore(client *redis.Client) authuc.SRPSessionManager {
	return &srpSessionStore{client: client}
}

// srpRecord is the JSON-serializable form of SRPSessEntry.
type srpRecord struct {
	UserID              string `json:"uid"`
	Email               string `json:"email"`
	SRPSaltHex          string `json:"srp_salt"`
	AHex                string `json:"a"`
	VerifierHex         string `json:"v"`
	BHex                string `json:"b"`
	BPubHex             string `json:"B"`
	CryptoSalt          string `json:"crypto_salt"`   // base64
	BcryptSalt          string `json:"bcrypt_salt"`
	EncryptedPrivateKey string `json:"epk"`           // base64
	KEKEncryptedMaster  string `json:"kek"`           // base64
	ExpiresAt           int64  `json:"exp"`
}

func (s *srpSessionStore) Store(id uuid.UUID, e *authuc.SRPSessEntry) bool {
	verifierHex, bHex, BHex := e.Session.Snapshot()
	rec := srpRecord{
		UserID:              e.UserID.String(),
		Email:               e.Email,
		SRPSaltHex:          e.SRPSaltHex,
		AHex:                e.AHex,
		VerifierHex:         verifierHex,
		BHex:                bHex,
		BPubHex:             BHex,
		CryptoSalt:          base64.StdEncoding.EncodeToString(e.CryptoSalt),
		BcryptSalt:          e.BcryptSalt,
		EncryptedPrivateKey: base64.StdEncoding.EncodeToString(e.EncryptedPrivateKey),
		KEKEncryptedMaster:  base64.StdEncoding.EncodeToString(e.KEKEncryptedMaster),
		ExpiresAt:           e.ExpiresAt.UnixMilli(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return false
	}
	ttl := time.Until(e.ExpiresAt)
	if ttl <= 0 {
		return false
	}
	err = s.client.Set(context.Background(), srpKeyPrefix+id.String(), data, ttl).Err()
	return err == nil
}

func (s *srpSessionStore) Consume(id uuid.UUID) (*authuc.SRPSessEntry, bool) {
	key := srpKeyPrefix + id.String()

	// Atomic get-and-delete: use a pipeline so no second caller can consume the same session.
	var getCmd *redis.StringCmd
	_, err := s.client.TxPipelined(context.Background(), func(pipe redis.Pipeliner) error {
		getCmd = pipe.GetDel(context.Background(), key)
		return nil
	})
	if err != nil {
		return nil, false
	}

	data, err := getCmd.Bytes()
	if err != nil {
		return nil, false
	}

	var rec srpRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}

	if time.Now().UnixMilli() > rec.ExpiresAt {
		return nil, false
	}

	userID, err := uuid.Parse(rec.UserID)
	if err != nil {
		return nil, false
	}

	session, err := srppkg.NewServerSessionFromSnapshot(rec.VerifierHex, rec.BHex, rec.BPubHex)
	if err != nil {
		return nil, false
	}

	cryptoSalt, _ := base64.StdEncoding.DecodeString(rec.CryptoSalt)
	epk, _         := base64.StdEncoding.DecodeString(rec.EncryptedPrivateKey)
	kek, _         := base64.StdEncoding.DecodeString(rec.KEKEncryptedMaster)

	return &authuc.SRPSessEntry{
		UserID:              userID,
		Email:               rec.Email,
		SRPSaltHex:          rec.SRPSaltHex,
		AHex:                rec.AHex,
		Session:             session,
		CryptoSalt:          cryptoSalt,
		BcryptSalt:          rec.BcryptSalt,
		EncryptedPrivateKey: epk,
		KEKEncryptedMaster:  kek,
		ExpiresAt:           time.UnixMilli(rec.ExpiresAt),
	}, true
}
