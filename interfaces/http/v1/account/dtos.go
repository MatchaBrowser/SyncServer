package account

import (
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"
)

type Usage struct {
	Records int64 `json:"records"`
	Blobs   int64 `json:"blobs"`
	Bytes   int64 `json:"bytes"`
}

type AccountResponse struct {
	AccountID       string           `json:"accountId"`
	Email           string           `json:"email"`
	CreatedAt       domain.Timestamp `json:"createdAt"`
	Usage           Usage            `json:"usage"`
	MaxAccountBytes *int             `json:"maxAccountBytes"`
}

type ChangePasswordRequest struct {
	AuthKey           []byte   `json:"authKey" validate:"required,len=32"`
	Kdf               dtos.Kdf `json:"kdf"`
	NewAuthKey        []byte   `json:"newAuthKey" validate:"required,len=32"`
	WrappedAccountKey []byte   `json:"wrappedAccountKey" validate:"required,len=60"`
}

type ChangeRecoveryRequest struct {
	AuthKey  []byte        `json:"authKey" validate:"required,len=32"`
	Recovery dtos.Recovery `json:"recovery"`
}

type ChangeEmailRequest struct {
	AuthKey []byte `json:"authKey" validate:"required,len=32"`
	Email   string `json:"email" validate:"required,email,max=254"`
}

type DeleteAccountRequest struct {
	AuthKey []byte `json:"authKey" validate:"required,len=32"`
}
