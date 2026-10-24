// Package admin holds the administrators of the console and their sessions
// (PLAN.md, « Console »). An administrator is created by the CLI, with a
// password drawn by the server and shown once; only its Argon2id hash is
// kept, as is only the hash of a session token.
package admin

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// SessionTTL is how long a session of the console lasts.
const SessionTTL = 12 * time.Hour

// ErrInvalidName: a name is 1 to 32 characters among a-z, 0-9, ".", "_" and
// "-", starting with a letter or a digit.
var ErrInvalidName = errors.New("the name of an administrator is 1 to 32 characters: a-z, 0-9, '.', '_' or '-'")

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

type AdminApplication struct {
	Logger zerolog.Logger
	Admins domain.AdminPers
	Hasher *secrets.Hasher
	// Audit, when set, records the logins and the changes of administrators.
	Audit domain.AuditRecorder
	// Now is the clock, replaced in the tests.
	Now func() time.Time
}

func NewAdminApplication(logger zerolog.Logger, admins domain.AdminPers, hasher *secrets.Hasher) *AdminApplication {
	return &AdminApplication{
		Logger: logger,
		Admins: admins,
		Hasher: hasher,
		Now:    func() time.Time { return time.Now().UTC() },
	}
}

// Created is a new administrator, or a renewed one, and its password.
type Created struct {
	Admin    domain.Admin
	Password string
}

// Session is an open session of the console. Token goes in the cookie; the
// server keeps its hash.
type Session struct {
	Token     string
	Admin     domain.Admin
	ExpiresAt time.Time
}

// NormalizeName lowercases and trims a name.
func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Create adds an administrator with a drawn password. ErrInvalidName,
// ErrDuplicate: the name is taken.
func (c *AdminApplication) Create(ctx context.Context, name string) (*Created, error) {
	name = NormalizeName(name)
	if !namePattern.MatchString(name) {
		return nil, ErrInvalidName
	}
	password, hash, err := c.drawPassword(ctx)
	if err != nil {
		return nil, err
	}
	admin := domain.Admin{ID: uuid.NewString(), Name: name, PasswordHash: hash, CreatedAt: c.Now()}
	if err := c.Admins.Create(ctx, &admin); err != nil {
		return nil, err
	}
	c.Logger.Info().Str("component", "application.admin.create").Str("event", "admin.created").
		Str("admin_id", admin.ID).Msg("Administrator created")
	c.record(ctx, domain.AuditEvent{Action: domain.AuditAdminCreated, Target: name})
	return &Created{Admin: admin, Password: password}, nil
}

// ResetPassword draws a new password, and closes the sessions of the
// administrator. ErrNotFound: unknown name.
func (c *AdminApplication) ResetPassword(ctx context.Context, name string) (*Created, error) {
	admin, err := c.Admins.GetByName(ctx, NormalizeName(name))
	if err != nil {
		return nil, err
	}
	password, hash, err := c.drawPassword(ctx)
	if err != nil {
		return nil, err
	}
	admin.PasswordHash = hash
	if err := c.Admins.Update(ctx, admin, "password_hash"); err != nil {
		return nil, err
	}
	if err := c.Admins.DeleteSessions(ctx, admin.ID); err != nil {
		return nil, err
	}
	c.record(ctx, domain.AuditEvent{Action: domain.AuditAdminPasswordReset, Target: admin.Name})
	return &Created{Admin: *admin, Password: password}, nil
}

// List returns the administrators by name.
func (c *AdminApplication) List(ctx context.Context) ([]domain.Admin, error) {
	return c.Admins.List(ctx)
}

// Delete removes an administrator and closes its sessions. ErrNotFound:
// unknown name.
func (c *AdminApplication) Delete(ctx context.Context, name string) error {
	admin, err := c.Admins.GetByName(ctx, NormalizeName(name))
	if err != nil {
		return err
	}
	if err := c.Admins.Delete(ctx, admin.ID); err != nil {
		return err
	}
	c.record(ctx, domain.AuditEvent{Action: domain.AuditAdminDeleted, Target: admin.Name})
	return nil
}

