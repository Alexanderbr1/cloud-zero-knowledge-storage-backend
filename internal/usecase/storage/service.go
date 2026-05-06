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

const dbTimeout = 10 * time.Second

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
	RenameBlob(ctx context.Context, blobID, userID uuid.UUID, name string) (bool, error)
	SearchBlobs(ctx context.Context, userID uuid.UUID, query string) ([]SearchBlobRecord, error)
}

// SearchBlobRecord extends Blob with the folder name for display in search results.
type SearchBlobRecord struct {
	entity.Blob
	FolderName *string
}

type RegisterBlobParams struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FileName         string
	ContentType      string
	ObjectKey        string
	FileSize         int64
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
	SearchFolders(ctx context.Context, userID uuid.UUID, query string) ([]entity.Folder, error)
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
	FileSize         int64
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

	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	if p.FolderID != nil {
		if err := s.requireFolder(dbCtx, *p.FolderID, p.UserID); err != nil {
			return nil, err
		}
	}

	if err := s.Blobs.RegisterBlob(dbCtx, RegisterBlobParams{
		ID: blobID, UserID: p.UserID, FileName: cleanName, ContentType: p.ContentType,
		ObjectKey: objectKey, FileSize: p.FileSize, EncryptedFileKey: p.EncryptedFileKey,
		FileIV: p.FileIV, FolderID: p.FolderID,
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
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
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
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
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
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	blobs, err := s.Blobs.ListBlobs(dbCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("list blobs: %w", err)
	}
	return blobs, nil
}

func (s *Service) ListBlobsInFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]entity.Blob, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	if folderID != nil {
		if err := s.requireFolder(dbCtx, *folderID, userID); err != nil {
			return nil, err
		}
	}

	blobs, err := s.Blobs.ListBlobsInFolder(dbCtx, userID, folderID)
	if err != nil {
		return nil, fmt.Errorf("list blobs in folder: %w", err)
	}
	return blobs, nil
}

func (s *Service) MoveBlob(ctx context.Context, userID, blobID uuid.UUID, folderID *uuid.UUID) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	if folderID != nil {
		if err := s.requireFolder(dbCtx, *folderID, userID); err != nil {
			return err
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

func (s *Service) RenameBlob(ctx context.Context, userID, blobID uuid.UUID, name string) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	name = sanitizeFileName(name)
	ok, err := s.Blobs.RenameBlob(dbCtx, blobID, userID, name)
	if err != nil {
		return fmt.Errorf("rename blob: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// ─── Folder methods ───────────────────────────────────────────────────────────

func (s *Service) CreateFolder(ctx context.Context, p CreateFolderParams) (entity.Folder, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	if p.ParentID != nil {
		if err := s.requireFolder(dbCtx, *p.ParentID, p.UserID); err != nil {
			return entity.Folder{}, err
		}
	}

	f, err := s.Folders.CreateFolder(dbCtx, p)
	if err != nil {
		return entity.Folder{}, fmt.Errorf("create folder: %w", err)
	}
	return f, nil
}

func (s *Service) GetFolder(ctx context.Context, userID, folderID uuid.UUID) (entity.Folder, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
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
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	if parentID != nil {
		if err := s.requireFolder(dbCtx, *parentID, userID); err != nil {
			return nil, err
		}
	}

	folders, err := s.Folders.ListFolders(dbCtx, userID, parentID)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return folders, nil
}

func (s *Service) RenameFolder(ctx context.Context, userID, folderID uuid.UUID, name string) (entity.Folder, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	f, err := s.Folders.RenameFolder(dbCtx, folderID, userID, name)
	if err != nil {
		return entity.Folder{}, fmt.Errorf("rename folder: %w", err)
	}
	return f, nil
}

func (s *Service) MoveFolder(ctx context.Context, p MoveFolderParams) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
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
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	if err := s.Folders.DeleteFolder(dbCtx, folderID, userID); err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	return nil
}

// ─── Search ───────────────────────────────────────────────────────────────────

type SearchParams struct {
	UserID uuid.UUID
	Query  string
}

type SearchResult struct {
	Blobs   []SearchBlobRecord
	Folders []entity.Folder
}

func (s *Service) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	type blobResult struct {
		blobs []SearchBlobRecord
		err   error
	}
	type folderResult struct {
		folders []entity.Folder
		err     error
	}

	blobCh := make(chan blobResult, 1)
	folderCh := make(chan folderResult, 1)

	go func() {
		blobs, err := s.Blobs.SearchBlobs(dbCtx, p.UserID, p.Query)
		blobCh <- blobResult{blobs, err}
	}()
	go func() {
		folders, err := s.Folders.SearchFolders(dbCtx, p.UserID, p.Query)
		folderCh <- folderResult{folders, err}
	}()

	br := <-blobCh
	fr := <-folderCh

	if br.err != nil {
		return SearchResult{}, fmt.Errorf("search blobs: %w", br.err)
	}
	if fr.err != nil {
		return SearchResult{}, fmt.Errorf("search folders: %w", fr.err)
	}
	return SearchResult{Blobs: br.blobs, Folders: fr.folders}, nil
}

// ─── Private helpers ──────────────────────────────────────────────────────────

func (s *Service) requireFolder(ctx context.Context, folderID, userID uuid.UUID) error {
	_, ok, err := s.Folders.GetFolder(ctx, folderID, userID)
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}
	if !ok {
		return ErrFolderNotFound
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
