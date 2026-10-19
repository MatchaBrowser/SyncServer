package apperrors

import "errors"

// Sentinel errors for cross-layer error handling.
// This package has no dependencies on domain, infrastructure, or interfaces,
// so it can be safely imported from any layer.
var (
	// Access & authentication
	ErrForbidden          = errors.New("forbidden")
	ErrAccessDenied       = errors.New("access denied")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("invalid current password")
	ErrUserNotActive      = errors.New("user is not active")

	// Not found
	ErrNotFound         = errors.New("not found")
	ErrUserNotFound     = errors.New("user not found")
	ErrSpaceNotFound    = errors.New("space not found")
	ErrDocumentNotFound = errors.New("document not found")
	ErrDatabaseNotFound = errors.New("database not found")
	ErrDrawingNotFound  = errors.New("drawing not found")
	ErrRowNotFound      = errors.New("row not found")
	ErrWebhookNotFound  = errors.New("webhook not found")
	ErrActionNotFound   = errors.New("action not found")
	ErrVersionNotFound  = errors.New("version not found")
	ErrFavoriteNotFound = errors.New("favorite not found")

	// Conflict / validation
	ErrConflict           = errors.New("conflict")
	ErrConflictChildren   = errors.New("conflict: space has active documents")
	ErrDuplicate          = errors.New("already exists")
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidMove        = errors.New("invalid move")
	ErrDocumentNotDeleted = errors.New("document is not deleted")

	// Session / token
	ErrInvalidToken   = errors.New("invalid token")
	ErrSessionExpired = errors.New("session has expired")
)
