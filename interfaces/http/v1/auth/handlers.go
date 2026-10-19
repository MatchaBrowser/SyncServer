package auth

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	authDto "github.com/matchabrowser/syncserver/application/auth/dto"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/apperrors"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/auth/dtos"
)

func (ctrl Controller) Login(ctx fiber.Ctx, req dtos.LoginRequest) (*dtos.LoginResponse, *fiberoapi.ErrorResponse) {
	requestId := requestid.FromContext(ctx)
	logger := ctrl.Logger.With().Str("request_id", requestId).Str("component", "http.api.v1.auth.login").Logger()

	resp, err := ctrl.AuthApplication.Authenticate(authDto.AuthenticateInput{
		Email:    req.Email,
		Password: req.Password,
		Context:  ctx,
	})
	if err != nil {
		logger.Error().Err(err).Str("email", req.Email).Msg("failed to authenticate user")
		if errors.Is(err, apperrors.ErrInvalidCredentials) || errors.Is(err, apperrors.ErrUserNotActive) {
			return nil, &fiberoapi.ErrorResponse{
				Code:    fiber.StatusUnauthorized,
				Details: err.Error(),
				Type:    "AUTHENTICATION_FAILED",
			}
		}
		return nil, &fiberoapi.ErrorResponse{
			Code:    fiber.StatusInternalServerError,
			Details: "Authentication failed",
			Type:    "INTERNAL_SERVER_ERROR",
		}
	}
	return &dtos.LoginResponse{Token: resp.Token}, nil
}

func (ctrl Controller) Logout(ctx fiber.Ctx, input struct{}) (*dtos.LogoutResponse, *fiberoapi.ErrorResponse) {
	requestId := requestid.FromContext(ctx)
	logger := ctrl.Logger.With().Str("request_id", requestId).Str("component", "http.api.v1.auth.logout").Logger()

	authCtx, err := fiberoapi.GetAuthContext(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get auth context")
		return nil, &fiberoapi.ErrorResponse{
			Code:    fiber.StatusUnauthorized,
			Details: "Authentication required",
			Type:    "AUTHENTICATION_REQUIRED",
		}
	}

	sessionId, _ := authCtx.Claims["session_id"].(string)
	err = ctrl.AuthApplication.Logout(authDto.LogoutInput{SessionId: sessionId})
	if err != nil {
		logger.Error().Err(err).Str("session_id", sessionId).Msg("failed to logout user")
		return nil, &fiberoapi.ErrorResponse{
			Code:    fiber.StatusInternalServerError,
			Details: err.Error(),
			Type:    "LOGOUT_FAILED",
		}
	}

	return &dtos.LogoutResponse{
		Message: "Logged out successfully",
	}, nil
}

func (ctrl Controller) Register(ctx fiber.Ctx, req dtos.RegisterRequest) (*dtos.RegisterResponse, *fiberoapi.ErrorResponse) {
	requestId := requestid.FromContext(ctx)
	logger := ctrl.Logger.With().Str("request_id", requestId).Str("component", "http.api.v1.auth.register").Logger()

	err := ctrl.AuthApplication.Register(authDto.RegisterInput{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		logger.Error().Err(err).Str("email", req.Email).Msg("failed to register user")
		return nil, &fiberoapi.ErrorResponse{
			Code:    fiber.StatusBadRequest,
			Details: err.Error(),
			Type:    "REGISTRATION_FAILED",
		}
	}

	return &dtos.RegisterResponse{
		Message: "User registered successfully",
	}, nil
}
