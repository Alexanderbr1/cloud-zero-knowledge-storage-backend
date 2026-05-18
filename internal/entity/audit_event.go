package entity

import (
	"time"

	"github.com/google/uuid"
)

type AuditEvent struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	EventType    string
	IPAddress    string
	UserAgent    string
	ResourceID   *uuid.UUID
	ResourceName string
	CreatedAt    time.Time
}

const (
	AuditLoginSuccess    = "login_success"
	AuditLoginFailed     = "login_failed"
	AuditLogout          = "logout"
	AuditSessionRevoked  = "session_revoked"
	AuditSessionsRevoked = "sessions_revoked_other"

	AuditFileUploaded    = "file_uploaded"
	AuditFileDownloaded  = "file_downloaded"
	AuditFileDeleted     = "file_deleted"
	AuditFileRestored    = "file_restored"
	AuditFileHardDeleted = "file_hard_deleted"
	AuditFileRenamed     = "file_renamed"
	AuditFileMoved       = "file_moved"
	AuditFolderMoved     = "folder_moved"
	AuditFileShared      = "file_shared"
	AuditFileShareRevoked = "file_share_revoked"

	AuditFolderCreated     = "folder_created"
	AuditFolderDeleted     = "folder_deleted"
	AuditFolderHardDeleted = "folder_hard_deleted"
	AuditFolderRestored    = "folder_restored"
	AuditFolderRenamed     = "folder_renamed"
)
