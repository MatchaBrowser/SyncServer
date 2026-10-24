// Package audit holds the journal of the security events: logins, changes
// of password, revocations, invitations, actions of the administrators
// (PLAN.md, « Journal d'audit »). Never each sync, never an email, a key, a
// token or a code. The events are kept audit.retention.
package audit

import (
	"context"
	"net/netip"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/rs/zerolog"
)

// DefaultLimit and MaxLimit bound a page of events.
const (
	DefaultLimit = 100
	MaxLimit     = 500
)

// maxFieldLength bounds the texts of an event that come from a request.
const maxFieldLength = 200

type AuditApplication struct {
	Logger zerolog.Logger
	Events domain.AuditPers
	// Retention is how long an event is kept.
	Retention time.Duration
	// TruncateIP keeps only the /24 of an IPv4 address and the /48 of an IPv6
	// address.
	TruncateIP bool
	// Now is the clock, replaced in the tests.
	Now func() time.Time
}

func NewAuditApplication(logger zerolog.Logger, events domain.AuditPers, retention time.Duration, truncateIP bool) *AuditApplication {
	return &AuditApplication{
		Logger:     logger,
		Events:     events,
		Retention:  retention,
		TruncateIP: truncateIP,
		Now:        func() time.Time { return time.Now().UTC() },
	}
}

// Record writes an event, completed by the origin of ctx. It is called after
// the transaction of the use case; a failure is logged, never returned.
func (c *AuditApplication) Record(ctx context.Context, event domain.AuditEvent) {
	origin := domain.OriginOf(ctx)
	event.ID = 0
	event.CreatedAt = c.Now()
	if event.Actor == "" {
		event.Actor = origin.Actor
	}
	if event.AccountID == nil && origin.AccountID != "" {
		event.AccountID = &origin.AccountID
	}
	if event.DeviceID == nil && origin.DeviceID != "" {
		event.DeviceID = &origin.DeviceID
	}
	event.IP = c.ip(origin.IP)
	event.Agent = truncate(origin.Agent)
	event.Target = truncate(event.Target)
	event.Detail = truncate(event.Detail)

	// The event is written even when the client has gone in the meantime.
	if err := c.Events.Create(context.WithoutCancel(ctx), &event); err != nil {
		c.Logger.Error().Err(err).Str("component", "application.audit.record").Str("event", "audit.record.failed").
			Str("action", event.Action).Msg("Failed to record an audit event")
	}
}

// List returns a page of events, newest first.
func (c *AuditApplication) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, error) {
	switch {
	case filter.Limit <= 0:
		filter.Limit = DefaultLimit
	case filter.Limit > MaxLimit:
		filter.Limit = MaxLimit
	}
	return c.Events.List(ctx, filter)
}

// Count counts the events of a filter.
func (c *AuditApplication) Count(ctx context.Context, filter domain.AuditFilter) (int64, error) {
	return c.Events.Count(ctx, filter)
}

// Purge removes the events older than the retention.
func (c *AuditApplication) Purge(ctx context.Context) error {
	purged, err := c.Events.PurgeBefore(ctx, c.Now().Add(-c.Retention))
	if err == nil && purged > 0 {
		c.Logger.Info().Str("component", "application.audit.purge").Str("event", "audit.purged").
			Int64("events", purged).Msg("Old audit events purged")
	}
	return err
}

// ip returns the address to keep: whole, or its network with TruncateIP.
func (c *AuditApplication) ip(raw string) string {
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return truncate(raw)
	}
	addr = addr.Unmap()
	if !c.TruncateIP {
		return addr.String()
	}
	bits := 48
	if addr.Is4() {
		bits = 24
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.Addr().String()
}

func truncate(s string) string {
	if len(s) <= maxFieldLength {
		return s
	}
	// Cut on a rune boundary.
	for i := maxFieldLength; i > 0; i-- {
		if (s[i] & 0xC0) != 0x80 {
			return s[:i]
		}
	}
	return ""
}
