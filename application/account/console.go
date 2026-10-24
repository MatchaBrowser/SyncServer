package account

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/domain"
)

// AccountSummary is an account as the admin console lists it: never its
// hashes nor its envelopes.
type AccountSummary struct {
	ID         string
	Email      string
	CreatedAt  time.Time
	UsageBytes int64
	// ActiveDevices counts the devices that are not revoked.
	ActiveDevices int
	// LastWriteAt is the time of the latest write: a version is a time in
	// microseconds (SYNC.md § 4.9). Nil when the account never wrote.
	LastWriteAt *time.Time
	// LastSeenAt is the latest login or renewal of the tokens of its
	// devices: at least every quarter of an hour while a device syncs.
	LastSeenAt *time.Time
}

// AccountPage is a page of the account list.
type AccountPage struct {
	Accounts []AccountSummary
	// Total counts the accounts of the search, on every page.
	Total int64
}

// AccountDetails describes an account for the console.
type AccountDetails struct {
	AccountSummary
	KeyVersion int
	Usage      domain.Usage
	// Avatar is nil when the account has none.
	Avatar *Avatar
	// Devices are all the devices of the account, revoked ones included.
	Devices []domain.Device
}

// Overview counts what the dashboard of the console shows.
type Overview struct {
	Accounts      int64
	ActiveDevices int64
}

// ListAccounts returns a page of accounts, newest first; search keeps the
// emails that contain it.
func (c *AccountApplication) ListAccounts(ctx context.Context, search string, offset, limit int) (*AccountPage, error) {
	accounts, total, err := c.Accounts.List(ctx, domain.AccountQuery{Search: search, Offset: offset, Limit: limit})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	devices, err := c.Devices.ListAll(ctx, ids...)
	if err != nil {
		return nil, err
	}
	byAccount := make(map[string][]domain.Device, len(accounts))
	for _, d := range devices {
		byAccount[d.AccountID] = append(byAccount[d.AccountID], d)
	}
	page := &AccountPage{Accounts: make([]AccountSummary, len(accounts)), Total: total}
	for i, a := range accounts {
		page.Accounts[i] = summarize(&a, byAccount[a.ID])
	}
	return page, nil
}

// Describe returns an account with all its devices. ErrNotFound: unknown.
func (c *AccountApplication) Describe(ctx context.Context, accountID string) (*AccountDetails, error) {
	account, err := c.Accounts.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	usage, err := c.Accounts.Usage(ctx, account)
	if err != nil {
		return nil, err
	}
	devices, err := c.Devices.ListAll(ctx, accountID)
	if err != nil {
		return nil, err
	}
	details := &AccountDetails{
		AccountSummary: summarize(account, devices),
		KeyVersion:     account.KeyVersion,
		Usage:          usage,
		Devices:        devices,
	}
	if account.AvatarID != nil {
		details.Avatar = &Avatar{Size: account.AvatarSize, UpdatedAt: *account.AvatarUpdatedAt}
	}
	return details, nil
}

// Overview counts the accounts and the active devices.
func (c *AccountApplication) Overview(ctx context.Context) (Overview, error) {
	var overview Overview
	_, total, err := c.Accounts.List(ctx, domain.AccountQuery{Limit: 1})
	if err != nil {
		return overview, err
	}
	overview.Accounts = total
	overview.ActiveDevices, err = c.Devices.CountActive(ctx)
	return overview, err
}

// Labels name the accounts and devices of audit events, for display only:
// the journal itself never holds an email.
type Labels struct {
	// Emails by account id; a deleted account is missing.
	Emails map[string]string
	// Devices are the names of the devices, by DeviceKey.
	Devices map[string]string
}

// DeviceKey is the key of a device in Labels.Devices.
func DeviceKey(accountID, deviceID string) string {
	return accountID + "|" + deviceID
}

// Labels returns the emails and device names of the accounts that still
// exist among ids.
func (c *AccountApplication) Labels(ctx context.Context, accountIDs []string) (Labels, error) {
	labels := Labels{Emails: map[string]string{}, Devices: map[string]string{}}
	accounts, err := c.Accounts.ListByIDs(ctx, accountIDs)
	if err != nil {
		return labels, err
	}
	for _, a := range accounts {
		labels.Emails[a.ID] = a.Email
	}
	devices, err := c.Devices.ListAll(ctx, accountIDs...)
	if err != nil {
		return labels, err
	}
	for _, d := range devices {
		labels.Devices[DeviceKey(d.AccountID, d.ID)] = d.Name
	}
	return labels, nil
}

func summarize(account *domain.Account, devices []domain.Device) AccountSummary {
	summary := AccountSummary{
		ID:         account.ID,
		Email:      account.Email,
		CreatedAt:  account.CreatedAt,
		UsageBytes: account.UsageBytes,
	}
	if account.LastVersion > 0 {
		lastWrite := time.UnixMicro(account.LastVersion).UTC()
		summary.LastWriteAt = &lastWrite
	}
	for _, d := range devices {
		if d.RevokedAt == nil {
			summary.ActiveDevices++
		}
		if summary.LastSeenAt == nil || d.LastSeenAt.After(*summary.LastSeenAt) {
			lastSeen := d.LastSeenAt
			summary.LastSeenAt = &lastSeen
		}
	}
	return summary
}
