package middleware

import (
	"strings"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/gofiber/fiber/v3"
)

// Origin puts the origin of a request of the API in its context, for the
// audit journal: a client, its IP address (behind the trusted proxies) and its
// Matcha-Client header. The validation of the token adds the account and the
// device.
func Origin(c fiber.Ctx) error {
	agent := c.Get(ClientHeader)
	if agent == "" {
		agent = c.Get(fiber.HeaderUserAgent)
	}
	// The values of fiber are only valid during the handler: copied.
	c.SetContext(domain.WithOrigin(c.Context(), domain.Origin{
		Actor: domain.ActorClient,
		IP:    strings.Clone(c.IP()),
		Agent: strings.Clone(agent),
	}))
	return c.Next()
}
