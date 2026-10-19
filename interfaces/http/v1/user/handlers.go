package user

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	userDto "github.com/matchabrowser/syncserver/application/user/dto"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/apperrors"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/mapper"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/user/dtos"
)

func (ctrl *Controller) GetProfile(ctx fiber.Ctx, input struct{}) (*dtos.ProfileResponse, *fiberoapi.ErrorResponse) {
	requestId := requestid.FromContext(ctx)
	logger := ctrl.Logger.With().Str("request_id", requestId).Str("component", "http.api.v1.user.get_profile").Logger()

	// Get the authenticated user context
	authCtx, err := fiberoapi.GetAuthContext(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get auth context")
		return nil, &fiberoapi.ErrorResponse{
			Code:    fiber.StatusUnauthorized,
			Details: "Authentication required",
			Type:    "AUTHENTICATION_REQUIRED",
		}
	}

	result, err := ctrl.UserApplication.GetByUserId(userDto.GetByUserIdInput{UserId: authCtx.UserID})
	if err != nil {
		logger.Error().Err(err).Msg("failed to get user by id")
		return nil, &fiberoapi.ErrorResponse{
			Code:    fiber.StatusInternalServerError,
			Details: "Failed to retrieve user",
			Type:    "INTERNAL_SERVER_ERROR",
		}
	}

	profile := dtos.ProfileResponse{}
	err = mapper.MapStructByFieldNames(result.User, &profile)
	if err != nil {
		logger.Error().Err(err).Msg("failed to map user to profile")
		return nil, &fiberoapi.ErrorResponse{
			Code:    fiber.StatusInternalServerError,
			Details: "Failed to retrieve profile",
			Type:    "INTERNAL_SERVER_ERROR",
		}
	}

	return &profile, nil
}

func (ctrl *Controller) UpdateProfile(ctx fiber.Ctx, req dtos.UpdateProfileRequest) (*dtos.UpdateProfileResponse, *fiberoapi.ErrorResponse) {
	requestId := requestid.FromContext(ctx)
	logger := ctrl.Logger.With().Str("request_id", requestId).Str("component", "http.api.v1.user.update_profile").Logger()

	authCtx, err := fiberoapi.GetAuthContext(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get auth context")
		return nil, &fiberoapi.ErrorResponse{Code: fiber.StatusUnauthorized, Details: "Authentication required", Type: "AUTHENTICATION_REQUIRED"}
	}

	result, err := ctrl.UserApplication.UpdateProfile(userDto.UpdateProfileInput{
		UserId:    authCtx.UserID,
		Username:  req.Username,
		AvatarUrl: req.AvatarUrl,
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to update profile")
		return nil, &fiberoapi.ErrorResponse{Code: fiber.StatusInternalServerError, Details: err.Error(), Type: "UPDATE_PROFILE_FAILED"}
	}

	return &dtos.UpdateProfileResponse{
		Id:        result.User.Id,
		Username:  result.User.Username,
		Email:     result.User.Email,
		AvatarUrl: result.User.AvatarUrl,
	}, nil
}

func (ctrl *Controller) ChangePassword(ctx fiber.Ctx, req dtos.ChangePasswordRequest) (*dtos.ChangePasswordResponse, *fiberoapi.ErrorResponse) {
	requestId := requestid.FromContext(ctx)
	logger := ctrl.Logger.With().Str("request_id", requestId).Str("component", "http.api.v1.user.change_password").Logger()

	authCtx, err := fiberoapi.GetAuthContext(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get auth context")
		return nil, &fiberoapi.ErrorResponse{Code: fiber.StatusUnauthorized, Details: "Authentication required", Type: "AUTHENTICATION_REQUIRED"}
	}

	err = ctrl.UserApplication.ChangePassword(userDto.ChangePasswordInput{
		UserId:          authCtx.UserID,
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrInvalidPassword) {
			return nil, &fiberoapi.ErrorResponse{Code: fiber.StatusBadRequest, Details: "Invalid current password", Type: "INVALID_PASSWORD"}
		}
		logger.Error().Err(err).Msg("failed to change password")
		return nil, &fiberoapi.ErrorResponse{Code: fiber.StatusInternalServerError, Details: err.Error(), Type: "CHANGE_PASSWORD_FAILED"}
	}

	return &dtos.ChangePasswordResponse{Message: "Password changed successfully"}, nil
}
