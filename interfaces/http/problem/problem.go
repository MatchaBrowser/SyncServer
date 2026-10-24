// Package problem builds the error responses of the API: RFC 9457 problems
// (application/problem+json) carrying a stable code (SYNC.md § 4.8).
package problem

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

// Error codes of SYNC.md § 4.8.
const (
	InvalidRequest     = "invalid_request"
	WeakKdf            = "weak_kdf"
	InvalidCredentials = "invalid_credentials"
	TokenExpired       = "token_expired"
	InvalidToken       = "invalid_token"
	DeviceRevoked      = "device_revoked"
	AccountDeleted     = "account_deleted"
	RegistrationClosed = "registration_closed"
	InvitationRequired = "invitation_required"
	NotFound           = "not_found"
	EmailTaken         = "email_taken"
	BlobExists         = "blob_exists"
	CursorExpired      = "cursor_expired"
	RequestTooLarge    = "request_too_large"
	BlobTooLarge       = "blob_too_large"
	AvatarTooLarge     = "avatar_too_large"
	UnsupportedClient  = "unsupported_client"
	RateLimited        = "rate_limited"
	ServerError        = "server_error"
	Unavailable        = "unavailable"
	QuotaExceeded      = "quota_exceeded"
)

// definition is the status and the title of a code.
type definition struct {
	status int
	title  string
}

var definitions = map[string]definition{
	InvalidRequest:     {http.StatusBadRequest, "Invalid request"},
	WeakKdf:            {http.StatusBadRequest, "Key derivation parameters out of bounds"},
	InvalidCredentials: {http.StatusUnauthorized, "Invalid credentials"},
	TokenExpired:       {http.StatusUnauthorized, "Access token expired"},
	InvalidToken:       {http.StatusUnauthorized, "Invalid token"},
	DeviceRevoked:      {http.StatusUnauthorized, "Device revoked"},
	AccountDeleted:     {http.StatusUnauthorized, "Account deleted"},
	RegistrationClosed: {http.StatusForbidden, "Registration closed"},
	InvitationRequired: {http.StatusForbidden, "Invitation required"},
	NotFound:           {http.StatusNotFound, "Not found"},
	EmailTaken:         {http.StatusConflict, "Email already taken"},
	BlobExists:         {http.StatusConflict, "Another package has this identifier"},
	CursorExpired:      {http.StatusGone, "Cursor expired"},
	RequestTooLarge:    {http.StatusRequestEntityTooLarge, "Request too large"},
	BlobTooLarge:       {http.StatusRequestEntityTooLarge, "Package too large"},
	AvatarTooLarge:     {http.StatusRequestEntityTooLarge, "Avatar too large"},
	UnsupportedClient:  {http.StatusUpgradeRequired, "Unsupported client version"},
	RateLimited:        {http.StatusTooManyRequests, "Too many requests"},
	ServerError:        {http.StatusInternalServerError, "Server error"},
	Unavailable:        {http.StatusServiceUnavailable, "Service unavailable"},
	QuotaExceeded:      {http.StatusInsufficientStorage, "Account storage full"},
}

// LibraryCodes maps the error categories of fiber-oapi to our codes, for
// fiberoapi.Config.ProblemCodes.
var LibraryCodes = map[string]string{
	"validation_error":     InvalidRequest,
	"parse_error":          InvalidRequest,
	"type_error":           InvalidRequest,
	"authentication_error": InvalidToken,
	"not_found":            NotFound,
	"serialization_error":  ServerError,
	"internal_error":       ServerError,
}

// New returns the problem for code, with an optional human-readable detail.
// The client shows its own message, chosen from the code (SYNC.md § 4.1).
func New(code, detail string) *fiberoapi.Problem {
	def, ok := definitions[code]
	if !ok {
		def = definitions[ServerError]
	}
	return &fiberoapi.Problem{
		Title:  def.title,
		Status: def.status,
		Detail: detail,
		Code:   code,
	}
}

// Write sends p with its status and the application/problem+json type.
func Write(c fiber.Ctx, p *fiberoapi.Problem) error {
	return c.Status(p.Status).JSON(p, p.ContentType())
}

// ErrorHandler is the fiber.Config.ErrorHandler: the errors that do not go
// through a route of fiber-oapi (body over the limit of the server, panic
// caught by the recover middleware...) are sent as problems too.
// isBlobUpload tells a package upload, whose body over the limit is a
// blob_too_large rather than a request_too_large.
func ErrorHandler(logger zerolog.Logger, isBlobUpload func(fiber.Ctx) bool) fiber.ErrorHandler {
	logger = logger.With().Str("component", "http.error_handler").Logger()
	return func(c fiber.Ctx, err error) error {
		var fiberErr *fiber.Error
		if !errors.As(err, &fiberErr) {
			logger.Error().Err(err).Str("method", c.Method()).Str("path", c.Path()).Msg("Unhandled error")
			return Write(c, New(ServerError, ""))
		}

		switch fiberErr.Code {
		case fiber.StatusRequestEntityTooLarge:
			if isBlobUpload != nil && isBlobUpload(c) {
				return Write(c, New(BlobTooLarge, ""))
			}
			return Write(c, New(RequestTooLarge, ""))
		case fiber.StatusNotFound:
			return Write(c, New(NotFound, ""))
		case fiber.StatusTooManyRequests:
			return Write(c, New(RateLimited, ""))
		case fiber.StatusServiceUnavailable:
			return Write(c, New(Unavailable, ""))
		}
		if fiberErr.Code >= 400 && fiberErr.Code < 500 {
			p := New(InvalidRequest, fiberErr.Message)
			p.Status = fiberErr.Code
			return Write(c, p)
		}
		logger.Error().Err(err).Str("method", c.Method()).Str("path", c.Path()).Msg("Server error")
		return Write(c, New(ServerError, ""))
	}
}
