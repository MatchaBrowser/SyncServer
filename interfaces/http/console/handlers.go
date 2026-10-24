package console

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/application/invitations"
	"github.com/matchabrowser/syncserver/domain"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// accountsPerPage is the size of a page of the account list.
const accountsPerPage = 50

// notices are the messages shown after an action, by the key of ?notice=.
var notices = map[string]string{
	"logged-out":         "You are logged out.",
	"device-revoked":     "Device revoked: its tokens no longer work, and it must log in again.",
	"device-gone":        "This device was already revoked.",
	"account-deleted":    "Account deleted, with its devices, records, packages and avatar.",
	"invitation-revoked": "Invitation revoked.",
	"invitation-gone":    "This invitation was already used or revoked.",
}

// lifetimes are the choices of lifetime of a new invitation.
var lifetimes = []struct {
	Value string
	Label string
	TTL   time.Duration
}{
	{"24h", "1 day", 24 * time.Hour},
	{"168h", "7 days", invitations.DefaultTTL},
	{"720h", "30 days", 30 * 24 * time.Hour},
}

func (c *Console) loginPage(ctx fiber.Ctx) error {
	if _, err := c.Admins.Authenticate(ctx.Context(), ctx.Cookies(cookieName)); err == nil {
		return redirect(ctx, "/")
	}
	return c.render(ctx, fiber.StatusOK, "login", page{Title: "Log in", Notice: notices[ctx.Query("notice")]})
}

func (c *Console) login(ctx fiber.Ctx) error {
	if ok, _ := c.loginLimit.Allow(ctx.IP()); !ok {
		return c.render(ctx, fiber.StatusTooManyRequests, "login", page{Title: "Log in", Error: "Too many attempts: try again in a quarter of an hour."})
	}
	session, err := c.Admins.Login(ctx.Context(), ctx.FormValue("name"), ctx.FormValue("password"))
	if errors.Is(err, domain.ErrInvalidCredentials) {
		return c.render(ctx, fiber.StatusUnauthorized, "login", page{Title: "Log in", Error: "Wrong name or password."})
	}
	if err != nil {
		return err
	}
	setSessionCookie(ctx, session.Token, session.ExpiresAt)
	return redirect(ctx, "/")
}

func (c *Console) logout(ctx fiber.Ctx) error {
	if err := c.Admins.Logout(ctx.Context(), ctx.Cookies(cookieName)); err != nil {
		return err
	}
	clearSessionCookie(ctx)
	return redirect(ctx, "/login?notice=logged-out")
}

// eventRow is an audit event, with the email and device name it refers to
// when they still exist.
type eventRow struct {
	domain.AuditEvent
	Email      string
	DeviceName string
	// Severity is "", "warn" or "danger".
	Severity string
}

// severities mark the events worth a look.
var severities = map[string]string{
	domain.AuditLoginFailed:         "warn",
	domain.AuditRecoveryFailed:      "warn",
	domain.AuditAccountCreateFailed: "warn",
	domain.AuditRateLimited:         "warn",
	domain.AuditAdminLoginFailed:    "danger",
	domain.AuditRefreshReused:       "danger",
	domain.AuditReauthFailed:        "danger",
}

// eventRows names the accounts and devices of events.
func (c *Console) eventRows(ctx fiber.Ctx, events []domain.AuditEvent) ([]eventRow, error) {
	seen := map[string]bool{}
	var ids []string
	for _, e := range events {
		if e.AccountID != nil && !seen[*e.AccountID] {
			seen[*e.AccountID] = true
			ids = append(ids, *e.AccountID)
		}
	}
	labels, err := c.Accounts.Labels(ctx.Context(), ids)
	if err != nil {
		return nil, err
	}
	rows := make([]eventRow, len(events))
	for i, e := range events {
		rows[i] = eventRow{AuditEvent: e, Severity: severities[e.Action]}
		if e.AccountID != nil {
			rows[i].Email = labels.Emails[*e.AccountID]
			if e.DeviceID != nil {
				rows[i].DeviceName = labels.Devices[account.DeviceKey(*e.AccountID, *e.DeviceID)]
			}
		}
	}
	return rows, nil
}

type dashboardData struct {
	Overview           account.Overview
	ValidInvitations   int
	Registration       string
	FailedLogins       int64
	RateLimited        int64
	AdminLoginsFailed  int64
	ReusedRefresh      int64
	AuditRetentionDays int
	Events             []eventRow
}

