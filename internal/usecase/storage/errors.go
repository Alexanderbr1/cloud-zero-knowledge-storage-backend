package storage

import "errors"

var (
	ErrNotFound       = errors.New("blob not found")
	ErrFolderNotFound = errors.New("folder not found")
	ErrFolderNotEmpty = errors.New("folder is not empty")
	ErrFolderCycle    = errors.New("moving folder would create a cycle")
	ErrFolderConflict = errors.New("folder with this name already exists")
	ErrFileTooLarge   = errors.New("file exceeds maximum upload size")
	ErrQuotaExceeded  = errors.New("storage quota exceeded")
)
