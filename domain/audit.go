package domain

import (
	"context"
	"time"
)

// AuditEvent is an entry of the security journal: who did what, and from
// where. It never holds an email, a key, a token, a code or a payload: the
// account appears by its id. It is kept audit.retention (PLAN.md, « Journal
// d'audit »).
type AuditEvent struct {
	ID        int64
	CreatedAt time.Time
	Action    string
	// Actor is who acted: client, cli, system or admin:<name>.
	Actor     string
	AccountID *string
	// DeviceID is the device that acted, for a client.
	DeviceID *string
	// Target is what the action is about, besides the account: a revoked
	// device, an invitation, an administrator.
	Target string
	IP     string
	// Agent is the Matcha-Client header of a client, or the User-Agent of a
	// browser.
	Agent string
	// Detail is a short reason, such as the error code of a refusal.
	Detail string
}

func (AuditEvent) TableName() string {
	return "audit_events"
}

// Actions of the audit journal. Only the events that matter for security are
// recorded, never each sync: the journal would otherwise follow the activity
// of every user.
const (
	AuditAccountCreated      = "account.created"
	AuditAccountCreateFailed = "account.create_failed"
	AuditLogin               = "session.login"
	AuditLoginFailed         = "session.login_failed"
	AuditRefreshReused       = "session.refresh_reused"
	AuditRecoveryStarted     = "account.recovery_started"
	AuditRecoveryFailed      = "account.recovery_failed"
	AuditRecovered           = "account.recovered"
	AuditPasswordChanged     = "account.password_changed"
	AuditRecoveryChanged     = "account.recovery_changed"
	AuditEmailChanged        = "account.email_changed"
	AuditReauthFailed        = "account.reauth_failed"
	AuditAccountDeleted      = "account.deleted"
	AuditDeviceRevoked       = "device.revoked"
	AuditRateLimited         = "rate_limited"
	AuditInvitationCreated   = "invitation.created"
	AuditInvitationRevoked   = "invitation.revoked"
	AuditInvitationsRotated  = "invitation.rotated"
	AuditAdminLogin          = "admin.login"
	AuditAdminLoginFailed    = "admin.login_failed"
	AuditAdminLogout         = "admin.logout"
	AuditAdminCreated        = "admin.created"
	AuditAdminDeleted        = "admin.deleted"
	AuditAdminPasswordReset  = "admin.password_reset"
)

// AuditActions lists every action, for the filters of the console.
var AuditActions = []string{
	AuditAccountCreated, AuditAccountCreateFailed, AuditLogin, AuditLoginFailed,
	AuditRefreshReused, AuditRecoveryStarted, AuditRecoveryFailed, AuditRecovered,
	AuditPasswordChanged, AuditRecoveryChanged, AuditEmailChanged, AuditReauthFailed,
	AuditAccountDeleted, AuditDeviceRevoked, AuditRateLimited,
	AuditInvitationCreated, AuditInvitationRevoked, AuditInvitationsRotated,
	AuditAdminLogin, AuditAdminLoginFailed, AuditAdminLogout,
	AuditAdminCreated, AuditAdminDeleted, AuditAdminPasswordReset,
}

// AuditFilter selects events, newest first. Empty fields do not filter.
type AuditFilter struct {
	Action    string
	AccountID string
	IP        string
	Target    string
	// Before keeps the events older than this id: the next page.
	Before int64
	// Since keeps the events at or after this time.
	Since time.Time
	Limit int
}

type AuditPers interface {
	Create(ctx context.Context, event *AuditEvent) error
	// List returns the events of the filter, newest first.
	List(ctx context.Context, filter AuditFilter) ([]AuditEvent, error)
	// Count counts the events of the filter (Before and Limit aside).
	Count(ctx context.Context, filter AuditFilter) (int64, error)
	// PurgeBefore removes the events older than a date.
	PurgeBefore(ctx context.Context, before time.Time) (int64, error)
}

// AuditRecorder records an event. The origin of the context completes it:
// actor, IP, agent, and the account and device of an authenticated client.
// A failure is logged, never returned: the journal does not fail a request.
type AuditRecorder interface {
	Record(ctx context.Context, event AuditEvent)
}

// Actors of the audit journal; an administrator is admin:<name>.
const (
	ActorClient = "client"
	ActorCLI    = "cli"
	ActorSystem = "system"
)

// ActorAdmin is the actor of an administrator of the console.
func ActorAdmin(name string) string {
	return "admin:" + name
}

// Origin tells where a request comes from. It travels in the context, so
// that the use cases record it without knowing the interface that called
// them.
type Origin struct {
	Actor string
	IP    string
	Agent string
	// AccountID and DeviceID are those of an authenticated client.
	AccountID string
	DeviceID  string
}

type originKey struct{}

// WithOrigin returns ctx carrying origin.
func WithOrigin(ctx context.Context, origin Origin) context.Context {
	return context.WithValue(ctx, originKey{}, origin)
}

// OriginOf returns the origin of ctx; the system when there is none.
func OriginOf(ctx context.Context) Origin {
	if origin, ok := ctx.Value(originKey{}).(Origin); ok {
		return origin
	}
	return Origin{Actor: ActorSystem}
}