func (c *Console) dashboard(ctx fiber.Ctx) error {
	data := dashboardData{
		Registration:       c.Config.Registration.Mode,
		AuditRetentionDays: int(c.Config.Audit.Retention.Hours() / 24),
	}
	var err error
	if data.Overview, err = c.Accounts.Overview(ctx.Context()); err != nil {
		return err
	}
	list, err := c.Invitations.List(ctx.Context())
	if err != nil {
		return err
	}
	now := c.Now()
	for _, i := range list {
		if i.Status(now) == domain.InvitationValid {
			data.ValidInvitations++
		}
	}
	since := now.Add(-24 * time.Hour)
	for action, count := range map[string]*int64{
		domain.AuditLoginFailed:      &data.FailedLogins,
		domain.AuditRateLimited:      &data.RateLimited,
		domain.AuditAdminLoginFailed: &data.AdminLoginsFailed,
		domain.AuditRefreshReused:    &data.ReusedRefresh,
	} {
		if *count, err = c.Audit.Count(ctx.Context(), domain.AuditFilter{Action: action, Since: since}); err != nil {
			return err
		}
	}
	events, err := c.Audit.List(ctx.Context(), domain.AuditFilter{Limit: 15})
	if err != nil {
		return err
	}
	if data.Events, err = c.eventRows(ctx, events); err != nil {
		return err
	}
	return c.render(ctx, fiber.StatusOK, "dashboard", page{Title: "Dashboard", Nav: "dashboard", Data: data})
}

type accountsData struct {
	Search   string
	Page     *account.AccountPage
	PageNum  int
	Previous string
	Next     string
}

func (c *Console) accounts(ctx fiber.Ctx) error {
	search := strings.TrimSpace(ctx.Query("q"))
	num, _ := strconv.Atoi(ctx.Query("page"))
	num = max(num, 1)
	result, err := c.Accounts.ListAccounts(ctx.Context(), search, (num-1)*accountsPerPage, accountsPerPage)
	if err != nil {
		return err
	}
	data := accountsData{Search: search, Page: result, PageNum: num}
	pageURL := func(n int) string {
		q := url.Values{"page": {strconv.Itoa(n)}}
		if search != "" {
			q.Set("q", search)
		}
		return "/accounts?" + q.Encode()
	}
	if num > 1 {
		data.Previous = pageURL(num - 1)
	}
	if int64(num*accountsPerPage) < result.Total {
		data.Next = pageURL(num + 1)
	}
	return c.render(ctx, fiber.StatusOK, "accounts", page{Title: "Accounts", Nav: "accounts", Notice: notices[ctx.Query("notice")], Data: data})
}

type accountData struct {
	Account *account.AccountDetails
	Events  []eventRow
}

// accountID reads the :id of a route; ErrNotFound when it is not a UUID.
func accountID(ctx fiber.Ctx, param string) (string, error) {
	id, err := uuid.Parse(ctx.Params(param))
	if err != nil {
		return "", fiber.ErrNotFound
	}
	return id.String(), nil
}

func (c *Console) account(ctx fiber.Ctx) error {
	return c.renderAccount(ctx, fiber.StatusOK, notices[ctx.Query("notice")], "")
}

func (c *Console) renderAccount(ctx fiber.Ctx, status int, notice, problem string) error {
	id, err := accountID(ctx, "id")
	if err != nil {
		return err
	}
	details, err := c.Accounts.Describe(ctx.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		return fiber.ErrNotFound
	}
	if err != nil {
		return err
	}
	events, err := c.Audit.List(ctx.Context(), domain.AuditFilter{AccountID: id, Limit: 30})
	if err != nil {
		return err
	}
	rows, err := c.eventRows(ctx, events)
	if err != nil {
		return err
	}
	return c.render(ctx, status, "account", page{Title: details.Email, Nav: "accounts", Notice: notice, Error: problem, Data: accountData{Account: details, Events: rows}})
}

func (c *Console) revokeDevice(ctx fiber.Ctx) error {
	id, err := accountID(ctx, "id")
	if err != nil {
		return err
	}
	deviceID, err := accountID(ctx, "device")
	if err != nil {
		return err
	}
	notice := "device-revoked"
	err = c.Accounts.RevokeDevice(ctx.Context(), id, deviceID)
	if errors.Is(err, domain.ErrNotFound) {
		notice = "device-gone"
	} else if err != nil {
		return err
	}
	return redirect(ctx, "/accounts/"+id+"?notice="+notice)
}

func (c *Console) deleteAccount(ctx fiber.Ctx) error {
	id, err := accountID(ctx, "id")
	if err != nil {
		return err
	}
	details, err := c.Accounts.Describe(ctx.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		return fiber.ErrNotFound
	}
	if err != nil {
		return err
	}
	if account.NormalizeEmail(ctx.FormValue("confirm")) != details.Email {
		return c.renderAccount(ctx, fiber.StatusBadRequest, "", "Type the email of the account to delete it.")
	}
	if err := c.Accounts.RemoveAccount(ctx.Context(), id); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return redirect(ctx, "/accounts?notice=account-deleted")
}

