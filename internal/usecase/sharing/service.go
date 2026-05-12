package sharing

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/entity"
)

const dbTimeout = 10 * time.Second

func dbCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dbTimeout)
}

// ─── Repository interfaces ────────────────────────────────────────────────

type ShareRepository interface {
	CreateShare(ctx context.Context, p CreateShareParams) (entity.FileShareView, error)
	GetShare(ctx context.Context, shareID uuid.UUID) (entity.FileShareView, bool, error)
	ListSharedWithUser(ctx context.Context, recipientID uuid.UUID) ([]entity.FileShareView, error)
	ListSharesForBlob(ctx context.Context, blobID, ownerID uuid.UUID) ([]entity.FileShareView, error)
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
	Share       entity.FileShareView
	DownloadURL string
	FileIV      []byte
	FileName    string
	ContentType string
}

// Notifier sends email notifications for sharing events.
type Notifier interface {
	NotifyNewShare(ctx context.Context, toEmail, ownerEmail, fileName string) error
}

// ─── Service ─────────────────────────────────────────────────────────────

type Service struct {
	Shares     ShareRepository
	Users      UserKeyStore
	Blobs      BlobStore
	Objects    ObjectSigner
	PresignTTL time.Duration
	Notifier   Notifier
	Logger     zerolog.Logger
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
func (s *Service) CreateShare(ctx context.Context, p CreateShareParams) (entity.FileShareView, error) {
	p.RecipientEmail = strings.TrimSpace(strings.ToLower(p.RecipientEmail))
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	info, ok, err := s.Blobs.GetBlobInfo(tctx, p.BlobID)
	if err != nil {
		return entity.FileShareView{}, fmt.Errorf("get blob: %w", err)
	}
	if !ok || info.OwnerID != p.OwnerID {
		return entity.FileShareView{}, ErrNotFound
	}

	_, recipientID, err := s.Users.GetPublicKeyByEmail(tctx, p.RecipientEmail)
	if err != nil {
		return entity.FileShareView{}, err
	}
	if recipientID == p.OwnerID {
		return entity.FileShareView{}, ErrSelfShare
	}

	share, err := s.Shares.CreateShare(tctx, CreateShareParams{
		BlobID:         p.BlobID,
		OwnerID:        p.OwnerID,
		RecipientID:    recipientID,
		EphemeralPub:   p.EphemeralPub,
		WrappedFileKey: p.WrappedFileKey,
		ExpiresAt:      p.ExpiresAt,
	})
	if err != nil {
		return entity.FileShareView{}, err
	}

	go func() {
		nctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Notifier.NotifyNewShare(nctx, share.RecipientEmail, share.OwnerEmail, share.BlobFileName); err != nil {
			s.Logger.Warn().Err(err).Msg("share notification failed")
		}
	}()

	return share, nil
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
	if err := validateShareAccess(share, callerID); err != nil {
		return SharedFileResult{}, err
	}

	info, ok, err := s.Blobs.GetBlobInfo(tctx, share.BlobID)
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

func validateShareAccess(share entity.FileShareView, callerID uuid.UUID) error {
	if share.RecipientID != callerID {
		return ErrForbidden
	}
	if share.RevokedAt != nil {
		return ErrRevoked
	}
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return ErrExpired
	}
	return nil
}

// ListSharedWithMe returns all active shares where the caller is the recipient.
func (s *Service) ListSharedWithMe(ctx context.Context, recipientID uuid.UUID) ([]entity.FileShareView, error) {
	tctx, cancel := dbCtx(ctx)
	defer cancel()

	shares, err := s.Shares.ListSharedWithUser(tctx, recipientID)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	return shares, nil
}

// ListMyShares returns shares the owner created for a specific blob.
func (s *Service) ListMyShares(ctx context.Context, blobID, ownerID uuid.UUID) ([]entity.FileShareView, error) {
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
	return s.Shares.RevokeShare(tctx, shareID, ownerID)
}
