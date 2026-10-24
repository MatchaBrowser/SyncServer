package admin

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/matchabrowser/syncserver/application/audit"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"

	"github.com/urfave/cli/v3"
)

// auditCommand reads the audit journal.
func auditCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "audit",
		Usage: "Read the journal of the security events",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List the latest events, newest first",
				Flags: append(getFlags(cfg),
					&cli.StringFlag{Name: "action", Usage: "Keep one kind of event (e.g. session.login_failed)"},
					&cli.StringFlag{Name: "account", Usage: "Keep the events of an account id"},
					&cli.StringFlag{Name: "ip", Usage: "Keep the events of an IP address"},
					&cli.StringFlag{Name: "target", Usage: "Keep the events about a device, an invitation or an administrator"},
					&cli.IntFlag{Name: "limit", Usage: "Most events shown", Value: 50},
				),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					filter := domain.AuditFilter{
						Action:    cmd.String("action"),
						AccountID: strings.ToLower(cmd.String("account")),
						IP:        cmd.String("ip"),
						Target:    cmd.String("target"),
						Limit:     min(max(cmd.Int("limit"), 1), audit.MaxLimit),
					}
					return withDatabase(ctx, *cfg, func(ctx context.Context, env environment) error {
						events, err := env.audit.List(ctx, filter)
						if err != nil {
							return err
						}
						out := cmd.Root().Writer
						if len(events) == 0 {
							fmt.Fprintln(out, "No event.")
							return nil
						}
						w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
						fmt.Fprintln(w, "TIME\tEVENT\tACTOR\tACCOUNT\tDEVICE\tTARGET\tIP\tDETAIL")
						for _, e := range events {
							fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", formatDate(e.CreatedAt), e.Action, e.Actor,
								orDash(e.AccountID), orDash(e.DeviceID), dash(e.Target), dash(e.IP), dash(e.Detail))
						}
						return w.Flush()
					})
				},
			},
		},
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orDash(s *string) string {
	if s == nil {
		return "-"
	}
	return dash(*s)
}