// Login opens a session for an administrator who gives its password. An
// unknown name takes the time of a verification, and answers the same error
// as a wrong password: ErrInvalidCredentials.
func (c *AdminApplication) Login(ctx context.Context, name, password string) (*Session, error) {
	name = NormalizeName(name)
	admin, err := c.Admins.GetByName(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		if err := c.Hasher.VerifyDummy(ctx, []byte(password)); err != nil {
			return nil, err
		}
		// The name typed is not kept: it may be a password typed in the wrong field.
		c.record(ctx, domain.AuditEvent{Action: domain.AuditAdminLoginFailed, Actor: "admin", Detail: "unknown name"})
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	// A password that is not in base 32 is checked as typed: it fails, in the
	// same time.
	normalized, err := secrets.NormalizeCrockford(password)
	if err != nil {
		normalized = password
	}
	match, weak, err := c.Hasher.Verify(ctx, []byte(normalized), admin.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !match {
		c.record(ctx, domain.AuditEvent{Action: domain.AuditAdminLoginFailed, Actor: domain.ActorAdmin(admin.Name)})
		return nil, domain.ErrInvalidCredentials
	}

	now := c.Now()
	columns := []string{"last_login_at"}
	admin.LastLoginAt = &now
	if weak {
		if admin.PasswordHash, err = c.Hasher.Hash(ctx, []byte(normalized)); err != nil {
			return nil, err
		}
		columns = append(columns, "password_hash")
	}
	if err := c.Admins.Update(ctx, admin, columns...); err != nil {
		return nil, err
	}
	token, hash, err := secrets.NewToken()
	if err != nil {
		return nil, err
	}
	session := domain.AdminSession{Hash: hash, AdminID: admin.ID, CreatedAt: now, ExpiresAt: now.Add(SessionTTL)}
	if err := c.Admins.CreateSession(ctx, &session); err != nil {
		return nil, err
	}
	c.record(ctx, domain.AuditEvent{Action: domain.AuditAdminLogin, Actor: domain.ActorAdmin(admin.Name)})
	return &Session{Token: token, Admin: *admin, ExpiresAt: session.ExpiresAt}, nil
}

// Authenticate returns the administrator of a session token.
// ErrInvalidToken: unknown or expired session, or deleted administrator.
func (c *AdminApplication) Authenticate(ctx context.Context, token string) (*domain.Admin, error) {
	if token == "" {
		return nil, domain.ErrInvalidToken
	}
	session, err := c.Admins.GetSession(ctx, secrets.HashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if !c.Now().Before(session.ExpiresAt) {
		return nil, domain.ErrInvalidToken
	}
	admin, err := c.Admins.GetByID(ctx, session.AdminID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidToken
	}
	return admin, err
}

// Logout closes the session of a token.
func (c *AdminApplication) Logout(ctx context.Context, token string) error {
	if err := c.Admins.DeleteSession(ctx, secrets.HashToken(token)); err != nil {
		return err
	}
	c.record(ctx, domain.AuditEvent{Action: domain.AuditAdminLogout})
	return nil
}

// PurgeExpired removes the expired sessions.
func (c *AdminApplication) PurgeExpired(ctx context.Context) error {
	_, err := c.Admins.PurgeSessions(ctx, c.Now())
	return err
}

// drawPassword returns a new password and its hash. The hash is that of the
// normalized password, so that it matches however it is typed.
func (c *AdminApplication) drawPassword(ctx context.Context) (password, hash string, err error) {
	if password, err = secrets.NewAdminPassword(); err != nil {
		return "", "", err
	}
	normalized, err := secrets.NormalizeCrockford(password)
	if err != nil {
		return "", "", err
	}
	if hash, err = c.Hasher.Hash(ctx, []byte(normalized)); err != nil {
		return "", "", err
	}
	return password, hash, nil
}

// record writes an audit event, when the journal is set.
func (c *AdminApplication) record(ctx context.Context, event domain.AuditEvent) {
	if c.Audit != nil {
		c.Audit.Record(ctx, event)
	}
}
