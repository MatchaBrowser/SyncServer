package dto

import fiberoapi "github.com/labbs/fiber-oapi/v3"

type HasRoleInput struct {
	Context *fiberoapi.AuthContext
	Role    string
}
