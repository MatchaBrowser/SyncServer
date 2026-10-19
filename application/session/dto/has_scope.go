package dto

import fiberoapi "github.com/labbs/fiber-oapi/v3"

type HasScopeInput struct {
	Context *fiberoapi.AuthContext
	Scope   string
}
