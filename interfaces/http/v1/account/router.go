package account

import (
	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/limits"

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

// SetupAccountRouter registers the routes of SYNC.md § 4.4. The changes need
// authKey on top of the token: they ask for the password again.
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
		Description: "The account, its devices, records and packages are erased at once.",
		OperationID: "account.delete", Tags: tags, SuccessStatus: 204,
	})
}
