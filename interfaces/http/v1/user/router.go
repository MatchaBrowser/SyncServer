package user

import (
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/matchabrowser/syncserver/application/user"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/rs/zerolog"
)

type Controller struct {
	Config          config.Config
	Logger          zerolog.Logger
	FiberOapi       *fiberoapi.OApiGroup
	UserApplication *user.UserApplication
}

func SetupUserRouter(controller Controller) {
	fiberoapi.Get(controller.FiberOapi, "/profile", controller.GetProfile, fiberoapi.OpenAPIOptions{
		Summary:     "Get user profile",
		Description: "Retrieve the profile of the authenticated user",
		OperationID: "user.getProfile",
		Tags:        []string{"User"},
	})

	// Profile management
	fiberoapi.Put(controller.FiberOapi, "/profile", controller.UpdateProfile, fiberoapi.OpenAPIOptions{
		Summary:     "Update user profile",
		Description: "Update the authenticated user's profile (username, avatar, preferences)",
		OperationID: "user.updateProfile",
		Tags:        []string{"User"},
	})
	fiberoapi.Post(controller.FiberOapi, "/change-password", controller.ChangePassword, fiberoapi.OpenAPIOptions{
		Summary:     "Change password",
		Description: "Change the authenticated user's password",
		OperationID: "user.changePassword",
		Tags:        []string{"User"},
	})
}
