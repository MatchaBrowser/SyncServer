package admin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/migration"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

// newConfigFile writes a configuration on a SQLite database, migrated when
// migrated is true.
func newConfigFile(t *testing.T, migrated bool) string {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "sync.sqlite")
	if migrated {
		var cfg config.Config
		cfg.Database.Dialect, cfg.Database.DSN = "sqlite", dsn
		db, err := database.Configure(cfg, zerolog.Nop())
		if err != nil {
			t.Fatal(err)
		}
		if err := migration.RunMigration(zerolog.Nop(), db.Db); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
	path := filepath.Join(dir, "config.yaml")
	yaml := "logger:\n  level: error\ndatabase:\n  dialect: sqlite\n  dsn: " + dsn + "\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// run runs the CLI as ./app does, and returns what it writes.
func run(args ...string) (stdout, stderr string, err error) {
	var out, errOut bytes.Buffer
	root := &cli.Command{Name: "app", Commands: []*cli.Command{NewInstance("test")}, Writer: &out, ErrWriter: &errOut}
	err = root.Run(context.Background(), append([]string{"app"}, args...))
	return out.String(), errOut.String(), err
}

var codePattern = regexp.MustCompile(`Invitation code: ([0-9A-Z]{4}-[0-9A-Z]{4})\nID: +([0-9a-f-]{36})`)

func TestInvitationCodes(t *testing.T) {
	cfg := newConfigFile(t, true)

	out, warning, err := run("admin", "invitation", "code", "create", "-c", cfg, "--note", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	first := codePattern.FindStringSubmatch(out)
	if first == nil || !strings.Contains(out, "Note:            Alice") || warning != "" {
		t.Fatalf("create: %q, warning %q", out, warning)
	}

	out, _, err = run("admin", "invitation", "code", "rotate", "-c", cfg, "--expires", "48h")
	if err != nil {
		t.Fatal(err)
	}
	second := codePattern.FindStringSubmatch(out)
	if second == nil || second[1] == first[1] || !strings.Contains(out, "Revoked codes:   1\n") {
		t.Fatalf("rotate: %q", out)
	}

	out, _, err = run("admin", "invitation", "code", "list", "-c", cfg)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "ID") ||
		!strings.Contains(lines[1], first[2]+"  revoked") || !strings.Contains(lines[1], "Alice") ||
		!strings.Contains(lines[2], second[2]+"  valid") {
		t.Errorf("list:\n%s", out)
	}
	if strings.Contains(out, first[1]) || strings.Contains(out, second[1]) {
		t.Error("list must not show codes: the server does not keep them")
	}

	if out, _, err = run("admin", "invitation", "code", "revoke", "-c", cfg, strings.ToUpper(second[2])); err != nil || !strings.Contains(out, "revoked") {
		t.Errorf("revoke: %q, %v", out, err)
	}
	if _, _, err = run("admin", "invitation", "code", "revoke", "-c", cfg, second[2]); err == nil {
		t.Error("revoking twice must fail")
	}
	if _, _, err = run("admin", "invitation", "code", "revoke", "-c", cfg); err == nil {
		t.Error("revoke without an id must fail")
	}
}

func TestInvitationCodes_Warnings(t *testing.T) {
	cfg := newConfigFile(t, true)
	_, warning, err := run("admin", "invitation", "code", "create", "-c", cfg, "--registration.mode", "open")
	if err != nil || !strings.Contains(warning, `registration.mode is "open"`) {
		t.Errorf("warning %q, %v", warning, err)
	}
	if _, _, err := run("admin", "invitation", "code", "create", "-c", cfg, "--expires", "0s"); err == nil {
		t.Error("a code without lifetime must be refused")
	}

	// A schema that is not up to date is refused, as by the server.
	_, _, err = run("admin", "invitation", "code", "create", "-c", newConfigFile(t, false))
	if !errors.Is(err, migration.ErrPendingMigrations) {
		t.Errorf("outdated schema: %v", err)
	}
}

var passwordPattern = regexp.MustCompile(`Password: +([0-9A-Z]{4}(-[0-9A-Z]{4}){4})\n`)

func TestUsers(t *testing.T) {
	cfg := newConfigFile(t, true)
	// Small Argon2id parameters: the cost is not what is tested.
	argon2 := []string{"--argon2.memory", "19", "--argon2.time", "2", "--argon2.threads", "1"}

	out, warning, err := run(append([]string{"admin", "user", "create", "-c", cfg, "Alice"}, argon2...)...)
	if err != nil {
		t.Fatal(err)
	}
	first := passwordPattern.FindStringSubmatch(out)
	if first == nil || !strings.Contains(out, "Administrator: alice") || !strings.Contains(warning, "admin.listen is empty") {
		t.Fatalf("create: %q, warning %q", out, warning)
	}
	if _, _, err := run(append([]string{"admin", "user", "create", "-c", cfg, "alice"}, argon2...)...); err == nil || !strings.Contains(err.Error(), "already named alice") {
		t.Errorf("same name: %v", err)
	}
	if _, warning, _ := run(append([]string{"admin", "user", "create", "-c", cfg, "bob", "--admin.listen", "127.0.0.1:9090"}, argon2...)...); warning != "" {
		t.Errorf("warning with a console: %q", warning)
	}

	out, _, err = run(append([]string{"admin", "user", "reset-password", "-c", cfg, "alice"}, argon2...)...)
	second := passwordPattern.FindStringSubmatch(out)
	if err != nil || second == nil || second[1] == first[1] {
		t.Errorf("reset-password: %q, %v", out, err)
	}
	if _, _, err := run(append([]string{"admin", "user", "reset-password", "-c", cfg, "zoe"}, argon2...)...); err == nil {
		t.Error("reset-password of an unknown name must fail")
	}

	out, _, err = run("admin", "user", "list", "-c", cfg)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != 3 || !strings.HasPrefix(lines[1], "alice ") || !strings.Contains(lines[1], "never") || !strings.HasPrefix(lines[2], "bob ") {
		t.Errorf("list: %q, %v", out, err)
	}
	if strings.Contains(out, second[1]) {
		t.Error("list must not show a password")
	}

	if out, _, err := run("admin", "user", "delete", "-c", cfg, "bob"); err != nil || !strings.Contains(out, "bob deleted") {
		t.Errorf("delete: %q, %v", out, err)
	}
	if _, _, err := run("admin", "user", "delete", "-c", cfg, "bob"); err == nil {
		t.Error("deleting twice must fail")
	}
	if _, _, err := run("admin", "user", "delete", "-c", cfg); err == nil {
		t.Error("delete without a name must fail")
	}
}

func TestAuditList(t *testing.T) {
	cfg := newConfigFile(t, true)
	out, _, err := run("admin", "audit", "list", "-c", cfg)
	if err != nil || strings.TrimSpace(out) != "No event." {
		t.Fatalf("empty journal: %q, %v", out, err)
	}

	// The actions of the CLI are recorded, with its actor.
	if _, _, err := run("admin", "invitation", "code", "create", "-c", cfg, "--note", "Alice"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("admin", "invitation", "code", "rotate", "-c", cfg); err != nil {
		t.Fatal(err)
	}
	out, _, err = run("admin", "audit", "list", "-c", cfg)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != 3 || !strings.Contains(lines[1], "invitation.rotated") || !strings.Contains(lines[2], "invitation.created") ||
		!strings.Contains(lines[2], " cli ") || !strings.Contains(lines[2], "Alice") {
		t.Fatalf("list:\n%s", out)
	}

	out, _, err = run("admin", "audit", "list", "-c", cfg, "--action", "invitation.created", "--limit", "5")
	if err != nil || strings.Count(out, "invitation.") != 1 {
		t.Errorf("filtered list:\n%s", out)
	}
}
