package auth

import (
	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/interfaces/http/authn"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

func (ctrl Controller) logger(ctx fiber.Ctx, component string) zerolog.Logger {
	return ctrl.Logger.With().Str("request_id", requestid.FromContext(ctx)).Str("component", "http.api.v1.auth."+component).Logger()
}

func (ctrl Controller) Prelogin(ctx fiber.Ctx, req PreloginRequest) (*PreloginResponse, *fiberoapi.Problem) {
	if p := ctrl.Limits.Prelogin(ctx); p != nil {
		return nil, p
	}
	kdf, err := ctrl.AccountApplication.Prelogin(ctx.Context(), req.Email)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "prelogin"), err)
	}
	return &PreloginResponse{Kdf: dtos.KdfFromApp(kdf)}, nil
}

func (ctrl Controller) CreateAccount(ctx fiber.Ctx, req CreateAccountRequest) (*dtos.Session, *fiberoapi.Problem) {
	if p := ctrl.Limits.Register(ctx); p != nil {
		return nil, p
	}
	session, err := ctrl.AccountApplication.Register(ctx.Context(), account.RegisterInput{
		Email:             req.Email,
		Invitation:        req.Invitation,
		Kdf:               req.Kdf.ToApp(),
		AuthKey:           req.AuthKey,
		WrappedAccountKey: req.WrappedAccountKey,
		Recovery:          req.Recovery.ToApp(),
		Device:            req.Device.ToApp(),
	})
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "create_account"), err)
	}
	return dtos.SessionFromApp(session), nil
}

func (ctrl Controller) Login(ctx fiber.Ctx, req LoginRequest) (*dtos.Session, *fiberoapi.Problem) {
	if p := ctrl.Limits.AuthKey(ctx, account.NormalizeEmail(req.Email)); p != nil {
		return nil, p
	}
	session, err := ctrl.AccountApplication.Login(ctx.Context(), account.LoginInput{
		Email:   req.Email,
		AuthKey: req.AuthKey,
		Device:  req.Device.ToApp(),
	})
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "login"), err)
	}
	return dtos.SessionFromApp(session), nil
}

func (ctrl Controller) Refresh(ctx fiber.Ctx, req RefreshRequest) (*RefreshResponse, *fiberoapi.Problem) {
	output, err := ctrl.AccountApplication.Refresh(ctx.Context(), req.RefreshToken)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "refresh"), err)
	}
	return &RefreshResponse{AccessToken: output.AccessToken, RefreshToken: output.RefreshToken, ExpiresIn: output.ExpiresIn}, nil
}

func (ctrl Controller) Logout(ctx fiber.Ctx, input dtos.Empty) (*dtos.Empty, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	if err := ctrl.AccountApplication.Logout(ctx.Context(), principal.AccountID, principal.DeviceID); err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "logout"), err)
	}
	return &dtos.Empty{}, nil
}

func (ctrl Controller) RecoverStart(ctx fiber.Ctx, req RecoverStartRequest) (*RecoverStartResponse, *fiberoapi.Problem) {
	if p := ctrl.Limits.AuthKey(ctx, account.NormalizeEmail(req.Email)); p != nil {
		return nil, p
	}
	output, err := ctrl.AccountApplication.RecoverStart(ctx.Context(), req.Email, req.RecoveryAuthKey)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "recover_start"), err)
	}
	return &RecoverStartResponse{
		RecoveryToken:             output.RecoveryToken,
		ExpiresIn:                 output.ExpiresIn,
		RecoveryWrappedAccountKey: output.RecoveryWrappedAccountKey,
	}, nil
}

func (ctrl Controller) RecoverFinish(ctx fiber.Ctx, req RecoverFinishRequest) (*dtos.Session, *fiberoapi.Problem) {
	input := account.RecoverFinishInput{
		RecoveryToken:     req.RecoveryToken,
		Kdf:               req.Kdf.ToApp(),
		AuthKey:           req.AuthKey,
		WrappedAccountKey: req.WrappedAccountKey,
		Device:            req.Device.ToApp(),
	}
	if req.Recovery != nil {
		recovery := req.Recovery.ToApp()
		input.Recovery = &recovery
	}
	session, err := ctrl.AccountApplication.RecoverFinish(ctx.Context(), input)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "recover_finish"), err)
	}
	return dtos.SessionFromApp(session), nil
}
