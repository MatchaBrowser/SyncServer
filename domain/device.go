package domain

import (
	"context"
	"time"
)

// Device is a Mac connected to an account. It is identified by the pair
// (account, id): a Mac keeps its id across accounts (SYNC.md § 4.3).
type Device struct {
	AccountID  string `gorm:"primaryKey"`
	ID         string `gorm:"primaryKey"`
	Name       string
	CreatedAt  time.Time
	LastSeenAt time.Time
	// RevokedAt is set when the device is disconnected by another one, or by
	// a change of password. A new login reactivates it.
	RevokedAt *time.Time
}

func (Device) TableName() string {
	return "devices"
}

type DevicePers interface {
	// Get returns a device, revoked or not. ErrNotFound: unknown.
	Get(ctx context.Context, accountID, id string) (*Device, error)
	// Save inserts or replaces a device.
	Save(ctx context.Context, device *Device) error
	// ListActive returns the devices that are not revoked.
	ListActive(ctx context.Context, accountID string) ([]Device, error)
	// ListAll returns every device of the accounts, revoked or not.
	ListAll(ctx context.Context, accountIDs ...string) ([]Device, error)
	// CountActive counts the devices that are not revoked, all accounts
	// together.
	CountActive(ctx context.Context) (int64, error)
	// Revoke marks the devices as revoked; all of them except keepID when
	// ids is empty.
	Revoke(ctx context.Context, accountID string, ids []string, keepID string, at time.Time) error
}