// invitationRow is an invitation and its status.
type invitationRow struct {
	domain.Invitation
	Status string
}

type invitationsData struct {
	Invitations  []invitationRow
	Lifetimes    any
	Created      *invitations.Created
	Revoked      int64
	Rotated      bool
	Registration string
}

func (c *Console) invitations(ctx fiber.Ctx) error {
	return c.renderInvitations(ctx, fiber.StatusOK, invitationsData{}, notices[ctx.Query("notice")], "")
}

func (c *Console) renderInvitations(ctx fiber.Ctx, status int, data invitationsData, notice, problem string) error {
	list, err := c.Invitations.List(ctx.Context())
	if err != nil {
		return err
	}
	now := c.Now()
	// Newest first.
	data.Invitations = make([]invitationRow, len(list))
	for i, inv := range list {
		data.Invitations[len(list)-1-i] = invitationRow{Invitation: inv, Status: inv.Status(now)}
	}
	data.Lifetimes = lifetimes
	data.Registration = c.Config.Registration.Mode
	return c.render(ctx, status, "invitations", page{Title: "Invitations", Nav: "invitations", Notice: notice, Error: problem, Data: data})
}

// lifetime reads the lifetime chosen in a form.
func lifetime(ctx fiber.Ctx) (time.Duration, bool) {
	value := ctx.FormValue("expires")
	for _, l := range lifetimes {
		if l.Value == value {
			return l.TTL, true
		}
	}
	return 0, false
}

// The new code is shown in the response, never kept: the page is rendered
// after the POST, not redirected.

func (c *Console) createInvitation(ctx fiber.Ctx) error {
	ttl, ok := lifetime(ctx)
	if !ok {
		return c.renderInvitations(ctx, fiber.StatusBadRequest, invitationsData{}, "", "Choose a lifetime.")
	}
	created, err := c.Invitations.Create(ctx.Context(), ctx.FormValue("note"), ttl)
	if err != nil {
		return err
	}
	return c.renderInvitations(ctx, fiber.StatusOK, invitationsData{Created: created}, "", "")
}

func (c *Console) rotateInvitations(ctx fiber.Ctx) error {
	ttl, ok := lifetime(ctx)
	if !ok {
		return c.renderInvitations(ctx, fiber.StatusBadRequest, invitationsData{}, "", "Choose a lifetime.")
	}
	created, revoked, err := c.Invitations.Rotate(ctx.Context(), ctx.FormValue("note"), ttl)
	if err != nil {
		return err
	}
	return c.renderInvitations(ctx, fiber.StatusOK, invitationsData{Created: created, Revoked: revoked, Rotated: true}, "", "")
}

func (c *Console) revokeInvitation(ctx fiber.Ctx) error {
	notice := "invitation-revoked"
	err := c.Invitations.Revoke(ctx.Context(), ctx.Params("id"))
	if errors.Is(err, domain.ErrNotFound) {
		notice = "invitation-gone"
	} else if err != nil {
		return err
	}
	return redirect(ctx, "/invitations?notice="+notice)
}

type auditData struct {
	Filter  domain.AuditFilter
	Actions []string
	Events  []eventRow
	// Next is the URL of the older events; empty on the last page.
	Next string
}

func (c *Console) audit(ctx fiber.Ctx) error {
	filter := domain.AuditFilter{
		Action:    strings.TrimSpace(ctx.Query("action")),
		AccountID: strings.ToLower(strings.TrimSpace(ctx.Query("account"))),
		IP:        strings.TrimSpace(ctx.Query("ip")),
		Target:    strings.TrimSpace(ctx.Query("target")),
	}
	filter.Before, _ = strconv.ParseInt(ctx.Query("before"), 10, 64)
	filter.Limit = 100
	events, err := c.Audit.List(ctx.Context(), filter)
	if err != nil {
		return err
	}
	rows, err := c.eventRows(ctx, events)
	if err != nil {
		return err
	}
	data := auditData{Filter: filter, Actions: domain.AuditActions, Events: rows}
	if len(events) == filter.Limit {
		q := url.Values{"before": {strconv.FormatInt(events[len(events)-1].ID, 10)}}
		for key, value := range map[string]string{"action": filter.Action, "account": filter.AccountID, "ip": filter.IP, "target": filter.Target} {
			if value != "" {
				q.Set(key, value)
			}
		}
		data.Next = "/audit?" + q.Encode()
	}
	return c.render(ctx, fiber.StatusOK, "audit", page{Title: "Audit", Nav: "audit", Data: data})
}
