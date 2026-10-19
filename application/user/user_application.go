package user

import (
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/rs/zerolog"
)

type UserApplication struct {
	Config   config.Config
	Logger   zerolog.Logger
	UserPres domain.UserPers
}

func NewUserApplication(
	config config.Config,
	logger zerolog.Logger,
	userPers domain.UserPers) *UserApplication {
	return &UserApplication{
		Config:   config,
		Logger:   logger,
		UserPres: userPers,
	}
}

// UserApp is a type alias for backward compatibility
type UserApp = UserApplication
