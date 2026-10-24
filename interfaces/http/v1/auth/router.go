package auth

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

// SetupAuthRouter registers the routes of SYNC.md § 4.3, which give a
// session. Only logout needs one.
func SetupAuthRouter(controller Controller) {
	public := func(options fiberoapi.OpenAPIOptions) fiberoapi.OpenAPIOptions {
		options.Tags = []string{"Sessions"}
		options.Security = "disabled"
		return options
	}

	fiberoapi.Post(controller.FiberOapi, "/auth/prelogin", controller.Prelogin, public(fiberoapi.OpenAPIOptions{
		Summary:     "Key derivation parameters of an account",
		Description: "An unknown identifier gets plausible and stable parameters: prelogin does not tell which accounts exist.",
		OperationID: "auth.prelogin",
	}))
	fiberoapi.Post(controller.FiberOapi, "/accounts", controller.CreateAccount, public(fiberoapi.OpenAPIOptions{
		Summary:       "Create an account",
		Description:   "Registers the proofs and envelopes of the account key, and opens the session of the first device (SYNC.md § 3.3).",
		OperationID:   "accounts.create",
		SuccessStatus: 201,
	}))
	fiberoapi.Post(controller.FiberOapi, "/auth/login", controller.Login, public(fiberoapi.OpenAPIOptions{
		Summary:     "Open a session",
		Description: "A device already known for the account is reactivated (SYNC.md § 3.4).",
		OperationID: "auth.login",
	}))
	fiberoapi.Post(controller.FiberOapi, "/auth/refresh", controller.Refresh, public(fiberoapi.OpenAPIOptions{
		Summary:     "Renew the tokens",
		Description: "A refresh token serves once: a reuse revokes the device.",
		OperationID: "auth.refresh",
	}))
	fiberoapi.Post(controller.FiberOapi, "/auth/logout", controller.Logout, fiberoapi.OpenAPIOptions{
		Summary:       "Close the session of this device",
		OperationID:   "auth.logout",
		Tags:          []string{"Sessions"},
		SuccessStatus: 204,
	})
	fiberoapi.Post(controller.FiberOapi, "/auth/recover/start", controller.RecoverStart, public(fiberoapi.OpenAPIOptions{
		Summary:     "Start a recovery with the recovery key",
		OperationID: "auth.recover.start",
	}))
	fiberoapi.Post(controller.FiberOapi, "/auth/recover/finish", controller.RecoverFinish, public(fiberoapi.OpenAPIOptions{
		Summary:     "Set a new master password",
		Description: "Every other device is revoked (SYNC.md § 3.7).",
		OperationID: "auth.recover.finish",
	}))
}
