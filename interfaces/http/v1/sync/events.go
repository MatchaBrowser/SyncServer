package sync

import (
	"bufio"
	"fmt"
	"time"

	"github.com/matchabrowser/syncserver/infrastructure/events"
	"github.com/matchabrowser/syncserver/interfaces/http/authn"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
)

// eventStream is the Server-Sent Events stream of a device (SYNC.md § 4.6):
//   - "event: changes" with the version, when another device of the account
//     applied changes;
//   - ": ping" every 30 seconds;
//   - "event: expired" when the access token that opened it expires, then the
//     end of the stream: the client renews its token and reconnects;
//   - the end of the stream, without "expired", when the device is revoked or
//     the account deleted (SYNC.md § 4.9).
type eventStream struct {
	hub       *events.Hub
	accountID string
	deviceID  string
	expiresAt time.Time
}

func (s *eventStream) Respond(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	// nginx must not buffer the events.
	c.Set("X-Accel-Buffering", "no")

	// The subscription is taken in the writer, which ends it: a stream that is
	// never written cannot leak it.
	return c.SendStreamWriter(func(w *bufio.Writer) {
		sub := s.hub.Subscribe(s.accountID, s.deviceID)
		defer s.hub.Unsubscribe(sub)

		ping := time.NewTicker(s.hub.PingInterval)
		defer ping.Stop()
		expiry := time.NewTimer(time.Until(s.expiresAt))
		defer expiry.Stop()

		// A first comment sends the headers at once.
		fmt.Fprint(w, ": ping\n\n")
		for {
			if err := w.Flush(); err != nil {
				return // the client is gone
			}
			select {
			case version := <-sub.Notifications():
				fmt.Fprintf(w, "event: changes\ndata: {\"version\":%d}\n\n", version)
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
			case <-expiry.C:
				fmt.Fprint(w, "event: expired\n\n")
				_ = w.Flush()
				return
			case <-sub.Closed():
				return
			}
		}
	})
}

func (ctrl Controller) Events(ctx fiber.Ctx, input dtos.Empty) (*eventStream, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	return &eventStream{hub: ctrl.EventsHub, accountID: principal.AccountID, deviceID: principal.DeviceID, expiresAt: principal.ExpiresAt}, nil
}
