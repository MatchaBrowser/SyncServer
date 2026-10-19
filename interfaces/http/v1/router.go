package v1

import (
	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/auth"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/user"
)

func SetupRouterV1(deps infrastructure.Deps) {
	deps.Logger.Info().Str("component", "http.router.v1").Msg("Setting up API v1 routes")
	grp := deps.Http.FiberOapi.Group("/api/v1")

	authCtrl := auth.Controller{
		Config:          deps.Config,
		Logger:          deps.Logger,
		FiberOapi:       grp.Group("/auth"),
		AuthApplication: deps.AuthApplication,
	}
	auth.SetupAuthRouter(authCtrl)

	userCtrl := user.Controller{
		Config:          deps.Config,
		Logger:          deps.Logger,
		FiberOapi:       grp.Group("/user"),
		UserApplication: deps.UserApplication,
	}
	user.SetupUserRouter(userCtrl)
}
