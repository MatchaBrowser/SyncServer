package account

import (
	"context"

	"github.com/matchabrowser/syncserver/domain"
)

// ListDevices returns the active devices of the account.
func (c *AccountApplication) ListDevices(ctx context.Context, accountID string) ([]domain.Device, error) {
	return c.Devices.ListActive(ctx, accountID)
}

// activeDevice returns an active device of the account, or ErrNotFound.
func (c *AccountApplication) activeDevice(ctx context.Context, accountID, deviceID string) (*domain.Device, error) {
	device, err := c.Devices.Get(ctx, accountID, deviceID)
	if err != nil {
		return nil, err
	}
	if device.RevokedAt != nil {
		return nil, domain.ErrNotFound
	}
	return device, nil
}

// RenameDevice changes the name of an active device.
func (c *AccountApplication) RenameDevice(ctx context.Context, accountID, deviceID, name string) (*domain.Device, error) {
	var device *domain.Device
	err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		var err error
		if device, err = c.activeDevice(ctx, accountID, deviceID); err != nil {
			return err
		}
		device.Name = name
		return c.Devices.Save(ctx, device)
	})
	return device, err
}

// RevokeDevice disconnects a device: its tokens are revoked at once, and it
// gets device_revoked. It keeps its local data (SYNC.md § 4.5).
func (c *AccountApplication) RevokeDevice(ctx context.Context, accountID, deviceID string) error {
	err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		if _, err := c.activeDevice(ctx, accountID, deviceID); err != nil {
			return err
		}
		now := c.Now()
		if err := c.Devices.Revoke(ctx, accountID, []string{deviceID}, "", now); err != nil {
			return err
		}
		return c.Tokens.Revoke(ctx, accountID, []string{deviceID}, "", now)
	})
	if err == nil {
		c.closeDevice(accountID, deviceID)
	}
	return err
}
