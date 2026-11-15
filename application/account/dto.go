package account

import (
	"time"

	"github.com/matchabrowser/syncserver/domain"
)

// Kdf are the parameters of the derivation of the master password.
type Kdf struct {
	Algorithm  string
	Iterations int
	Salt       []byte
}

// DeviceInput names the device that opens a session.
type DeviceInput struct {
	ID   string
	Name string
}

// RecoveryInput is the proof and the envelopes of a recovery key.
type RecoveryInput struct {
	AuthKey           []byte
	WrappedAccountKey []byte
	// WrappedVaultRootKey is nil for an account without a vault.
	WrappedVaultRootKey []byte
}

// Session is what a login gives to a device (SYNC.md § 4.3).
type Session struct {
	AccountID         string
	DeviceID          string
	AccessToken       string
	RefreshToken      string
	ExpiresIn         int
	WrappedAccountKey []byte
	// WrappedVaultRootKey is nil for an account without a vault.
	WrappedVaultRootKey []byte
	KeyVersion          int
}

type RegisterInput struct {
	Email               string
	Invitation          string
	Kdf                 Kdf
	AuthKey             []byte
	WrappedAccountKey   []byte
	WrappedVaultRootKey []byte
	Recovery            RecoveryInput
	Device              DeviceInput
}

type LoginInput struct {
	Email   string
	AuthKey []byte
	Device  DeviceInput
}

type RefreshOutput struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type RecoverStartOutput struct {
	RecoveryToken             string
	ExpiresIn                 int
	RecoveryWrappedAccountKey []byte
	// RecoveryWrappedVaultRootKey is nil for an account without a vault.
	RecoveryWrappedVaultRootKey []byte
}

type RecoverFinishInput struct {
	RecoveryToken       string
	Kdf                 Kdf
	AuthKey             []byte
	WrappedAccountKey   []byte
	WrappedVaultRootKey []byte
	// Recovery, when set, replaces the recovery key.
	Recovery *RecoveryInput
	Device   DeviceInput
}

// Principal is the device behind a valid access token.
type Principal struct {
	AccountID string
	DeviceID  string
	TokenHash []byte
	ExpiresAt time.Time
}

type AccountInfo struct {
	AccountID string
	Email     string
	CreatedAt time.Time
	Usage     domain.Usage
	// MaxAccountBytes is nil when the account has no quota.
	MaxAccountBytes *int
	// Avatar is nil when the account has none.
	Avatar *Avatar
}

// Avatar describes the encrypted avatar of the account.
type Avatar struct {
	Size      int64
	UpdatedAt time.Time
}

type ChangePasswordInput struct {
	AuthKey             []byte
	Kdf                 Kdf
	NewAuthKey          []byte
	WrappedAccountKey   []byte
	WrappedVaultRootKey []byte
}

// checkVaultEnvelopes refuses a change of the envelopes of the account key
// that does not change those of the vault root key with them: an envelope
// is required for an account that has a vault, and refused for one that has
// none (VAULT.md § 2.2).
func checkVaultEnvelopes(account *domain.Account, envelopes ...[]byte) error {
	for _, envelope := range envelopes {
		if account.HasVault() != (len(envelope) > 0) {
			return domain.ErrVaultEnvelope
		}
	}
	return nil
}
