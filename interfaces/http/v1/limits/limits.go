// Package limits holds the rate limits of the API (SYNC.md § 4.1).
package limits

import (
	"math"
	"strconv"
	"time"

	"github.com/matchabrowser/syncserver/domain"
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

	// Audit, when set, records the rejections: at most one per key and per
	// quarter of an hour, so that an attack does not fill the journal.
	Audit   domain.AuditRecorder
	audited *ratelimit.Limiter
}

// Kinds of limits, as the audit journal names them.
const (
	KindAuthKey  = "auth_key"
	KindPrelogin = "prelogin"
	KindRegister = "register"
)

func New() *Limits {
	return &Limits{
		authKeyPair:    ratelimit.New(10, 15*time.Minute),
		authKeyAccount: ratelimit.New(100, time.Hour),
		prelogin:       ratelimit.New(60, 15*time.Minute),
		register:       ratelimit.New(10, time.Hour),
		audited:        ratelimit.New(1, 15*time.Minute),
	}
}

// AuthKey checks an attempt on a route that verifies a key of the account
// (its email before login, its id after).
func (l *Limits) AuthKey(c fiber.Ctx, account string) *fiberoapi.Problem {
	ok, retryAfter := ratelimit.AllowAll(
		ratelimit.Attempt{Limiter: l.authKeyPair, Key: c.IP() + "|" + account},
		ratelimit.Attempt{Limiter: l.authKeyAccount, Key: account},
	)
	return l.reject(c, ok, retryAfter, KindAuthKey, account)
}

// Prelogin checks a prelogin from the IP address of the request.
func (l *Limits) Prelogin(c fiber.Ctx) *fiberoapi.Problem {
	ok, retryAfter := l.prelogin.Allow(c.IP())
	return l.reject(c, ok, retryAfter, KindPrelogin, "")
}

// Register checks an account creation from the IP address of the request.
func (l *Limits) Register(c fiber.Ctx) *fiberoapi.Problem {
	ok, retryAfter := l.register.Allow(c.IP())
	return l.reject(c, ok, retryAfter, KindRegister, "")
}

// reject returns nil, or a rate_limited problem with its Retry-After header.
func (l *Limits) reject(c fiber.Ctx, ok bool, retryAfter time.Duration, kind, key string) *fiberoapi.Problem {
	if ok {
		return nil
	}
	l.record(c, kind, key)
	c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	return problem.New(problem.RateLimited, "")
}

// record writes a rate_limited event, once per (kind, IP, key) and quarter of
// an hour. The key, an email before login, stays in memory: the event only
// has the account of an authenticated request, from the origin.
func (l *Limits) record(c fiber.Ctx, kind, key string) {
	if l.Audit == nil {
		return
	}
	if first, _ := l.audited.Allow(kind + "|" + c.IP() + "|" + key); !first {
		return
	}
	l.Audit.Record(c.Context(), domain.AuditEvent{Action: domain.AuditRateLimited, Detail: kind})
}
