package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/infrastructure/config"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// listen serves the test server on a real port: an event stream never ends,
// which app.Test cannot read.
func (s *testServer) listen() string {
	s.t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.t.Fatal(err)
	}
	go func() { _ = s.app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	s.t.Cleanup(func() {
		s.deps.EventsHub.CloseAll()
		_ = s.app.ShutdownWithTimeout(5 * time.Second)
	})
	return "http://" + ln.Addr().String()
}

// event is an event of the stream, or a comment ("ping").
type event struct {
	name string
	data string
}

type stream struct {
	t      *testing.T
	resp   *http.Response
	events chan event
}

// openStream connects to GET /sync/events and reads its events in the
// background; the channel is closed when the server ends the stream.
func openStream(t *testing.T, base, token string) *stream {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/v1/sync/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	s := &stream{t: t, resp: resp, events: make(chan event, 16)}
	if resp.StatusCode != 200 {
		close(s.events)
		return s
	}
	go func() {
		defer close(s.events)
		scanner := bufio.NewScanner(resp.Body)
		var current event
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == ": ping":
				s.events <- event{name: "ping"}
			case strings.HasPrefix(line, "event: "):
				current.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			case line == "" && current.name != "":
				s.events <- current
				current = event{}
			}
		}
	}()
	return s
}

// next returns the next event that is not a ping, or "closed" when the stream
// ends.
func (s *stream) next(timeout time.Duration) event {
	s.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case e, ok := <-s.events:
			if !ok {
				return event{name: "closed"}
			}
			if e.name != "ping" {
				return e
			}
		case <-deadline:
			s.t.Fatal("no event before the timeout")
		}
	}
}

func TestEvents_Changes(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	s.deps.EventsHub.PingInterval = 50 * time.Millisecond
	base := s.listen()
	a, b := s.connectedMacs(t)

	stream := openStream(t, base, b.session.AccessToken)
	if stream.resp.StatusCode != 200 {
		t.Fatalf("status %d", stream.resp.StatusCode)
	}
	h := stream.resp.Header
	if h.Get("Content-Type") != "text/event-stream" || h.Get("Cache-Control") != "no-cache" || h.Get("X-Accel-Buffering") != "no" || h.Get("Content-Encoding") != "" {
		t.Errorf("headers %v", h)
	}
	// The pings keep the connection alive.
	pings := 0
	for pings < 2 {
		if e := <-stream.events; e.name == "ping" {
			pings++
		}
	}

	// A pushes: B is told the version.
	results := a.results(a.change(uuid.NewString(), nil, `{}`))
	e := stream.next(2 * time.Second)
	if e.name != "changes" || e.data != fmt.Sprintf(`{"version":%d}`, results[0].Version) {
		t.Errorf("event %+v, want changes with version %d", e, results[0].Version)
	}

	// A is not told of its own changes.
	own := openStream(t, base, a.session.AccessToken)
	b.results(b.change(uuid.NewString(), nil, `{}`))
	if e := own.next(2 * time.Second); e.name != "changes" {
		t.Errorf("A must be told of the changes of B: %+v", e)
	}
	a.results(a.change(uuid.NewString(), nil, `{}`))
	select {
	case e := <-own.events:
		if e.name == "changes" {
			t.Errorf("A was told of its own changes: %+v", e)
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func TestEvents_Expired(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	base := s.listen()
	m := newMacAccount(t, "a@example.com", "a long password")
	s.register(m)

	// A session whose access token expires in a second.
	s.deps.AccountApplication.Now = func() time.Time { return time.Now().UTC().Add(-15*time.Minute + time.Second) }
	session, _ := s.login(m.email, m.password, "Mac mini")
	s.deps.AccountApplication.Now = func() time.Time { return time.Now().UTC().Add(-15 * time.Minute) }

	stream := openStream(t, base, session.AccessToken)
	if e := stream.next(5 * time.Second); e.name != "expired" {
		t.Fatalf("event %+v, want expired", e)
	}
	if e := stream.next(2 * time.Second); e.name != "closed" {
		t.Errorf("the stream must end after expired: %+v", e)
	}
}

func TestEvents_ClosedOnRevocation(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	base := s.listen()
	a, b := s.connectedMacs(t)

	stream := openStream(t, base, b.session.AccessToken)
	waitSubscribed(t, s, 1)
	s.expect(s.call("DELETE", "/devices/"+b.session.DeviceID, a.session.AccessToken, nil), 204, "")
	if e := stream.next(2 * time.Second); e.name != "closed" {
		t.Errorf("revoked device: %+v, want the end of the stream without expired", e)
	}
	// Reconnecting: the token is revoked.
	if again := openStream(t, base, b.session.AccessToken); again.resp.StatusCode != 401 {
		t.Errorf("reconnection of a revoked device: %d", again.resp.StatusCode)
	}

	own := openStream(t, base, a.session.AccessToken)
	waitSubscribed(t, s, 1)
	s.expect(s.call("POST", "/auth/logout", a.session.AccessToken, nil), 204, "")
	if e := own.next(2 * time.Second); e.name != "closed" {
		t.Errorf("logged out device: %+v", e)
	}
}

func TestEvents_ClosedOnPasswordChangeAndDeletion(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	base := s.listen()
	m := newMacAccount(t, "a@example.com", "a long password")
	current := s.register(m)
	other, _ := s.login(m.email, m.password, "Mac mini")

	mine := openStream(t, base, current.AccessToken)
	theirs := openStream(t, base, other.AccessToken)
	waitSubscribed(t, s, 2)

	// A new password revokes the other Mac: its stream ends, not this one.
	change := map[string]any{"authKey": m.keys.Auth, "kdf": m.kdf(), "newAuthKey": m.keys.Auth, "wrappedAccountKey": wrap(t, m.keys.Wrap, m.accountKey)}
	s.expect(s.call("POST", "/account/password", current.AccessToken, change), 204, "")
	if e := theirs.next(2 * time.Second); e.name != "closed" {
		t.Errorf("other device after a change of password: %+v", e)
	}
	waitSubscribed(t, s, 1)

	// Deleting the account ends every stream.
	s.expect(s.call("POST", "/account/delete", current.AccessToken, map[string]any{"authKey": m.keys.Auth}), 204, "")
	if e := mine.next(2 * time.Second); e.name != "closed" {
		t.Errorf("stream of a deleted account: %+v", e)
	}
	waitSubscribed(t, s, 0)
}

// waitSubscribed waits until n streams are open in the hub.
func waitSubscribed(t *testing.T, s *testServer, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for s.deps.EventsHub.Len() != n {
		select {
		case <-ctx.Done():
			t.Fatalf("%d streams open, want %d", s.deps.EventsHub.Len(), n)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
