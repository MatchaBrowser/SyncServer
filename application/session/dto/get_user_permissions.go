package dto

import fiberoapi "github.com/labbs/fiber-oapi/v3"

type GetUserPermissionsInput struct {
	Context      *fiberoapi.AuthContext
	ResourceType string
	ResourceID   string
}

type GetUserPermissionsOutput struct {
	Permission *fiberoapi.ResourcePermission
}
