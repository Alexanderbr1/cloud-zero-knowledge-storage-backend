package sharing

import "errors"

var (
	ErrNotFound       = errors.New("share not found")
	ErrForbidden      = errors.New("forbidden")
	ErrNoPublicKey    = errors.New("recipient has no public key")
	ErrExpired        = errors.New("share expired")
	ErrRevoked        = errors.New("share revoked")
	ErrDuplicateShare = errors.New("active share already exists for this recipient")
	ErrSelfShare      = errors.New("cannot share a file with yourself")
)
