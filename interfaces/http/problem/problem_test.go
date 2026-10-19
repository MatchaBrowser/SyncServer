package problem

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
)

// TestNew_StatusesOfTheSpec checks every code against the table of SYNC.md § 4.8.
func TestNew_StatusesOfTheSpec(t *testing.T) {
	spec := map[string]int{
		InvalidRequest:     400,
		WeakKdf:            400,
		InvalidCredentials: 401,
		TokenExpired:       401,
		InvalidToken:       401,
		DeviceRevoked:      401,
		AccountDeleted:     401,
		RegistrationClosed: 403,
		InvitationRequired: 403,
		NotFound:           404,
		EmailTaken:         409,
		BlobExists:         409,
		CursorExpired:      410,
		RequestTooLarge:    413,
		BlobTooLarge:       413,
		UnsupportedClient:  426,
		RateLimited:        429,
		ServerError:        500,
		Unavailable:        503,
		QuotaExceeded:      507,
	}
	if len(spec) != len(definitions) {
		t.Fatalf("%d codes defined, %d in the spec", len(definitions), len(spec))
	}
	for code, status := range spec {
		p := New(code, "")
		if p.Status != status {
			t.Errorf("%s: status %d, want %d", code, p.Status, status)
		}
		if p.Code != code || p.Title == "" {
			t.Errorf("%s: code %q, title %q", code, p.Code, p.Title)
		}
	}
}

func TestNew_UnknownCodeIsAServerError(t *testing.T) {
	if p := New("no_such_code", ""); p.Status != 500 {
		t.Errorf("status %d, want 500", p.Status)
	}
}

type body struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func run(t *testing.T, app *fiber.App, method, path string) (int, string, body) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(method, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var b body
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("body %q: %v", raw, err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), b
}

func TestErrorHandler(t *testing.T) {
	isBlobUpload := func(c fiber.Ctx) bool { return c.Method() == fiber.MethodPut }
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(zerolog.Nop(), isBlobUpload)})
	app.All("/too-large", func(c fiber.Ctx) error { return fiber.ErrRequestEntityTooLarge })
	app.Get("/internal", func(c fiber.Ctx) error { return errors.New("database password in a message") })
	app.Get("/bad", func(c fiber.Ctx) error { return fiber.NewError(fiber.StatusBadRequest, "bad header") })

	status, contentType, b := run(t, app, "POST", "/too-large")
	if status != 413 || b.Code != RequestTooLarge {
		t.Errorf("POST over the limit: %d %q", status, b.Code)
	}
	if contentType != "application/problem+json" {
		t.Errorf("Content-Type %q", contentType)
	}

	if status, _, b = run(t, app, "PUT", "/too-large"); status != 413 || b.Code != BlobTooLarge {
		t.Errorf("package over the limit: %d %q", status, b.Code)
	}

	status, _, b = run(t, app, "GET", "/internal")
	if status != 500 || b.Code != ServerError {
		t.Errorf("internal error: %d %q", status, b.Code)
	}
	if b.Detail != "" {
		t.Errorf("an internal error must not be sent to the client, got detail %q", b.Detail)
	}

	if status, _, b = run(t, app, "GET", "/bad"); status != 400 || b.Code != InvalidRequest {
		t.Errorf("client error: %d %q", status, b.Code)
	}

	if status, _, b = run(t, app, "GET", "/missing"); status != 404 || b.Code != NotFound {
		t.Errorf("unknown route: %d %q", status, b.Code)
	}
}
