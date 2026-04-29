package sharing

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

const dbTimeout = 10 * time.Second

func dbCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dbTimeout)
}

// ─── Repository interfaces ────────────────────────────────────────────────

type ShareRepository interface {
	CreateShare(ctx context.Context, p CreateShareParams) (entity.FileShare, error)
	GetShare(ctx context.Context, shareID uuid.UUID) (entity.FileShare, bool, error)
	ListSharedWithUser(ctx context.Context, recipientID uuid.UUID) ([]entity.FileShare, error)
	ListSharesForBlob(ctx context.Context, blobID, ownerID uuid.UUID) ([]entity.FileShare, error)
	RevokeShare(ctx context.Context, shareID, ownerID uuid.UUID) error
}

type UserKeyStore interface {
	// GetPublicKeyByEmail returns the SPKI-encoded P-256 public key for a user.
	// Returns ErrNoPublicKey if the user exists but has no key yet.
	GetPublicKeyByEmail(ctx context.Context, email string) (publicKey []byte, userID uuid.UUID, err error)
}

type BlobStore interface {
	// GetBlobInfo returns object key and metadata for any blob (no ownership check).
	GetBlobInfo(ctx context.Context, blobID uuid.UUID) (BlobInfo, bool, error)
}

type ObjectSigner interface {
	PresignedGetObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
}

// ─── Value objects ────────────────────────────────────────────────────────

type BlobInfo struct {
	ObjectKey   string
	FileName    string
	ContentType string
	FileIV      []byte
	OwnerID     uuid.UUID
}

type CreateShareParams struct {
	BlobID         uuid.UUID
	OwnerID        uuid.UUID
	RecipientEmail string    // input field; resolved to RecipientID before reaching repo
	RecipientID    uuid.UUID // populated by Service.CreateShare before calling Shares.CreateShare
	EphemeralPub   []byte
	WrappedFileKey []byte
	ExpiresAt      *time.Time
}

type SharedFileResult struct {
	Share       entity.FileShare
	DownloadURL string
	FileIV      []byte
	FileName    string
	ContentType string
}

// ─── Service ─────────────────────────────────────────────────────────────

type Service struct {
	Shares     ShareRepository
	Users      UserKeyStore
	Blobs      BlobStore
	Objects    ObjectSigner
	PresignTTL time.Duration
}

// GetRecipientPublicKey looks up a user's SPKI public key by email.
// Used by the client before creating a share (to verify the recipient exists and has a key).
func (s *Service) GetRecipientPublicKey(ctx context.Context, email string) ([]byte, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	pub, _, err := s.Users.GetPublicKeyByEmail(tctx, email)
	if err != nil {
		return nil, err
	}
	return pub, nil
}

// CreateShare stores a new file share record.
// The caller (owner) must already own the blob; the wrapped file key must have
// been derived client-side using ECIES (ephemeral ECDH + HKDF + AES-KW).
func (s *Service) CreateShare(ctx context.Context, p CreateShareParams) (entity.FileShare, error) {
	p.RecipientEmail = strings.TrimSpace(strings.ToLower(p.RecipientEmail))
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	// Verify blob exists and belongs to the owner.
	info, ok, err := s.Blobs.GetBlobInfo(tctx, p.BlobID)
	if err != nil {
		return entity.FileShare{}, fmt.Errorf("get blob: %w", err)
	}
	if !ok || info.OwnerID != p.OwnerID {
		return entity.FileShare{}, ErrNotFound
	}

	tctx2, cancel2 := dbCtx(ctx)
	defer cancel2()

	// Verify recipient exists and has a public key, and get their ID in one query.
	_, recipientID, err := s.Users.GetPublicKeyByEmail(tctx2, p.RecipientEmail)
	if err != nil {
		return entity.FileShare{}, err
	}
	if recipientID == p.OwnerID {
		return entity.FileShare{}, ErrSelfShare
	}

	tctx3, cancel3 := dbCtx(ctx)
	defer cancel3()

	return s.Shares.CreateShare(tctx3, CreateShareParams{
		BlobID:         p.BlobID,
		OwnerID:        p.OwnerID,
		RecipientID:    recipientID, // already resolved — repo does no extra lookup
		EphemeralPub:   p.EphemeralPub,
		WrappedFileKey: p.WrappedFileKey,
		ExpiresAt:      p.ExpiresAt,
	})
}

// GetSharedFile returns the share details and a presigned download URL.
// Only the recipient may call this.
func (s *Service) GetSharedFile(ctx context.Context, shareID, callerID uuid.UUID) (SharedFileResult, error) {
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	share, ok, err := s.Shares.GetShare(tctx, shareID)
	if err != nil {
		return SharedFileResult{}, fmt.Errorf("get share: %w", err)
	}
	if !ok {
		return SharedFileResult{}, ErrNotFound
	}
	if share.RecipientID != callerID {
		return SharedFileResult{}, ErrForbidden
	}
	if share.RevokedAt != nil {
		return SharedFileResult{}, ErrRevoked
	}
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return SharedFileResult{}, ErrExpired
	}

	tctx2, cancel2 := dbCtx(ctx)
	defer cancel2()

	info, ok, err := s.Blobs.GetBlobInfo(tctx2, share.BlobID)
	if err != nil {
		return SharedFileResult{}, fmt.Errorf("get blob: %w", err)
	}
	if !ok {
		return SharedFileResult{}, ErrNotFound
	}

	u, err := s.Objects.PresignedGetObject(ctx, info.ObjectKey, s.PresignTTL)
	if err != nil {
		return SharedFileResult{}, fmt.Errorf("presign: %w", err)
	}

	return SharedFileResult{
		Share:       share,
		DownloadURL: u.String(),
		FileIV:      info.FileIV,
		FileName:    info.FileName,
		ContentType: info.ContentType,
	}, nil
}

// ListSharedWithMe returns all active shares where the caller is the recipient.
func (s *Service) ListSharedWithMe(ctx context.Context, recipientID uuid.UUID) ([]entity.FileShare, error) {
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	shares, err := s.Shares.ListSharedWithUser(tctx, recipientID)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	return shares, nil
}

// ListMyShares returns shares the owner created for a specific blob.
func (s *Service) ListMyShares(ctx context.Context, blobID, ownerID uuid.UUID) ([]entity.FileShare, error) {
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	shares, err := s.Shares.ListSharesForBlob(tctx, blobID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list blob shares: %w", err)
	}
	return shares, nil
}

// RevokeShare soft-deletes a share. Only the owner may revoke.
func (s *Service) RevokeShare(ctx context.Context, shareID, ownerID uuid.UUID) error {
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	if err := s.Shares.RevokeShare(tctx, shareID, ownerID); err != nil {
		return err
	}
	return nil
}
