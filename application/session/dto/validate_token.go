package dto

import fiberoapi "github.com/labbs/fiber-oapi/v3"

type ValidateTokenInput struct {
	Token string
}

type ValidateTokenOutput struct {
	AuthContext *fiberoapi.AuthContext
}
