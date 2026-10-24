package problem

import (
	"context"
	"errors"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

// codes maps the errors of the domain to the codes of the API.
var codes = []struct {
	err  error
	code string
}{
	{domain.ErrRegistrationClosed, RegistrationClosed},
	{domain.ErrInvitationRequired, InvitationRequired},
	{domain.ErrEmailTaken, EmailTaken},
	{domain.ErrWeakKdf, WeakKdf},
	{domain.ErrInvalidCredentials, InvalidCredentials},
	{domain.ErrInvalidToken, InvalidToken},
	{domain.ErrTokenExpired, TokenExpired},
	{domain.ErrDeviceRevoked, DeviceRevoked},
	{domain.ErrAccountDeleted, AccountDeleted},
	{domain.ErrNotFound, NotFound},
	{domain.ErrInvalidCursor, InvalidRequest},
	{domain.ErrCursorExpired, CursorExpired},
	{domain.ErrTooManyChanges, RequestTooLarge},
	{domain.ErrIdempotencyMismatch, InvalidRequest},
	{domain.ErrBlobExists, BlobExists},
	{domain.ErrBlobTooLarge, BlobTooLarge},
	{domain.ErrQuotaExceeded, QuotaExceeded},
}

// FromError returns the problem for an error of a use case. An unexpected
// error is logged and answered as server_error, without its message.
func FromError(logger zerolog.Logger, err error) *fiberoapi.Problem {
	for _, c := range codes {
		if errors.Is(err, c.err) {
			return New(c.code, "")
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return New(Unavailable, "")
	}
	logger.Error().Err(err).Str("event", "http.unexpected_error").Msg("Unexpected error")
	return New(ServerError, "")
}

// AuthErrorHandler answers the authentication failures of fiber-oapi with the
// code of their cause: token_expired, device_revoked, account_deleted, or
// invalid_token (missing, unknown or malformed token).
func AuthErrorHandler(logger zerolog.Logger) fiberoapi.AuthErrorHandler {
	logger = logger.With().Str("component", "http.auth_error_handler").Logger()
	return func(c fiber.Ctx, err *fiberoapi.AuthError) error {
		for _, code := range []struct {
			err  error
			code string
		}{
			{domain.ErrTokenExpired, TokenExpired},
			{domain.ErrDeviceRevoked, DeviceRevoked},
			{domain.ErrAccountDeleted, AccountDeleted},
			{domain.ErrInvalidToken, InvalidToken},
		} {
			if errors.Is(err, code.err) {
				return Write(c, New(code.code, ""))
			}
		}
		if err.StatusCode >= 500 {
			logger.Error().Err(err).Str("event", "http.auth.failed").Msg("Authentication failed on the server side")
			return Write(c, New(ServerError, ""))
		}
		return Write(c, New(InvalidToken, ""))
	}
}
