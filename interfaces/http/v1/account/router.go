package account

import (
	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/interfaces/http/middleware"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/limits"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type Controller struct {
	Config             config.Config
	Logger             zerolog.Logger
	FiberOapi          *fiberoapi.OApiGroup
	AccountApplication *account.AccountApplication
	Limits             *limits.Limits
}

// SetupAccountRouter registers the routes of SYNC.md § 4.4. The changes of
// the password, recovery key and email need authKey on top of the token: they
// ask for the password again. The avatar does not.
func SetupAccountRouter(controller Controller) {
	tags := []string{"Account"}
	fiberoapi.Get(controller.FiberOapi, "/account", controller.GetAccount, fiberoapi.OpenAPIOptions{
		Summary: "Describe the account and its usage", OperationID: "account.get", Tags: tags,
	})
	fiberoapi.Post(controller.FiberOapi, "/account/password", controller.ChangePassword, fiberoapi.OpenAPIOptions{
		Summary:     "Change the master password",
		Description: "The account key is wrapped again; the other devices are revoked (SYNC.md § 3.6).",
		OperationID: "account.password", Tags: tags, SuccessStatus: 204,
	})
	fiberoapi.Post(controller.FiberOapi, "/account/recovery", controller.ChangeRecovery, fiberoapi.OpenAPIOptions{
		Summary: "Replace the recovery key", OperationID: "account.recovery", Tags: tags, SuccessStatus: 204,
	})
	fiberoapi.Post(controller.FiberOapi, "/account/email", controller.ChangeEmail, fiberoapi.OpenAPIOptions{
		Summary: "Change the identifier", OperationID: "account.email", Tags: tags, SuccessStatus: 204,
	})
	fiberoapi.Post(controller.FiberOapi, "/account/delete", controller.DeleteAccount, fiberoapi.OpenAPIOptions{
		Summary:     "Delete the account",
		Description: "The account, its devices, records, packages and avatar are erased at once.",
		OperationID: "account.delete", Tags: tags, SuccessStatus: 204,
	})
	fiberoapi.Put(controller.FiberOapi, "/account/avatar", controller.PutAvatar, fiberoapi.OpenAPIOptions{
		Summary:            "Upload or replace the encrypted avatar",
		Description:        "201 for a first avatar, 200 when it replaces the previous one.",
		OperationID:        "account.avatar.put",
		Tags:               tags,
		RequestContentType: fiber.MIMEOctetStream,
		SuccessStatus:      fiber.StatusCreated,
		Responses: map[string]any{
			"200": map[string]any{"description": "The previous avatar is replaced"},
		},
		// The JSON limit of the group does not apply: this limit does.
		Middlewares: []fiber.Handler{middleware.BodyLimit(controller.Config.Sync.MaxAvatarBytes, problem.AvatarTooLarge, nil)},
	})
	fiberoapi.Get(controller.FiberOapi, "/account/avatar", controller.GetAvatar, fiberoapi.OpenAPIOptions{
		Summary:             "Download the encrypted avatar",
		OperationID:         "account.avatar.get",
		Tags:                tags,
		ResponseContentType: fiber.MIMEOctetStream,
	})
	fiberoapi.Delete(controller.FiberOapi, "/account/avatar", controller.DeleteAvatar, fiberoapi.OpenAPIOptions{
		Summary:       "Delete the avatar",
		Description:   "204 even when the account has none.",
		OperationID:   "account.avatar.delete",
		Tags:          tags,
		SuccessStatus: fiber.StatusNoContent,
	})
}
