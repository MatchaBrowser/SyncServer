package auth

import (
	"github.com/matchabrowser/syncserver/application/ports"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/rs/zerolog"
)

type AuthApplication struct {
	Config             config.Config
	Logger             zerolog.Logger
	UserApplication    ports.UserPort
	SessionApplication ports.SessionPort

	oidcUserinfoEndpoint string // cached from OIDC discovery
}

func NewAuthApplication(config config.Config, logger zerolog.Logger) *AuthApplication {
	return &AuthApplication{
		Config: config,
		Logger: logger,
	}
}
