package ports

import (
	"github.com/matchabrowser/syncserver/application/session/dto"
)

type SessionPort interface {
	Create(input dto.CreateSessionInput) (*dto.CreateSessionOutput, error)
	DeleteExpired() error
	ValidateToken(input dto.ValidateTokenInput) (*dto.ValidateTokenOutput, error)
	HasRole(input dto.HasRoleInput) bool
	HasScope(input dto.HasScopeInput) bool
	CanAccessResource(input dto.CanAccessResourceInput) (bool, error)
	GetUserPermissions(input dto.GetUserPermissionsInput) (*dto.GetUserPermissionsOutput, error)
	InvalidateSession(input dto.InvalidateSessionInput) error
}
