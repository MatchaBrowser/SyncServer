package infrastructure

import (
	"github.com/matchabrowser/syncserver/application/auth"
	"github.com/matchabrowser/syncserver/application/session"
	"github.com/matchabrowser/syncserver/application/user"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/cronscheduler"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/rs/zerolog"
)

type Deps struct {
	Config        config.Config
	Logger        zerolog.Logger
	Http          http.Config
	CronScheduler cronscheduler.Config
	Database      database.Config

	UserApplication    *user.UserApplication
	SessionApplication *session.SessionApplication
	AuthApplication    *auth.AuthApplication
}
