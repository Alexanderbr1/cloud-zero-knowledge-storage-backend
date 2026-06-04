package v1_test

// ─── This file declares mock implementations for every service interface used
// by handlers that are NOT auth. Mocks for AuthService and ParseBearerJWT live
// in auth_handler_test.go so they can be shared across all handler test files.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi/v1"
	"cloud-backend/internal/entity"
	favoritesuc "cloud-backend/internal/usecase/favorites"
	sharinguc "cloud-backend/internal/usecase/sharing"
	storageuc "cloud-backend/internal/usecase/storage"
)

// ─── mockBlobService ─────────────────────────────────────────────────────────

type mockBlobService struct {
	usageResult    storageuc.StorageUsage
	usageErr       error
	presignGetOut  *storageuc.PresignGetResult
	presignGetErr  error
	deleteFileName string
	deleteErr      error
	blobs          []entity.Blob
	listErr        error
	renameErr      error
	searchResult   storageuc.SearchResult
	searchErr      error
}

func (m *mockBlobService) GetStorageUsage(_ context.Context, _ uuid.UUID) (storageuc.StorageUsage, error) {
	return m.usageResult, m.usageErr
}
func (m *mockBlobService) PresignGet(_ context.Context, _, _ uuid.UUID) (*storageuc.PresignGetResult, error) {
	return m.presignGetOut, m.presignGetErr
}
func (m *mockBlobService) DeleteBlob(_ context.Context, _, _ uuid.UUID) (string, error) {
	return m.deleteFileName, m.deleteErr
}
func (m *mockBlobService) ListBlobs(_ context.Context, _ uuid.UUID) ([]entity.Blob, error) {
	return m.blobs, m.listErr
}
func (m *mockBlobService) ListBlobsInFolder(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]entity.Blob, error) {
	return m.blobs, m.listErr
}
func (m *mockBlobService) RenameBlob(_ context.Context, _, _ uuid.UUID, _ string) error {
	return m.renameErr
}
func (m *mockBlobService) Search(_ context.Context, _ storageuc.SearchParams) (storageuc.SearchResult, error) {
	return m.searchResult, m.searchErr
}
func (m *mockBlobService) InitiateMultipartUpload(_ context.Context, _ storageuc.InitiateMultipartParams) (*storageuc.InitiateMultipartResult, error) {
	return &storageuc.InitiateMultipartResult{BlobID: uuid.New(), UploadID: "uid", PartURLs: []storageuc.PartURL{{PartNumber: 1, URL: "https://minio/part"}}}, nil
}
func (m *mockBlobService) CompleteMultipartUpload(_ context.Context, _ storageuc.CompleteMultipartParams) error {
	return nil
}

// ─── mockFolderService ───────────────────────────────────────────────────────

type mockFolderService struct {
	createFolder     entity.Folder
	createParentName string
	createErr        error
	getFolder        entity.Folder
	getErr           error
	listFolders      []entity.Folder
	listErr          error
	renameFolder     entity.Folder
	renameErr        error
	moveFolderInfo   storageuc.FolderMoveInfo
	moveFolderErr    error
	deleteFolderName string
	deleteFolderErr  error
	moveBlobInfo     storageuc.MoveBlobInfo
	moveBlobErr      error
}

func (m *mockFolderService) CreateFolder(_ context.Context, _ storageuc.CreateFolderParams) (entity.Folder, string, error) {
	return m.createFolder, m.createParentName, m.createErr
}
func (m *mockFolderService) GetFolder(_ context.Context, _, _ uuid.UUID) (entity.Folder, error) {
	return m.getFolder, m.getErr
}
func (m *mockFolderService) ListFolders(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]entity.Folder, error) {
	return m.listFolders, m.listErr
}
func (m *mockFolderService) RenameFolder(_ context.Context, _, _ uuid.UUID, _ string) (entity.Folder, error) {
	return m.renameFolder, m.renameErr
}
func (m *mockFolderService) MoveFolder(_ context.Context, _ storageuc.MoveFolderParams) (storageuc.FolderMoveInfo, error) {
	return m.moveFolderInfo, m.moveFolderErr
}
func (m *mockFolderService) DeleteFolder(_ context.Context, _, _ uuid.UUID) (string, error) {
	return m.deleteFolderName, m.deleteFolderErr
}
func (m *mockFolderService) MoveBlob(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID) (storageuc.MoveBlobInfo, error) {
	return m.moveBlobInfo, m.moveBlobErr
}

