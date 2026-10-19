// Package dtos holds the JSON shapes shared by several routes of the API
// (SYNC.md § 4). Binary values are []byte: encoding/json reads and writes them
// in standard base 64 with padding.
package dtos

import (
	"strings"

	"github.com/matchabrowser/syncserver/application/account"
)

// Kdf are the parameters of the derivation of the master password. Their
// bounds are checked by the application (weak_kdf), not by the validator
// (invalid_request).
type Kdf struct {
	Algorithm  string `json:"algorithm" validate:"required"`
	Iterations int    `json:"iterations" validate:"required"`
	Salt       []byte `json:"salt" validate:"required"`
}

func (k Kdf) ToApp() account.Kdf {
	return account.Kdf{Algorithm: k.Algorithm, Iterations: k.Iterations, Salt: k.Salt}
}

func KdfFromApp(k *account.Kdf) Kdf {
	return Kdf{Algorithm: k.Algorithm, Iterations: k.Iterations, Salt: k.Salt}
}

// Device names the device that opens a session.
type Device struct {
	ID   string `json:"id" validate:"required,uuid"`
	Name string `json:"name" validate:"required,max=100"`
}

// ToApp keeps the identifier in lowercase: UUIDs compare without case
// (SYNC.md § 4.1).
func (d Device) ToApp() account.DeviceInput {
	return account.DeviceInput{ID: strings.ToLower(d.ID), Name: strings.TrimSpace(d.Name)}
}

// Recovery is the proof and the envelope of a recovery key.
type Recovery struct {
	AuthKey           []byte `json:"authKey" validate:"required,len=32"`
	WrappedAccountKey []byte `json:"wrappedAccountKey" validate:"required,len=60"`
}

func (r Recovery) ToApp() account.RecoveryInput {
	return account.RecoveryInput{AuthKey: r.AuthKey, WrappedAccountKey: r.WrappedAccountKey}
}

// Session is what a login gives to a device (SYNC.md § 4.3).
type Session struct {
	AccountID         string `json:"accountId"`
	DeviceID          string `json:"deviceId"`
	AccessToken       string `json:"accessToken"`
	RefreshToken      string `json:"refreshToken"`
	ExpiresIn         int    `json:"expiresIn"`
	WrappedAccountKey []byte `json:"wrappedAccountKey"`
	KeyVersion        int    `json:"keyVersion"`
}

func SessionFromApp(s *account.Session) *Session {
	return &Session{
		AccountID:         s.AccountID,
		DeviceID:          s.DeviceID,
		AccessToken:       s.AccessToken,
		RefreshToken:      s.RefreshToken,
		ExpiresIn:         s.ExpiresIn,
		WrappedAccountKey: s.WrappedAccountKey,
		KeyVersion:        s.KeyVersion,
	}
}

// Empty is the input of a route without parameters, and the output of a 204.
type Empty struct{}
