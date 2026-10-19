package auth

import "github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

type PreloginRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

type PreloginResponse struct {
	Kdf dtos.Kdf `json:"kdf"`
}

type CreateAccountRequest struct {
	Email             string        `json:"email" validate:"required,email,max=254"`
	Invitation        string        `json:"invitation,omitempty" validate:"max=64"`
	Kdf               dtos.Kdf      `json:"kdf"`
	AuthKey           []byte        `json:"authKey" validate:"required,len=32"`
	WrappedAccountKey []byte        `json:"wrappedAccountKey" validate:"required,len=60"`
	Recovery          dtos.Recovery `json:"recovery"`
	Device            dtos.Device   `json:"device"`
}

type LoginRequest struct {
	Email   string      `json:"email" validate:"required,email,max=254"`
	AuthKey []byte      `json:"authKey" validate:"required,len=32"`
	Device  dtos.Device `json:"device"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required,max=128"`
}

type RefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
}

type RecoverStartRequest struct {
	Email           string `json:"email" validate:"required,email,max=254"`
	RecoveryAuthKey []byte `json:"recoveryAuthKey" validate:"required,len=32"`
}

type RecoverStartResponse struct {
	RecoveryToken             string `json:"recoveryToken"`
	ExpiresIn                 int    `json:"expiresIn"`
	RecoveryWrappedAccountKey []byte `json:"recoveryWrappedAccountKey"`
}

type RecoverFinishRequest struct {
	RecoveryToken     string         `json:"recoveryToken" validate:"required,max=128"`
	Kdf               dtos.Kdf       `json:"kdf"`
	AuthKey           []byte         `json:"authKey" validate:"required,len=32"`
	WrappedAccountKey []byte         `json:"wrappedAccountKey" validate:"required,len=60"`
	Recovery          *dtos.Recovery `json:"recovery,omitempty"`
	Device            dtos.Device    `json:"device"`
}
