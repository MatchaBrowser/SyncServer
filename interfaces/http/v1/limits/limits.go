// Package limits holds the rate limits of the API (SYNC.md § 4.1).
package limits

import (
	"math"
	"strconv"
	"time"

	"github.com/matchabrowser/syncserver/infrastructure/ratelimit"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
)

// Limits are the rate limits of the routes that check a key or create
// accounts. They are kept in memory: the server is a single instance.
type Limits struct {
	// 10 attempts per quarter of an hour per (IP address, account), and 100
	// per hour per account, on every route that checks authKey or
	// recoveryAuthKey. A limit per account as low as the first would let
	// anyone block the login of an account.
	authKeyPair    *ratelimit.Limiter
	authKeyAccount *ratelimit.Limiter
	prelogin       *ratelimit.Limiter
	register       *ratelimit.Limiter
}

func New() *Limits {
	return &Limits{
		authKeyPair:    ratelimit.New(10, 15*time.Minute),
		authKeyAccount: ratelimit.New(100, time.Hour),
		prelogin:       ratelimit.New(60, 15*time.Minute),
		register:       ratelimit.New(10, time.Hour),
	}
}

// AuthKey checks an attempt on a route that verifies a key of the account
// (its email before login, its id after).
func (l *Limits) AuthKey(c fiber.Ctx, account string) *fiberoapi.Problem {
	ok, retryAfter := ratelimit.AllowAll(
		ratelimit.Attempt{Limiter: l.authKeyPair, Key: c.IP() + "|" + account},
		ratelimit.Attempt{Limiter: l.authKeyAccount, Key: account},
	)
	return reject(c, ok, retryAfter)
}

// Prelogin checks a prelogin from the IP address of the request.
func (l *Limits) Prelogin(c fiber.Ctx) *fiberoapi.Problem {
	ok, retryAfter := l.prelogin.Allow(c.IP())
	return reject(c, ok, retryAfter)
}

// Register checks an account creation from the IP address of the request.
func (l *Limits) Register(c fiber.Ctx) *fiberoapi.Problem {
	ok, retryAfter := l.register.Allow(c.IP())
	return reject(c, ok, retryAfter)
}

// reject returns nil, or a rate_limited problem with its Retry-After header.
func reject(c fiber.Ctx, ok bool, retryAfter time.Duration) *fiberoapi.Problem {
	if ok {
		return nil
	}
	c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	return problem.New(problem.RateLimited, "")
}
