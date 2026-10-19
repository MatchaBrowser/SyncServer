package http

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	infrahttp "github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
)

const (
	testMaxRequestBytes = 1024
	testMaxBlobBytes    = 2048
)

// newTestApp builds the HTTP server as the server command does.
func newTestApp(t *testing.T, minimumClientVersion string) *fiber.App {
	t.Helper()
	var cfg config.Config
	cfg.Version = "test"
	cfg.Server.MinimumClientVersion = minimumClientVersion
	cfg.Sync.MaxRequestBytes = testMaxRequestBytes
	cfg.Sync.MaxBlobBytes = testMaxBlobBytes

	deps := infrastructure.Deps{Config: cfg, Logger: zerolog.Nop()}
	var err error
	deps.Http, err = infrahttp.Configure(cfg, deps.Logger, HTTPOptions(deps))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetupRoutes(deps); err != nil {
		t.Fatal(err)
	}
	return deps.Http.Fiber
}

type response struct {
	status      int
	contentType string
	code        string
	raw         []byte
}

func send(t *testing.T, app *fiber.App, method, path string, body []byte, header map[string]string) response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	r := response{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), raw: raw}
	var p struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &p) == nil {
		r.code = p.Code
	}
	return r
}

func TestHealth(t *testing.T) {
	app := newTestApp(t, "")
	r := send(t, app, "GET", "/api/health", nil, nil)
	if r.status != 200 {
		t.Fatalf("status %d: %s", r.status, r.raw)
	}
	var health struct{ Status, Service, Version string }
	if err := json.Unmarshal(r.raw, &health); err != nil {
		t.Fatal(err)
	}
	if health.Status != "ok" || health.Service != "syncserver" || health.Version != "test" {
		t.Errorf("health %+v", health)
	}
}

func TestUnknownRouteIsAProblem(t *testing.T) {
	app := newTestApp(t, "")
	r := send(t, app, "GET", "/api/v1/nothing-here", nil, nil)
	if r.status != 404 || r.code != problem.NotFound {
		t.Fatalf("status %d, code %q: %s", r.status, r.code, r.raw)
	}
	if r.contentType != "application/problem+json" {
		t.Errorf("Content-Type %q", r.contentType)
	}
}

func TestRequestSizeLimits(t *testing.T) {
	app := newTestApp(t, "")

	// A JSON request over sync.max_request_bytes.
	r := send(t, app, "POST", "/api/v1/sync/push", make([]byte, testMaxRequestBytes+1), nil)
	if r.status != 413 || r.code != problem.RequestTooLarge {
		t.Errorf("JSON body over the limit: status %d, code %q", r.status, r.code)
	}

	// A package upload is exempted from that limit: it reaches the route,
	// which asks for a token.
	r = send(t, app, "PUT", "/api/v1/blobs/5a1c9e3b-7d2f-4b8a-9c6e-1f0d3b7a2e58", make([]byte, testMaxRequestBytes+1), nil)
	if r.status != 401 || r.code != problem.InvalidToken {
		t.Errorf("package under the server limit: status %d, code %q", r.status, r.code)
	}

	// Over the limit of the server, fasthttp rejects the body before any
	// route and Fiber hands fiber.ErrRequestEntityTooLarge to the
	// ErrorHandler (tested in the problem package). app.Test cannot observe
	// that response: it returns the error of fasthttp instead.
	if limit := app.Config().BodyLimit; limit != testMaxBlobBytes+1<<20 {
		t.Errorf("BodyLimit %d, want sync.max_blob_bytes + 1 MiB", limit)
	}
}

func TestMinimumClientVersion(t *testing.T) {
	app := newTestApp(t, "1.2")

	r := send(t, app, "GET", "/api/v1/nothing-here", nil, map[string]string{"Matcha-Client": "1.1.9 (100)"})
	if r.status != 426 || r.code != problem.UnsupportedClient {
		t.Errorf("old client: status %d, code %q", r.status, r.code)
	}
	r = send(t, app, "GET", "/api/v1/nothing-here", nil, map[string]string{"Matcha-Client": "1.2.0 (120)"})
	if r.status != 404 {
		t.Errorf("recent client: status %d, code %q", r.status, r.code)
	}
	// The health route is outside the API: it answers any client.
	if r = send(t, app, "GET", "/api/health", nil, nil); r.status != 200 {
		t.Errorf("health without Matcha-Client: status %d", r.status)
	}
}
