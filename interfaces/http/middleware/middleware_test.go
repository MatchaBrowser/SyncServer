package middleware

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/interfaces/http/problem"

	"github.com/gofiber/fiber/v3"
)

func ok(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

// do sends the request and returns its status and the code of its problem.
func do(t *testing.T, app *fiber.App, method, path, body string, header map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode < 400 {
		return resp.StatusCode, ""
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type %q, want application/problem+json", ct)
	}
	raw, _ := io.ReadAll(resp.Body)
	var p struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("body %q: %v", raw, err)
	}
	return resp.StatusCode, p.Code
}

func TestBodyLimit(t *testing.T) {
	app := fiber.New()
	skip := func(c fiber.Ctx) bool { return c.Path() == "/upload" }
	app.Use(BodyLimit(10, problem.RequestTooLarge, skip))
	app.Post("/json", ok)
	app.Post("/upload", ok)

	if status, _ := do(t, app, "POST", "/json", "0123456789", nil); status != 200 {
		t.Errorf("a body at the limit: %d", status)
	}
	if status, code := do(t, app, "POST", "/json", "0123456789a", nil); status != 413 || code != problem.RequestTooLarge {
		t.Errorf("a body over the limit: %d %q", status, code)
	}
	if status, _ := do(t, app, "POST", "/upload", strings.Repeat("x", 100), nil); status != 200 {
		t.Errorf("an exempted request: %d", status)
	}
}

func TestClientVersion(t *testing.T) {
	if handler, err := ClientVersion(""); handler != nil || err != nil {
		t.Fatalf("no minimum: handler %v, err %v", handler, err)
	}
	if _, err := ClientVersion("one.two"); err == nil {
		t.Fatal("an invalid minimum must be an error")
	}

	handler, err := ClientVersion("1.4")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(handler)
	app.Get("/", ok)

	tests := []struct {
		header string
		status int
	}{
		{"", 426},
		{"garbage", 426},
		{"1.3.9 (412)", 426},
		{"1.4.0 (420)", 200},
		{"1.4 (420)", 200},
		{"1.10.2-beta (500)", 200},
		{"2.0.0", 200},
	}
	for _, tt := range tests {
		header := map[string]string{}
		if tt.header != "" {
			header[ClientHeader] = tt.header
		}
		status, code := do(t, app, "GET", "/", "", header)
		if status != tt.status {
			t.Errorf("Matcha-Client %q: status %d, want %d", tt.header, status, tt.status)
		}
		if status == 426 && code != problem.UnsupportedClient {
			t.Errorf("Matcha-Client %q: code %q", tt.header, code)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"1", Version{1, 0, 0}, true},
		{"1.4", Version{1, 4, 0}, true},
		{"1.4.2", Version{1, 4, 2}, true},
		{"1.4.2-rc.1", Version{1, 4, 2}, true},
		{"1.4.2+512", Version{1, 4, 2}, true},
		{"", Version{}, false},
		{"1.4.2.7", Version{}, false},
		{"v1.4", Version{}, false},
		{"1.-4", Version{}, false},
	}
	for _, tt := range tests {
		got, err := ParseVersion(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseVersion(%q) = %v, %v", tt.in, got, err)
		}
	}
	if !(Version{1, 9, 0}).Less(Version{1, 10, 0}) {
		t.Error("1.9.0 must be older than 1.10.0")
	}
}
