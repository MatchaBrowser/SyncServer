package console

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

//go:embed templates/*.html
var templateFiles embed.FS

//go:embed static/*
var staticFiles embed.FS

// pageNames are the templates of templates/, each rendered in the layout.
var pageNames = []string{"login", "dashboard", "accounts", "account", "invitations", "audit", "error"}

// views are the parsed pages and the embedded files, with their versions.
type views struct {
	pages map[string]*template.Template
	// static holds the embedded files, by name.
	static map[string][]byte
	// versions are the first bytes of the SHA-256 of each file: its URL
	// changes with its content, so that it can be cached for good.
	versions map[string]string
}

func parseViews() (*views, error) {
	v := &views{pages: map[string]*template.Template{}, static: map[string][]byte{}, versions: map[string]string{}}
	entries, err := fs.ReadDir(staticFiles, "static")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		data, err := fs.ReadFile(staticFiles, "static/"+entry.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		v.static[entry.Name()] = data
		v.versions[entry.Name()] = hex.EncodeToString(sum[:6])
	}

	funcs := template.FuncMap{
		"asset": func(name string) (string, error) {
			version, ok := v.versions[name]
			if !ok {
				return "", fmt.Errorf("unknown asset %q", name)
			}
			return "/static/" + name + "?v=" + version, nil
		},
		"date":  formatDate,
		"ago":   ago,
		"bytes": formatBytes,
		"short": short,
		"deref": deref,
	}
	for _, name := range pageNames {
		page, err := template.New("layout").Funcs(funcs).ParseFS(templateFiles, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		v.pages[name] = page
	}
	return v, nil
}

// page is what every template receives.
type page struct {
	Title string
	// Nav is the active entry of the menu.
	Nav string
	// Admin and CSRF are set behind the login.
	Admin   string
	CSRF    string
	Version string
	Notice  string
	Error   string
	// Now is the time of the rendering, for the relative dates.
	Now  time.Time
	Data any
}

// render writes a page of the console.
func (c *Console) render(ctx fiber.Ctx, status int, name string, p page) error {
	if v, ok := viewerOf(ctx); ok {
		p.Admin, p.CSRF = v.Admin.Name, v.CSRF
	}
	p.Version = c.Config.Version
	p.Now = c.Now()
	var buf bytes.Buffer
	if err := c.views.pages[name].ExecuteTemplate(&buf, "layout", p); err != nil {
		return err
	}
	ctx.Status(status)
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return ctx.Send(buf.Bytes())
}

// static serves an embedded file. Its URL carries its version: it is cached
// for a year.
func (c *Console) static(ctx fiber.Ctx) error {
	name := ctx.Params("name")
	data, ok := c.views.static[name]
	if !ok {
		return fiber.ErrNotFound
	}
	ctx.Set(fiber.HeaderContentType, mime.TypeByExtension(path.Ext(name)))
	ctx.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	return ctx.Send(data)
}

// ErrorHandler renders the errors of the console as a page.
func (c *Console) ErrorHandler(ctx fiber.Ctx, err error) error {
	status, message := fiber.StatusInternalServerError, "Something went wrong. The error is in the logs of the server."
	if e, ok := err.(*fiber.Error); ok {
		status = e.Code
		switch status {
		case fiber.StatusNotFound:
			message = "This page does not exist."
		case fiber.StatusForbidden:
			message = e.Message
			if message == "" || message == "Forbidden" {
				message = "This request comes from another site: it is refused."
			}
		default:
			message = e.Message
		}
	} else {
		c.Logger.Error().Err(err).Str("event", "console.error").Str("path", ctx.Path()).Msg("Unexpected error")
	}
	return c.render(ctx, status, "error", page{Title: "Error", Error: message, Data: status})
}

const dateLayout = "2006-01-02 15:04 MST"

// formatDate shows a time, *time.Time or nil in UTC.
func formatDate(value any) string {
	switch t := value.(type) {
	case time.Time:
		if t.IsZero() {
			return "—"
		}
		return t.UTC().Format(dateLayout)
	case *time.Time:
		if t == nil {
			return "—"
		}
		return t.UTC().Format(dateLayout)
	}
	return "—"
}

// ago shows how long ago a time was, roughly.
func ago(now time.Time, value any) string {
	var t time.Time
	switch v := value.(type) {
	case time.Time:
		t = v
	case *time.Time:
		if v == nil {
			return "never"
		}
		t = *v
	}
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "in the future"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

// formatBytes shows a size in binary units.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// short shows the first group of a UUID; any other text as it is.
func short(id string) string {
	if len(id) == 36 && strings.Count(id, "-") == 4 {
		return id[:8]
	}
	return id
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
