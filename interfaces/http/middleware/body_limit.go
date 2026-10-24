package middleware

import (
	"fmt"

	"github.com/matchabrowser/syncserver/interfaces/http/problem"

	"github.com/gofiber/fiber/v3"
)

// BodyLimit answers with the problem code (413) when the request body is
// larger than limit bytes. The body is already in memory (Fiber reads it
// whole, up to fiber.Config.BodyLimit): this keeps a large body away from the
// JSON decoder. skip, when set, exempts a request.
func BodyLimit(limit int, code string, skip func(fiber.Ctx) bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		if skip != nil && skip(c) {
			return c.Next()
		}
		if len(c.Body()) > limit {
			return problem.Write(c, problem.New(code, fmt.Sprintf("the request body is larger than %d bytes", limit)))
		}
		return c.Next()
	}
}
