package storage

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

type Service struct {
	Objects ObjectStore
	Blobs   BlobRegistry
	Folders FolderRegistry

	PresignTTL time.Duration
}

// ─── Object store ─────────────────────────────────────────────────────────────

type ObjectStore interface {
	EnsureBucket(ctx context.Context) error
	PresignedPutObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
	PresignedGetObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
	RemoveObject(ctx context.Context, objectKey string) error
}

// ─── Blob registry ────────────────────────────────────────────────────────────

type BlobRegistry interface {
	RegisterBlob(ctx context.Context, p RegisterBlobParams) error
	GetBlobMeta(ctx context.Context, blobID, userID uuid.UUID) (BlobMeta, bool, error)
	RemoveBlob(ctx context.Context, blobID, userID uuid.UUID) (objectKey string, ok bool, err error)
	ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error)
	ListBlobsInFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]entity.Blob, error)
	MoveBlob(ctx context.Context, blobID, userID uuid.UUID, folderID *uuid.UUID) (bool, error)
}

type RegisterBlobParams struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FileName         string
	ContentType      string
	ObjectKey        string
	EncryptedFileKey []byte
	FileIV           []byte
	FolderID         *uuid.UUID
}

type BlobMeta struct {
	ObjectKey        string
	ContentType      string
	EncryptedFileKey []byte
	FileIV           []byte
}

// ─── Folder registry ──────────────────────────────────────────────────────────

type FolderRegistry interface {
	CreateFolder(ctx context.Context, p CreateFolderParams) (entity.Folder, error)
	GetFolder(ctx context.Context, folderID, userID uuid.UUID) (entity.Folder, bool, error)
	ListFolders(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]entity.Folder, error)
	RenameFolder(ctx context.Context, folderID, userID uuid.UUID, name string) (entity.Folder, error)
	MoveFolder(ctx context.Context, p MoveFolderParams) error
	// DeleteFolder atomically checks that the folder is empty and deletes it.
	// Returns ErrFolderNotEmpty if it has children, ErrFolderNotFound if absent/not owned.
	DeleteFolder(ctx context.Context, folderID, userID uuid.UUID) error
	IsDescendantOf(ctx context.Context, ancestorID, candidateID uuid.UUID) (bool, error)
}

type CreateFolderParams struct {
	UserID   uuid.UUID
	ParentID *uuid.UUID
	Name     string
}

type MoveFolderParams struct {
	FolderID    uuid.UUID
	UserID      uuid.UUID
	NewParentID *uuid.UUID
}

// ─── PresignPut / PresignGet result types ────────────────────────────────────

type PresignPutParams struct {
	UserID           uuid.UUID
	FileName         string
	ContentType      string
	EncryptedFileKey []byte
	FileIV           []byte
	FolderID         *uuid.UUID
}

type PresignPutResult struct {
	BlobID      uuid.UUID
	ObjectKey   string
	UploadURL   string
	ExpiresIn   int64
	HTTPMethod  string
	ContentType string
}

type PresignGetResult struct {
	BlobID           uuid.UUID
	ObjectKey        string
	DownloadURL      string
	ExpiresIn        int64
	HTTPMethod       string
	ContentType      string
	EncryptedFileKey []byte
	FileIV           []byte
}

// ─── Blob methods ─────────────────────────────────────────────────────────────

// PresignPut: object key format — blobs/<user_id>/<blob_id>.
func (s *Service) PresignPut(ctx context.Context, p PresignPutParams) (*PresignPutResult, error) {
	p.ContentType = strings.TrimSpace(p.ContentType)
	blobID := uuid.New()
	cleanName := sanitizeFileName(p.FileName)
	objectKey := fmt.Sprintf("blobs/%s/%s", p.UserID, blobID)

	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if p.FolderID != nil {
		_, ok, err := s.Folders.GetFolder(dbCtx, *p.FolderID, p.UserID)
		if err != nil {
			return nil, fmt.Errorf("get folder: %w", err)
		}
		if !ok {
			return nil, ErrFolderNotFound
		}
	}

	if err := s.Blobs.RegisterBlob(dbCtx, RegisterBlobParams{
		ID: blobID, UserID: p.UserID, FileName: cleanName, ContentType: p.ContentType,
		ObjectKey: objectKey, EncryptedFileKey: p.EncryptedFileKey, FileIV: p.FileIV,
		FolderID: p.FolderID,
	}); err != nil {
		return nil, fmt.Errorf("register blob: %w", err)
	}

	u, err := s.Objects.PresignedPutObject(ctx, objectKey, s.PresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign put: %w", err)
	}

	return &PresignPutResult{
		BlobID:      blobID,
		ObjectKey:   objectKey,
		UploadURL:   u.String(),
		ExpiresIn:   int64(s.PresignTTL.Seconds()),
		HTTPMethod:  "PUT",
		ContentType: p.ContentType,
	}, nil
}

func (s *Service) PresignGet(ctx context.Context, userID, blobID uuid.UUID) (*PresignGetResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	meta, ok, err := s.Blobs.GetBlobMeta(dbCtx, blobID, userID)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	if !ok {
		return nil, ErrNotFound
	}

	u, err := s.Objects.PresignedGetObject(ctx, meta.ObjectKey, s.PresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign get: %w", err)
	}

	return &PresignGetResult{
		BlobID:           blobID,
		ObjectKey:        meta.ObjectKey,
		DownloadURL:      u.String(),
		ExpiresIn:        int64(s.PresignTTL.Seconds()),
		HTTPMethod:       "GET",
		ContentType:      meta.ContentType,
		EncryptedFileKey: meta.EncryptedFileKey,
		FileIV:           meta.FileIV,
	}, nil
}

