package account

import (
	"io"

	app "github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

	"github.com/gofiber/fiber/v3"
)

type Usage struct {
	Records int64 `json:"records"`
	Blobs   int64 `json:"blobs"`
	Bytes   int64 `json:"bytes"`
}

// Avatar describes the encrypted avatar of the account.
type Avatar struct {
	Size      int64            `json:"size"`
	UpdatedAt domain.Timestamp `json:"updatedAt"`
}

func avatarFromApp(a *app.Avatar) *Avatar {
	if a == nil {
		return nil
	}
	return &Avatar{Size: a.Size, UpdatedAt: domain.Timestamp(a.UpdatedAt)}
}

type AccountResponse struct {
	AccountID       string           `json:"accountId"`
	Email           string           `json:"email"`
	CreatedAt       domain.Timestamp `json:"createdAt"`
	Usage           Usage            `json:"usage"`
	MaxAccountBytes *int             `json:"maxAccountBytes"`
	// Avatar is null when the account has none.
	Avatar *Avatar `json:"avatar"`
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

type PutAvatarRequest struct {
	Body []byte `body:"raw" validate:"required"`
}

// PutAvatarResponse is 201 for a first avatar, 200 for a replaced one.
type PutAvatarResponse struct {
	Size      int64            `json:"size"`
	UpdatedAt domain.Timestamp `json:"updatedAt"`
	created   bool
}

func (r PutAvatarResponse) HTTPStatus() int {
	if r.created {
		return fiber.StatusCreated
	}
	return fiber.StatusOK
}

// avatarStream sends the avatar from its file, without loading it in memory.
type avatarStream struct {
	file io.ReadCloser
	size int64
}

func (s *avatarStream) Respond(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, fiber.MIMEOctetStream)
	// fasthttp closes the file once it is sent.
	return c.SendStream(s.file, int(s.size))
}
