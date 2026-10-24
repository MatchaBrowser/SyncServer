// Package authn connects the access tokens to fiber-oapi: it validates the
// Bearer token of the protected routes, and gives the handlers the device
// behind it.
package authn

import (
	"context"
	"errors"
	"time"

	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/domain"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
)

// Claims of the AuthContext.
const (
	claimDeviceID  = "deviceId"
	claimTokenHash = "tokenHash"
	claimExpiresAt = "expiresAt"
)

// authService validates the access tokens with the account application. The
// embedded TokenValidatorFunc provides the methods that fiber-oapi needs and
// this API does not use (roles, resources).
type authService struct {
	fiberoapi.TokenValidatorFunc
	app *account.AccountApplication
}

// NewAuthService returns the AuthService of fiber-oapi.
func NewAuthService(app *account.AccountApplication) fiberoapi.AuthorizationService {
	s := authService{app: app}
	s.TokenValidatorFunc = func(token string) (*fiberoapi.AuthContext, error) {
		return s.validate(context.Background(), token)
	}
	return s
}

// ValidateTokenCtx is preferred by fiber-oapi: the request context is passed
// to the database.
func (s authService) ValidateTokenCtx(c fiber.Ctx, token string) (*fiberoapi.AuthContext, error) {
	return s.validate(c.Context(), token)
}

func (s authService) validate(ctx context.Context, token string) (*fiberoapi.AuthContext, error) {
	principal, err := s.app.ValidateAccessToken(ctx, token)
	if err != nil {
		for _, known := range []error{domain.ErrInvalidToken, domain.ErrTokenExpired, domain.ErrDeviceRevoked, domain.ErrAccountDeleted} {
			if errors.Is(err, known) {
				return nil, err
			}
		}
		// A failure of the database is not the fault of the client.
		return nil, &fiberoapi.AuthError{StatusCode: fiber.StatusInternalServerError, Message: "token validation failed", Err: err}
	}
	return &fiberoapi.AuthContext{
		UserID: principal.AccountID,
		Claims: map[string]any{
			claimDeviceID:  principal.DeviceID,
			claimTokenHash: principal.TokenHash,
			claimExpiresAt: principal.ExpiresAt,
		},
	}, nil
}

// Principal is the device behind the request.
type Principal struct {
	AccountID string
	DeviceID  string
	TokenHash []byte
	ExpiresAt time.Time
}

// FromContext returns the device behind a request of a protected route.
func FromContext(c fiber.Ctx) (Principal, bool) {
	auth, err := fiberoapi.GetAuthContext(c)
	if err != nil || auth.UserID == "" {
		return Principal{}, false
	}
	deviceID, _ := auth.Claims[claimDeviceID].(string)
	tokenHash, _ := auth.Claims[claimTokenHash].([]byte)
	expiresAt, _ := auth.Claims[claimExpiresAt].(time.Time)
	if deviceID == "" {
		return Principal{}, false
	}
	return Principal{AccountID: auth.UserID, DeviceID: deviceID, TokenHash: tokenHash, ExpiresAt: expiresAt}, true
}
