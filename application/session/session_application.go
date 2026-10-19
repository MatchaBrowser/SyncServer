package session

import (
	"github.com/matchabrowser/syncserver/application/ports"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/rs/zerolog"
)

type SessionApplication struct {
	Config          config.Config
	Logger          zerolog.Logger
	SessionPers     domain.SessionPers
	UserApplication ports.UserPort
}

func NewSessionApplication(config config.Config, logger zerolog.Logger, sessionPers domain.SessionPers) *SessionApplication {
	return &SessionApplication{
		Config:      config,
		Logger:      logger,
		SessionPers: sessionPers,
	}
}

// SessionApp is a type alias for backward compatibility
type SessionApp = SessionApplication