// ─── mockTrashService ────────────────────────────────────────────────────────

type mockTrashService struct {
	trashResult          storageuc.TrashResult
	trashListErr         error
	restoreBlobName      string
	restoreBlobErr       error
	hardDeleteBlobName   string
	hardDeleteBlobErr    error
	restoreFolderName    string
	restoreFolderErr     error
	hardDeleteFolderName string
	hardDeleteFolderErr  error
	emptyBlobs           []storageuc.EmptiedBlob
	emptyFolders         []storageuc.EmptiedFolder
	emptyErr             error
}

func (m *mockTrashService) ListTrash(_ context.Context, _ uuid.UUID) (storageuc.TrashResult, error) {
	return m.trashResult, m.trashListErr
}
func (m *mockTrashService) RestoreBlob(_ context.Context, _, _ uuid.UUID) (string, error) {
	return m.restoreBlobName, m.restoreBlobErr
}
func (m *mockTrashService) HardDeleteBlob(_ context.Context, _, _ uuid.UUID) (string, error) {
	return m.hardDeleteBlobName, m.hardDeleteBlobErr
}
func (m *mockTrashService) RestoreFolder(_ context.Context, _, _ uuid.UUID) (string, error) {
	return m.restoreFolderName, m.restoreFolderErr
}
func (m *mockTrashService) HardDeleteFolder(_ context.Context, _, _ uuid.UUID) (string, error) {
	return m.hardDeleteFolderName, m.hardDeleteFolderErr
}
func (m *mockTrashService) EmptyTrash(_ context.Context, _ uuid.UUID) ([]storageuc.EmptiedBlob, []storageuc.EmptiedFolder, error) {
	return m.emptyBlobs, m.emptyFolders, m.emptyErr
}

// ─── mockSharingService ──────────────────────────────────────────────────────

type mockSharingService struct {
	publicKey       []byte
	publicKeyErr    error
	createShare     entity.FileShareView
	createShareErr  error
	sharedWithMe    []entity.FileShareView
	sharedWithMeErr error
	myShares        []entity.FileShareView
	mySharesErr     error
	sharedFile      sharinguc.SharedFileResult
	sharedFileErr   error
	revokeErr       error
}

func (m *mockSharingService) GetRecipientPublicKey(_ context.Context, _ string) ([]byte, error) {
	return m.publicKey, m.publicKeyErr
}
func (m *mockSharingService) CreateShare(_ context.Context, _ sharinguc.CreateShareParams) (entity.FileShareView, error) {
	return m.createShare, m.createShareErr
}
func (m *mockSharingService) ListSharedWithMe(_ context.Context, _ uuid.UUID) ([]entity.FileShareView, error) {
	return m.sharedWithMe, m.sharedWithMeErr
}
func (m *mockSharingService) ListMyShares(_ context.Context, _, _ uuid.UUID) ([]entity.FileShareView, error) {
	return m.myShares, m.mySharesErr
}
func (m *mockSharingService) GetSharedFile(_ context.Context, _, _ uuid.UUID) (sharinguc.SharedFileResult, error) {
	return m.sharedFile, m.sharedFileErr
}
func (m *mockSharingService) RevokeShare(_ context.Context, _, _ uuid.UUID) error {
	return m.revokeErr
}

// ─── mockFavoritesService ────────────────────────────────────────────────────

