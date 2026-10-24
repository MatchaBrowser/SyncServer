package domain

import (
	"context"
	"time"
)

// Admin is an administrator of the console. Created by the CLI, with a
// password drawn by the server: only its Argon2id hash is kept.
type Admin struct {
	ID           string
	Name         string
	PasswordHash string
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

func (Admin) TableName() string {
	return "admins"
}

// AdminSession is a session of the console. Only the hash of its token is
// kept; the token lives in a cookie.
type AdminSession struct {
	Hash      []byte `gorm:"primaryKey"`
	AdminID   string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (AdminSession) TableName() string {
	return "admin_sessions"
}

type AdminPers interface {
	// Create inserts an administrator. ErrDuplicate: the name is taken.
	Create(ctx context.Context, admin *Admin) error
	// GetByName and GetByID: ErrNotFound when unknown.
	GetByName(ctx context.Context, name string) (*Admin, error)
	GetByID(ctx context.Context, id string) (*Admin, error)
	// List returns the administrators by name.
	List(ctx context.Context) ([]Admin, error)
	// Update writes the given columns of the administrator.
	Update(ctx context.Context, admin *Admin, columns ...string) error
	// Delete removes an administrator and its sessions. ErrNotFound: unknown.
	Delete(ctx context.Context, id string) error

	CreateSession(ctx context.Context, session *AdminSession) error
	// GetSession returns a session. ErrNotFound: unknown.
	GetSession(ctx context.Context, hash []byte) (*AdminSession, error)
	DeleteSession(ctx context.Context, hash []byte) error
	// DeleteSessions closes every session of an administrator.
	DeleteSessions(ctx context.Context, adminID string) error
	// PurgeSessions removes the sessions expired before a date.
	PurgeSessions(ctx context.Context, before time.Time) (int64, error)
}
