package storage

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"cloud-backend/internal/entity"
)

type ObjectStore interface {
	EnsureBucket(ctx context.Context) error
	PresignedPutObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
	PresignedGetObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
	RemoveObject(ctx context.Context, objectKey string) error
	StatObject(ctx context.Context, objectKey string) (int64, error)
}

type BlobRepo interface {
	GetUserStorageUsed(ctx context.Context, userID uuid.UUID) (int64, error)
	RegisterBlob(ctx context.Context, p RegisterBlobParams) error
	GetPendingBlobMeta(ctx context.Context, blobID, userID uuid.UUID) (PendingBlobMeta, bool, error)
	ConfirmBlobUploadWithSize(ctx context.Context, blobID, userID uuid.UUID, actualSize int64) (bool, error)
	PurgeOrphanedBlobs(ctx context.Context, before time.Time) ([]string, error)
	GetBlobMeta(ctx context.Context, blobID, userID uuid.UUID) (BlobMeta, bool, error)
	RemoveBlob(ctx context.Context, blobID, userID uuid.UUID) (objectKey string, ok bool, err error)
	ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error)
	ListBlobsInFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]entity.Blob, error)
	MoveBlob(ctx context.Context, blobID, userID uuid.UUID, folderID *uuid.UUID) (MoveBlobInfo, bool, error)
	RenameBlob(ctx context.Context, blobID, userID uuid.UUID, name string) (bool, error)
	SearchBlobs(ctx context.Context, userID uuid.UUID, query string) ([]SearchBlobRecord, error)
	TrashBlob(ctx context.Context, blobID, userID uuid.UUID) (fileName string, ok bool, err error)
}

type BlobTrashRepo interface {
	RestoreBlob(ctx context.Context, blobID, userID uuid.UUID) (fileName string, ok bool, err error)
	HardDeleteBlob(ctx context.Context, blobID, userID uuid.UUID) (objectKey, fileName string, ok bool, err error)
	ListTrashedBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error)
	EmptyTrashedBlobs(ctx context.Context, userID uuid.UUID) ([]string, error)
	PurgeExpiredBlobs(ctx context.Context, before time.Time) ([]string, error)
}

type FolderRepo interface {
	CreateFolder(ctx context.Context, p CreateFolderParams) (entity.Folder, error)
	GetFolder(ctx context.Context, folderID, userID uuid.UUID) (entity.Folder, bool, error)
	ListFolders(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]entity.Folder, error)
	RenameFolder(ctx context.Context, folderID, userID uuid.UUID, name string) (entity.Folder, error)
	MoveFolder(ctx context.Context, p MoveFolderParams) error
	IsDescendantOf(ctx context.Context, ancestorID, candidateID uuid.UUID) (bool, error)
	SearchFolders(ctx context.Context, userID uuid.UUID, query string) ([]entity.Folder, error)
	TrashFolder(ctx context.Context, folderID, userID uuid.UUID) (bool, error)
}

type FolderTrashRepo interface {
	RestoreFolder(ctx context.Context, folderID, userID uuid.UUID) (name string, ok bool, err error)
	HardDeleteFolder(ctx context.Context, folderID, userID uuid.UUID) (objectKeys []string, name string, ok bool, err error)
	ListTrashedFolders(ctx context.Context, userID uuid.UUID) ([]entity.Folder, error)
	EmptyTrashedFolders(ctx context.Context, userID uuid.UUID) ([]string, error)
	PurgeExpiredFolders(ctx context.Context, before time.Time) ([]string, error)
}

const dbTimeout = 10 * time.Second

type Service struct {
	Objects     ObjectStore
	Blobs       BlobRepo
	BlobTrash   BlobTrashRepo
	Folders     FolderRepo
	FolderTrash FolderTrashRepo

	PresignTTL     time.Duration
	MaxUploadBytes int64 // 0 = unlimited
	QuotaBytes     int64 // 0 = unlimited
}

type StorageUsage struct {
	UsedBytes  int64
	QuotaBytes int64
}

func (s *Service) GetStorageUsage(ctx context.Context, userID uuid.UUID) (StorageUsage, error) {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	used, err := s.Blobs.GetUserStorageUsed(dbCtx, userID)
	if err != nil {
		return StorageUsage{}, fmt.Errorf("get storage used: %w", err)
	}
	return StorageUsage{UsedBytes: used, QuotaBytes: s.QuotaBytes}, nil
}