type mockFavoritesService struct {
	addBlobErr      error
	removeBlobErr   error
	addFolderErr    error
	removeFolderErr error
	blobs           []favoritesuc.FavoriteBlob
	listBlobsErr    error
	folders         []entity.Folder
	listFoldersErr  error
}

func (m *mockFavoritesService) AddBlob(_ context.Context, _, _ uuid.UUID) error {
	return m.addBlobErr
}
func (m *mockFavoritesService) RemoveBlob(_ context.Context, _, _ uuid.UUID) error {
	return m.removeBlobErr
}
func (m *mockFavoritesService) AddFolder(_ context.Context, _, _ uuid.UUID) error {
	return m.addFolderErr
}
func (m *mockFavoritesService) RemoveFolder(_ context.Context, _, _ uuid.UUID) error {
	return m.removeFolderErr
}
func (m *mockFavoritesService) ListBlobs(_ context.Context, _ uuid.UUID) ([]favoritesuc.FavoriteBlob, error) {
	return m.blobs, m.listBlobsErr
}
func (m *mockFavoritesService) ListFolders(_ context.Context, _ uuid.UUID) ([]entity.Folder, error) {
	return m.folders, m.listFoldersErr
}

// ─── mockSessionsService ─────────────────────────────────────────────────────

type mockSessionsService struct {
	sessions       []entity.DeviceSession
	listErr        error
	revokeErr      error
	revokeOtherErr error
}

func (m *mockSessionsService) ListDeviceSessions(_ context.Context, _ uuid.UUID) ([]entity.DeviceSession, error) {
	return m.sessions, m.listErr
}
func (m *mockSessionsService) RevokeDeviceSession(_ context.Context, _, _ uuid.UUID) error {
	return m.revokeErr
}
func (m *mockSessionsService) RevokeOtherDeviceSessions(_ context.Context, _, _ uuid.UUID) error {
	return m.revokeOtherErr
}

// ─── fullRouter ───────────────────────────────────────────────────────────────
// Builds a fully-wired router with optional per-service overrides.
// Any nil field falls back to a no-op mock that returns zero values.

type routerOpts struct {
	auth           v1.AuthService
	blobs          v1.BlobService
	folders        v1.FolderService
	trash          v1.TrashService
	sharing        v1.SharingService
	favorites      v1.FavoritesService
	deviceSessions v1.SessionsService
	audit          v1.AuditService
	jwt            *mockParseJWT
}

func fullRouter(o routerOpts) http.Handler {
	if o.auth == nil {
		o.auth = &mockAuthService{}
	}
	if o.blobs == nil {
		o.blobs = &mockBlobService{}
	}
	if o.folders == nil {
		o.folders = &mockFolderService{}
	}
	if o.trash == nil {
		o.trash = &mockTrashService{}
	}
	if o.sharing == nil {
		o.sharing = &mockSharingService{}
	}
	if o.favorites == nil {
		o.favorites = &mockFavoritesService{}
	}
	if o.deviceSessions == nil {
		o.deviceSessions = &mockSessionsService{}
	}
	if o.audit == nil {
		o.audit = &mockAuditService{}
	}
	if o.jwt == nil {
		o.jwt = &mockParseJWT{userID: uuid.New(), sessionID: uuid.New()}
	}
	return v1.NewRouter(v1.Deps{
		Auth:                 o.auth,
		Blobs:                o.blobs,
		Folders:              o.folders,
		Trash:                o.trash,
		Sharing:              o.sharing,
		Favorites:            o.favorites,
		DeviceSessions:       o.deviceSessions,
		Tokens:               o.jwt,
		Blocklist:            &mockBlocklistHandler{},
		LoginRateLimiter:     &mockRateLimiterAllow{},
		PublicKeyRateLimiter: &mockRateLimiterAllow{},
		Audit:                o.audit,
		Logger:               zerolog.Nop(),
		RefreshCookie:        config.RefreshCookieConfig{Name: "refresh_token", Path: "/"},
	})
}
