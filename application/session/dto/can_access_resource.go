package dto

import fiberoapi "github.com/labbs/fiber-oapi/v3"

type CanAccessResourceInput struct {
	Context      *fiberoapi.AuthContext
	ResourceType string
	ResourceID   string
	Action       string
}
