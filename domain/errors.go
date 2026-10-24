package domain

import "errors"

// Errors of the persistence layer.
var (
	// ErrNotFound is returned when a row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrDuplicate is returned when a row conflicts with an existing one
	// (same email, same identifier...).
	ErrDuplicate = errors.New("duplicate")
)

// Errors of the API (SYNC.md § 4.8).
var (
	ErrRegistrationClosed = errors.New("registration closed")
	ErrInvitationRequired = errors.New("invitation required")
	ErrEmailTaken         = errors.New("email taken")
	ErrWeakKdf            = errors.New("kdf parameters out of bounds")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("access token expired")
	ErrDeviceRevoked      = errors.New("device revoked")
	ErrAccountDeleted     = errors.New("account deleted")
	ErrInvalidCursor      = errors.New("invalid cursor")
	ErrCursorExpired      = errors.New("cursor expired")
	ErrTooManyChanges     = errors.New("too many changes in one push")
	// ErrIdempotencyMismatch: the same Idempotency-Key with another body.
	ErrIdempotencyMismatch = errors.New("idempotency key reused with another request")
	ErrBlobExists          = errors.New("another package has this identifier")
	ErrBlobTooLarge        = errors.New("package too large")
	ErrQuotaExceeded       = errors.New("account storage full")
	ErrAvatarTooLarge      = errors.New("avatar too large")
)