// DeleteBlob атомарно удаляет запись в БД (DELETE RETURNING), затем объект из MinIO.
// Если запись отсутствует — возвращает ErrNotFound без обращения к MinIO.
// Если объект в MinIO уже отсутствует — операция идемпотентна.
func (s *Service) DeleteBlob(ctx context.Context, userID, blobID uuid.UUID) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	objectKey, ok, err := s.Blobs.RemoveBlob(dbCtx, blobID, userID)
	if err != nil {
		return fmt.Errorf("remove blob record: %w", err)
	}
	if !ok {
		return ErrNotFound
	}

	if err := s.Objects.RemoveObject(ctx, objectKey); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}

func (s *Service) ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	blobs, err := s.Blobs.ListBlobs(dbCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("list blobs: %w", err)
	}
	return blobs, nil
}

func (s *Service) ListBlobsInFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]entity.Blob, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if folderID != nil {
		_, ok, err := s.Folders.GetFolder(dbCtx, *folderID, userID)
		if err != nil {
			return nil, fmt.Errorf("get folder: %w", err)
		}
		if !ok {
			return nil, ErrFolderNotFound
		}
	}

	blobs, err := s.Blobs.ListBlobsInFolder(dbCtx, userID, folderID)
	if err != nil {
		return nil, fmt.Errorf("list blobs in folder: %w", err)
	}
	return blobs, nil
}

func (s *Service) MoveBlob(ctx context.Context, userID, blobID uuid.UUID, folderID *uuid.UUID) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if folderID != nil {
		_, ok, err := s.Folders.GetFolder(dbCtx, *folderID, userID)
		if err != nil {
			return fmt.Errorf("get folder: %w", err)
		}
		if !ok {
			return ErrFolderNotFound
		}
	}

	ok, err := s.Blobs.MoveBlob(dbCtx, blobID, userID, folderID)
	if err != nil {
		return fmt.Errorf("move blob: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// ─── Folder methods ───────────────────────────────────────────────────────────

func (s *Service) CreateFolder(ctx context.Context, p CreateFolderParams) (entity.Folder, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if p.ParentID != nil {
		_, ok, err := s.Folders.GetFolder(dbCtx, *p.ParentID, p.UserID)
		if err != nil {
			return entity.Folder{}, fmt.Errorf("get parent folder: %w", err)
		}
		if !ok {
			return entity.Folder{}, ErrFolderNotFound
		}
	}

	f, err := s.Folders.CreateFolder(dbCtx, p)
	if err != nil {
		return entity.Folder{}, fmt.Errorf("create folder: %w", err)
	}
	return f, nil
}

func (s *Service) GetFolder(ctx context.Context, userID, folderID uuid.UUID) (entity.Folder, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	f, ok, err := s.Folders.GetFolder(dbCtx, folderID, userID)
	if err != nil {
		return entity.Folder{}, fmt.Errorf("get folder: %w", err)
	}
	if !ok {
		return entity.Folder{}, ErrFolderNotFound
	}
	return f, nil
}

func (s *Service) ListFolders(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]entity.Folder, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if parentID != nil {
		_, ok, err := s.Folders.GetFolder(dbCtx, *parentID, userID)
		if err != nil {
			return nil, fmt.Errorf("get parent folder: %w", err)
		}
		if !ok {
			return nil, ErrFolderNotFound
		}
	}

	folders, err := s.Folders.ListFolders(dbCtx, userID, parentID)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return folders, nil
}

func (s *Service) RenameFolder(ctx context.Context, userID, folderID uuid.UUID, name string) (entity.Folder, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	f, err := s.Folders.RenameFolder(dbCtx, folderID, userID, name)
	if err != nil {
		return entity.Folder{}, fmt.Errorf("rename folder: %w", err)
	}
	return f, nil
}

func (s *Service) MoveFolder(ctx context.Context, p MoveFolderParams) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if _, ok, err := s.Folders.GetFolder(dbCtx, p.FolderID, p.UserID); err != nil {
		return fmt.Errorf("get folder: %w", err)
	} else if !ok {
		return ErrFolderNotFound
	}

	if p.NewParentID != nil {
		if *p.NewParentID == p.FolderID {
			return ErrFolderCycle
		}

		if _, ok, err := s.Folders.GetFolder(dbCtx, *p.NewParentID, p.UserID); err != nil {
			return fmt.Errorf("get new parent: %w", err)
		} else if !ok {
			return ErrFolderNotFound
		}

		// Prevent cycle: new parent must not be a descendant of the folder being moved.
		isDesc, err := s.Folders.IsDescendantOf(dbCtx, p.FolderID, *p.NewParentID)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if isDesc {
			return ErrFolderCycle
		}
	}

	if err := s.Folders.MoveFolder(dbCtx, p); err != nil {
		return fmt.Errorf("move folder: %w", err)
	}
	return nil
}

func (s *Service) DeleteFolder(ctx context.Context, userID, folderID uuid.UUID) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if err := s.Folders.DeleteFolder(dbCtx, folderID, userID); err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file.bin"
	}
	// filepath.Base strips all leading path segments on Unix ('/' separator).
	// Backslashes are not path separators on Linux, so replace them explicitly
	// to handle filenames coming from Windows clients.
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "." || name == ".." {
		return "file.bin"
	}
	return name
}
