package http

import (
	"github.com/matchabrowser/syncserver/infrastructure"
	infrahttp "github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/matchabrowser/syncserver/interfaces/http/authn"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	v1 "github.com/matchabrowser/syncserver/interfaces/http/v1"
)

// HTTPOptions wires the API layer into the HTTP server: validation of the
// access tokens, error codes, error handler, and the responses left
// uncompressed.
func HTTPOptions(deps infrastructure.Deps) infrahttp.Options {
	return infrahttp.Options{
		AuthService:      authn.NewAuthService(deps.AccountApplication),
		AuthErrorHandler: problem.AuthErrorHandler(deps.Logger),
		ProblemCodes:     problem.LibraryCodes,
		ErrorHandler:     problem.ErrorHandler(deps.Logger, v1.IsBlobUpload),
		Uncompressed:     v1.Uncompressed,
	}
}
