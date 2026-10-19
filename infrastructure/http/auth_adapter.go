package http

import (
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/matchabrowser/syncserver/application/session"
	"github.com/matchabrowser/syncserver/application/session/dto"
)

// SessionAuthAdapter adapts SessionApp to fiberoapi.AuthorizationService
type SessionAuthAdapter struct {
	sessionApp *session.SessionApp
}

func NewSessionAuthAdapter(sessionApp *session.SessionApp) *SessionAuthAdapter {
	return &SessionAuthAdapter{sessionApp: sessionApp}
}

func (a *SessionAuthAdapter) ValidateToken(token string) (*fiberoapi.AuthContext, error) {
	result, err := a.sessionApp.ValidateToken(dto.ValidateTokenInput{Token: token})
	if err != nil {
		return nil, err
	}
	return result.AuthContext, nil
}

func (a *SessionAuthAdapter) HasRole(ctx *fiberoapi.AuthContext, role string) bool {
	return a.sessionApp.HasRole(dto.HasRoleInput{Context: ctx, Role: role})
}

func (a *SessionAuthAdapter) HasScope(ctx *fiberoapi.AuthContext, scope string) bool {
	return a.sessionApp.HasScope(dto.HasScopeInput{Context: ctx, Scope: scope})
}

func (a *SessionAuthAdapter) CanAccessResource(ctx *fiberoapi.AuthContext, resourceType, resourceID, action string) (bool, error) {
	return a.sessionApp.CanAccessResource(dto.CanAccessResourceInput{
		Context:      ctx,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
	})
}

func (a *SessionAuthAdapter) GetUserPermissions(ctx *fiberoapi.AuthContext, resourceType, resourceID string) (*fiberoapi.ResourcePermission, error) {
	result, err := a.sessionApp.GetUserPermissions(dto.GetUserPermissionsInput{
		Context:      ctx,
		ResourceType: resourceType,
		ResourceID:   resourceID,
	})
	if err != nil {
		return nil, err
	}
	return result.Permission, nil
}
