package session

import (
	"fmt"
	"time"

	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/matchabrowser/syncserver/application/session/dto"
	userDto "github.com/matchabrowser/syncserver/application/user/dto"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/apperrors"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/tokenutil"
)

func (c *SessionApplication) ValidateToken(input dto.ValidateTokenInput) (*dto.ValidateTokenOutput, error) {
	logger := c.Logger.With().Str("component", "application.session.validate_token").Logger()

	sessionId, err := tokenutil.GetSessionIdFromToken(input.Token, c.Config)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get session id from token")
		return nil, apperrors.ErrInvalidToken
	}

	session, err := c.SessionPers.GetById(sessionId)
	if err != nil {
		logger.Error().Err(err).Str("session_id", sessionId).Msg("failed to get session by id")
		return nil, apperrors.ErrInvalidToken
	}

	if session.ExpiresAt.Before(time.Now()) {
		logger.Warn().Str("session_id", sessionId).Msg("session has expired")
		return nil, apperrors.ErrSessionExpired
	}

	// Fetch user to populate roles
	_, err = c.UserApplication.GetByUserId(userDto.GetByUserIdInput{UserId: session.UserId})
	if err != nil {
		logger.Error().Err(err).Str("user_id", session.UserId).Msg("failed to get user for role population")
		return nil, apperrors.ErrInvalidToken
	}

	ctx := &fiberoapi.AuthContext{
		UserID: session.UserId,
		Claims: map[string]any{
			"session_id": session.Id,
		},
	}

	return &dto.ValidateTokenOutput{AuthContext: ctx}, nil
}

func (c *SessionApplication) HasRole(input dto.HasRoleInput) bool {
	for _, r := range input.Context.Roles {
		if r == input.Role {
			return true
		}
	}
	return false
}

func (c *SessionApplication) HasScope(input dto.HasScopeInput) bool {
	for _, s := range input.Context.Scopes {
		if s == input.Scope {
			return true
		}
	}
	return false
}

func (c *SessionApplication) CanAccessResource(input dto.CanAccessResourceInput) (bool, error) {
	return true, nil // Default to true for now, implement actual access control logic later
}

func actionToRequiredRole(action string) string {
	switch action {
	case "read":
		return "viewer"
	case "create", "write":
		return "editor"
	case "delete":
		return "owner"
	default:
		return "viewer"
	}
}

func (c *SessionApplication) GetUserPermissions(input dto.GetUserPermissionsInput) (*dto.GetUserPermissionsOutput, error) {
	logger := c.Logger.With().Str("component", "application.session.get_user_permissions").Logger()

	logger.Warn().Msg("not implemented")

	return nil, fmt.Errorf("not implemented")
}
