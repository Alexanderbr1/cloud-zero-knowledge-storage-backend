package sharing

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

func shareFor(recipientID uuid.UUID) entity.FileShareView {
	return entity.FileShareView{
		FileShare: entity.FileShare{
			RecipientID: recipientID,
		},
	}
}

func TestValidateShareAccess_HappyPath(t *testing.T) {
	callerID := uuid.New()
	share := shareFor(callerID)

	if err := validateShareAccess(share, callerID); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestValidateShareAccess_WrongCaller(t *testing.T) {
	share := shareFor(uuid.New())

	err := validateShareAccess(share, uuid.New()) // different caller
	if err != ErrForbidden {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestValidateShareAccess_Revoked(t *testing.T) {
	callerID := uuid.New()
	share := shareFor(callerID)
	now := time.Now()
	share.RevokedAt = &now

	err := validateShareAccess(share, callerID)
	if err != ErrRevoked {
		t.Fatalf("want ErrRevoked, got %v", err)
	}
}

func TestValidateShareAccess_Expired(t *testing.T) {
	callerID := uuid.New()
	share := shareFor(callerID)
	past := time.Now().Add(-time.Hour)
	share.ExpiresAt = &past

	err := validateShareAccess(share, callerID)
	if err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestValidateShareAccess_FutureExpiry_Allowed(t *testing.T) {
	callerID := uuid.New()
	share := shareFor(callerID)
	future := time.Now().Add(time.Hour)
	share.ExpiresAt = &future

	if err := validateShareAccess(share, callerID); err != nil {
		t.Fatalf("share with future expiry should be valid, got %v", err)
	}
}

// Revoked check must fire before expired check — order matters for UX error messages.
func TestValidateShareAccess_RevokedBeforeExpired(t *testing.T) {
	callerID := uuid.New()
	share := shareFor(callerID)
	past := time.Now().Add(-time.Hour)
	share.ExpiresAt = &past
	share.RevokedAt = &past

	err := validateShareAccess(share, callerID)
	if err != ErrRevoked {
		t.Fatalf("want ErrRevoked (takes priority over ErrExpired), got %v", err)
	}
}
