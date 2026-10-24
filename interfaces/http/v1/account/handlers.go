package account

import (
	app "github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/interfaces/http/authn"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

func (ctrl Controller) logger(ctx fiber.Ctx, component string) zerolog.Logger {
	return ctrl.Logger.With().Str("request_id", requestid.FromContext(ctx)).Str("component", "http.api.v1.account."+component).Logger()
}

// sensitive returns the device behind the request, after the rate limit of
// the routes that check authKey.
func (ctrl Controller) sensitive(ctx fiber.Ctx) (authn.Principal, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return principal, problem.New(problem.InvalidToken, "")
	}
	return principal, ctrl.Limits.AuthKey(ctx, principal.AccountID)
}

func (ctrl Controller) GetAccount(ctx fiber.Ctx, input dtos.Empty) (*AccountResponse, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	info, err := ctrl.AccountApplication.Get(ctx.Context(), principal.AccountID)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "get"), err)
	}
	return &AccountResponse{
		AccountID:       info.AccountID,
		Email:           info.Email,
		CreatedAt:       domain.Timestamp(info.CreatedAt),
		Usage:           Usage{Records: info.Usage.Records, Blobs: info.Usage.Blobs, Bytes: info.Usage.Bytes},
		MaxAccountBytes: info.MaxAccountBytes,
		Avatar:          avatarFromApp(info.Avatar),
	}, nil
}

func (ctrl Controller) ChangePassword(ctx fiber.Ctx, req ChangePasswordRequest) (*dtos.Empty, *fiberoapi.Problem) {
	principal, p := ctrl.sensitive(ctx)
	if p != nil {
		return nil, p
	}
	err := ctrl.AccountApplication.ChangePassword(ctx.Context(), principal.AccountID, principal.DeviceID, app.ChangePasswordInput{
		AuthKey:           req.AuthKey,
		Kdf:               req.Kdf.ToApp(),
		NewAuthKey:        req.NewAuthKey,
		WrappedAccountKey: req.WrappedAccountKey,
	})
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "password"), err)
	}
	return &dtos.Empty{}, nil
}

func (ctrl Controller) ChangeRecovery(ctx fiber.Ctx, req ChangeRecoveryRequest) (*dtos.Empty, *fiberoapi.Problem) {
	principal, p := ctrl.sensitive(ctx)
	if p != nil {
		return nil, p
	}
	if err := ctrl.AccountApplication.ChangeRecovery(ctx.Context(), principal.AccountID, req.AuthKey, req.Recovery.ToApp()); err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "recovery"), err)
	}
	return &dtos.Empty{}, nil
}

func (ctrl Controller) ChangeEmail(ctx fiber.Ctx, req ChangeEmailRequest) (*dtos.Empty, *fiberoapi.Problem) {
	principal, p := ctrl.sensitive(ctx)
	if p != nil {
		return nil, p
	}
	if err := ctrl.AccountApplication.ChangeEmail(ctx.Context(), principal.AccountID, req.AuthKey, req.Email); err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "email"), err)
	}
	return &dtos.Empty{}, nil
}

func (ctrl Controller) DeleteAccount(ctx fiber.Ctx, req DeleteAccountRequest) (*dtos.Empty, *fiberoapi.Problem) {
	principal, p := ctrl.sensitive(ctx)
	if p != nil {
		return nil, p
	}
	if err := ctrl.AccountApplication.Delete(ctx.Context(), principal.AccountID, req.AuthKey); err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "delete"), err)
	}
	return &dtos.Empty{}, nil
}

func (ctrl Controller) PutAvatar(ctx fiber.Ctx, req PutAvatarRequest) (*PutAvatarResponse, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	avatar, created, err := ctrl.AccountApplication.PutAvatar(ctx.Context(), principal.AccountID, req.Body)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "avatar.put"), err)
	}
	return &PutAvatarResponse{Size: avatar.Size, UpdatedAt: domain.Timestamp(avatar.UpdatedAt), created: created}, nil
}

func (ctrl Controller) GetAvatar(ctx fiber.Ctx, input dtos.Empty) (*avatarStream, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	file, size, err := ctrl.AccountApplication.GetAvatar(ctx.Context(), principal.AccountID)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "avatar.get"), err)
	}
	return &avatarStream{file: file, size: size}, nil
}

func (ctrl Controller) DeleteAvatar(ctx fiber.Ctx, input dtos.Empty) (*dtos.Empty, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	if err := ctrl.AccountApplication.DeleteAvatar(ctx.Context(), principal.AccountID); err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "avatar.delete"), err)
	}
	return &dtos.Empty{}, nil
}