func (s *Service) PresignPut(ctx context.Context, p PresignPutParams) (*PresignPutResult, error) {
	p.ContentType = strings.TrimSpace(p.ContentType)
	blobID := uuid.New()
	cleanName := sanitizeFileName(p.FileName)
	objectKey := fmt.Sprintf("%s/%s", p.UserID, blobID)

	if s.MaxUploadBytes > 0 && p.FileSize > s.MaxUploadBytes {
		return nil, ErrFileTooLarge
	}

	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	if s.QuotaBytes > 0 {
		used, err := s.Blobs.GetUserStorageUsed(dbCtx, p.UserID)
		if err != nil {
			return nil, fmt.Errorf("get storage used: %w", err)
		}
		if used+p.FileSize > s.QuotaBytes {
			return nil, ErrQuotaExceeded
		}
	}

	if p.FolderID != nil {
		if err := s.requireFolder(dbCtx, *p.FolderID, p.UserID); err != nil {
			return nil, err
		}
	}

	if err := s.Blobs.RegisterBlob(dbCtx, RegisterBlobParams{
		ID:               blobID,
		UserID:           p.UserID,
		FileName:         cleanName,
		ContentType:      p.ContentType,
		ObjectKey:        objectKey,
		FileSize:         p.FileSize,
		EncryptedFileKey: p.EncryptedFileKey,
		FileIV:           p.FileIV,
		FolderID:         p.FolderID,
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
		FileName:         meta.FileName,
	}, nil
}

func (s *Service) DeleteBlob(ctx context.Context, userID, blobID uuid.UUID) (string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	fileName, ok, err := s.Blobs.TrashBlob(dbCtx, blobID, userID)
	if err != nil {
		return "", fmt.Errorf("trash blob: %w", err)
	}
	if !ok {
		return "", ErrNotFound
	}
	return fileName, nil
}

func (s *Service) RestoreBlob(ctx context.Context, userID, blobID uuid.UUID) (string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	fileName, ok, err := s.BlobTrash.RestoreBlob(dbCtx, blobID, userID)
	if err != nil {
		return "", fmt.Errorf("restore blob: %w", err)
	}
	if !ok {
		return "", ErrNotFound
	}
	return fileName, nil
}

func (s *Service) HardDeleteBlob(ctx context.Context, userID, blobID uuid.UUID) (string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	objectKey, fileName, ok, err := s.BlobTrash.HardDeleteBlob(dbCtx, blobID, userID)
	if err != nil {
		return "", fmt.Errorf("hard delete blob record: %w", err)
	}
	if !ok {
		return "", ErrNotFound
	}

	if err := s.Objects.RemoveObject(ctx, objectKey); err != nil {
		return "", fmt.Errorf("remove object: %w", err)
	}
	return fileName, nil
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

func (s *Service) MoveBlob(ctx context.Context, userID, blobID uuid.UUID, folderID *uuid.UUID) (MoveBlobInfo, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	var dstFolderName string
	if folderID != nil {
		folder, ok, err := s.Folders.GetFolder(dbCtx, *folderID, userID)
		if err != nil {
			return MoveBlobInfo{}, fmt.Errorf("get folder: %w", err)
		}
		if !ok {
			return MoveBlobInfo{}, ErrFolderNotFound
		}
		dstFolderName = folder.Name
	}

	info, ok, err := s.Blobs.MoveBlob(dbCtx, blobID, userID, folderID)
	if err != nil {
		return MoveBlobInfo{}, fmt.Errorf("move blob: %w", err)
	}
	if !ok {
		return MoveBlobInfo{}, ErrNotFound
	}
	info.DstFolderName = dstFolderName
	return info, nil
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

func (s *Service) CreateFolder(ctx context.Context, p CreateFolderParams) (entity.Folder, string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	var parentFolderName string
	if p.ParentID != nil {
		parent, ok, err := s.Folders.GetFolder(dbCtx, *p.ParentID, p.UserID)
		if err != nil {
			return entity.Folder{}, "", fmt.Errorf("get parent folder: %w", err)
		}
		if !ok {
			return entity.Folder{}, "", ErrFolderNotFound
		}
		parentFolderName = parent.Name
	}

	f, err := s.Folders.CreateFolder(dbCtx, p)
	if err != nil {
		return entity.Folder{}, "", fmt.Errorf("create folder: %w", err)
	}
	return f, parentFolderName, nil
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

func (s *Service) MoveFolder(ctx context.Context, p MoveFolderParams) (FolderMoveInfo, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	folder, ok, err := s.Folders.GetFolder(dbCtx, p.FolderID, p.UserID)
	if err != nil {
		return FolderMoveInfo{}, fmt.Errorf("get folder: %w", err)
	}
	if !ok {
		return FolderMoveInfo{}, ErrFolderNotFound
	}

	// Resolve current (source) parent folder name — one extra query, but cheap.
	var srcFolderName string
	if folder.ParentID != nil {
		if srcFolder, ok, err := s.Folders.GetFolder(dbCtx, *folder.ParentID, p.UserID); err == nil && ok {
			srcFolderName = srcFolder.Name
		}
	}

	var dstFolderName string
	if p.NewParentID != nil {
		if *p.NewParentID == p.FolderID {
			return FolderMoveInfo{}, ErrFolderCycle
		}

		dstFolder, ok, err := s.Folders.GetFolder(dbCtx, *p.NewParentID, p.UserID)
		if err != nil {
			return FolderMoveInfo{}, fmt.Errorf("get new parent: %w", err)
		}
		if !ok {
			return FolderMoveInfo{}, ErrFolderNotFound
		}
		dstFolderName = dstFolder.Name

		isDesc, err := s.Folders.IsDescendantOf(dbCtx, p.FolderID, *p.NewParentID)
		if err != nil {
			return FolderMoveInfo{}, fmt.Errorf("cycle check: %w", err)
		}
		if isDesc {
			return FolderMoveInfo{}, ErrFolderCycle
		}
	}

	if err := s.Folders.MoveFolder(dbCtx, p); err != nil {
		return FolderMoveInfo{}, fmt.Errorf("move folder: %w", err)
	}
	return FolderMoveInfo{
		FolderName:    folder.Name,
		SrcFolderName: srcFolderName,
		DstFolderName: dstFolderName,
	}, nil
}

func (s *Service) DeleteFolder(ctx context.Context, userID, folderID uuid.UUID) (string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	folder, ok, err := s.Folders.GetFolder(dbCtx, folderID, userID)
	if err != nil {
		return "", fmt.Errorf("get folder: %w", err)
	}
	if !ok {
		return "", ErrFolderNotFound
	}

	trashed, err := s.Folders.TrashFolder(dbCtx, folderID, userID)
	if err != nil {
		return "", fmt.Errorf("trash folder: %w", err)
	}
	if !trashed {
		return "", ErrFolderNotFound
	}
	return folder.Name, nil
}

func (s *Service) RestoreFolder(ctx context.Context, userID, folderID uuid.UUID) (string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	name, ok, err := s.FolderTrash.RestoreFolder(dbCtx, folderID, userID)
	if err != nil {
		return "", fmt.Errorf("restore folder: %w", err)
	}
	if !ok {
		return "", ErrFolderNotFound
	}
	return name, nil
}

func (s *Service) HardDeleteFolder(ctx context.Context, userID, folderID uuid.UUID) (string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	objectKeys, name, ok, err := s.FolderTrash.HardDeleteFolder(dbCtx, folderID, userID)
	if err != nil {
		return "", fmt.Errorf("hard delete folder: %w", err)
	}
	if !ok {
		return "", ErrFolderNotFound
	}

	for _, key := range objectKeys {
		_ = s.Objects.RemoveObject(ctx, key) // best-effort
	}
	return name, nil
}

func (s *Service) ListTrash(ctx context.Context, userID uuid.UUID) (TrashResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	g, gctx := errgroup.WithContext(dbCtx)
	var blobs []entity.Blob
	var folders []entity.Folder

	g.Go(func() error {
		var err error
		blobs, err = s.BlobTrash.ListTrashedBlobs(gctx, userID)
		if err != nil {
			return fmt.Errorf("list trashed blobs: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		folders, err = s.FolderTrash.ListTrashedFolders(gctx, userID)
		if err != nil {
			return fmt.Errorf("list trashed folders: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return TrashResult{}, err
	}
	return TrashResult{Blobs: blobs, Folders: folders}, nil
}

func (s *Service) EmptyTrash(ctx context.Context, userID uuid.UUID) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	blobKeys, err := s.BlobTrash.EmptyTrashedBlobs(dbCtx, userID)
	if err != nil {
		return fmt.Errorf("empty trashed blobs: %w", err)
	}

	folderBlobKeys, err := s.FolderTrash.EmptyTrashedFolders(dbCtx, userID)
	if err != nil {
		return fmt.Errorf("empty trashed folders: %w", err)
	}

	for _, key := range append(blobKeys, folderBlobKeys...) {
		_ = s.Objects.RemoveObject(ctx, key)
	}
	return nil
}

func (s *Service) ConfirmUpload(ctx context.Context, userID, blobID uuid.UUID) (fileName, folderName string, err error) {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	pending, ok, err := s.Blobs.GetPendingBlobMeta(dbCtx, blobID, userID)
	if err != nil {
		return "", "", fmt.Errorf("get pending blob: %w", err)
	}
	if !ok {
		return "", "", ErrNotFound
	}

	actualSize, err := s.Objects.StatObject(ctx, pending.ObjectKey)
	if err != nil {
		return "", "", fmt.Errorf("stat object: %w", err)
	}

	if s.QuotaBytes > 0 && actualSize != pending.DeclaredSize {
		used, err := s.Blobs.GetUserStorageUsed(dbCtx, userID)
		if err != nil {
			return "", "", fmt.Errorf("get storage used: %w", err)
		}
		// used already includes the pending blob's declared size — replace it with actual.
		if used-pending.DeclaredSize+actualSize > s.QuotaBytes {
			_ = s.Objects.RemoveObject(ctx, pending.ObjectKey)
			return "", "", ErrQuotaExceeded
		}
	}

	confirmed, err := s.Blobs.ConfirmBlobUploadWithSize(dbCtx, blobID, userID, actualSize)
	if err != nil {
		return "", "", fmt.Errorf("confirm upload: %w", err)
	}
	if !confirmed {
		return "", "", ErrNotFound
	}

	if pending.FolderID != nil {
		enrichCtx, enrichCancel := context.WithTimeout(context.Background(), dbTimeout)
		if folder, ok, ferr := s.Folders.GetFolder(enrichCtx, *pending.FolderID, userID); ferr == nil && ok {
			folderName = folder.Name
		}
		enrichCancel()
	}
	return pending.FileName, folderName, nil
}

func (s *Service) CleanOrphanedBlobs(ctx context.Context, ttl time.Duration) error {
	before := time.Now().Add(-ttl)
	keys, err := s.Blobs.PurgeOrphanedBlobs(ctx, before)
	if err != nil {
		return fmt.Errorf("purge orphaned blobs: %w", err)
	}
	for _, key := range keys {
		_ = s.Objects.RemoveObject(ctx, key)
	}
	return nil
}

func (s *Service) CleanExpiredTrash(ctx context.Context, ttl time.Duration) error {
	before := time.Now().Add(-ttl)

	blobKeys, err := s.BlobTrash.PurgeExpiredBlobs(ctx, before)
	if err != nil {
		return fmt.Errorf("purge expired blobs: %w", err)
	}

	folderBlobKeys, err := s.FolderTrash.PurgeExpiredFolders(ctx, before)
	if err != nil {
		return fmt.Errorf("purge expired folders: %w", err)
	}

	for _, key := range append(blobKeys, folderBlobKeys...) {
		_ = s.Objects.RemoveObject(ctx, key)
	}
	return nil
}

func (s *Service) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	g, gctx := errgroup.WithContext(dbCtx)
	var blobs []SearchBlobRecord
	var folders []entity.Folder

	g.Go(func() error {
		var err error
		blobs, err = s.Blobs.SearchBlobs(gctx, p.UserID, p.Query)
		if err != nil {
			return fmt.Errorf("search blobs: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		folders, err = s.Folders.SearchFolders(gctx, p.UserID, p.Query)
		if err != nil {
			return fmt.Errorf("search folders: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Blobs: blobs, Folders: folders}, nil
}

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
