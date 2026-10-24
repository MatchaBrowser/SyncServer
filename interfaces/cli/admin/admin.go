// Package admin holds the administration commands, run next to the server
// with its configuration: the invitation codes (SYNC.md § 4.9), the
// administrators of the console and the audit journal. Their actions are
// recorded in the journal, with the cli actor.
package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/user"
	"text/tabwriter"
	"time"

	"github.com/matchabrowser/syncserver/application/audit"
	"github.com/matchabrowser/syncserver/application/invitations"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/logger"
	"github.com/matchabrowser/syncserver/infrastructure/migration"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
	"gorm.io/gorm"
)

// dateLayout is how the dates of the invitations are shown.
const dateLayout = "2006-01-02 15:04 MST"

// NewInstance creates the "admin" command. Each leaf command takes the flags
// of the configuration, after its name, as the server does: a flag of a
// parent command would read the configuration file before -c is parsed.
func NewInstance(version string) *cli.Command {
	cfg := &config.Config{}
	cfg.Version = version

	return &cli.Command{
		Name:  "admin",
		Usage: "Administer the server",
		Commands: []*cli.Command{
			{
				Name:  "invitation",
				Usage: "Manage the invitations, asked when registration.mode is invitation",
				Commands: []*cli.Command{
					{
						Name:  "code",
						Usage: "Manage the invitation codes",
						Commands: []*cli.Command{
							createCommand(cfg),
							rotateCommand(cfg),
							listCommand(cfg),
							revokeCommand(cfg),
						},
					},
				},
			},
			usersCommand(cfg),
			auditCommand(cfg),
		},
	}
}

// getFlags returns the list of CLI flags required for the admin commands.
func getFlags(cfg *config.Config) (list []cli.Flag) {
	list = append(list, config.GenericFlags(cfg)...)
	list = append(list, config.LoggerFlags(cfg)...)
	list = append(list, config.DatabaseFlags(cfg)...)
	list = append(list, config.RegistrationFlags(cfg)...)
	return
}

// codeFlags are the options of a new code.
func codeFlags(cfg *config.Config) []cli.Flag {
	return append(getFlags(cfg),
		&cli.DurationFlag{
			Name:  "expires",
			Usage: "Lifetime of the code",
			Value: invitations.DefaultTTL,
		},
		&cli.StringFlag{
			Name:  "note",
			Usage: "Who the code is for, shown by list",
		},
	)
}

func createCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Draw a new invitation code, valid once",
		Flags: codeFlags(cfg),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return withApplication(ctx, *cfg, func(ctx context.Context, app *invitations.InvitationsApplication) error {
				created, err := app.Create(ctx, cmd.String("note"), cmd.Duration("expires"))
				if err != nil {
					return err
				}
				warnRegistrationMode(cmd.Root().ErrWriter, *cfg)
				printCreated(cmd.Root().Writer, created)
				return nil
			})
		},
	}
}

func rotateCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "rotate",
		Usage: "Revoke every code still valid, and draw a new one",
		Flags: codeFlags(cfg),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return withApplication(ctx, *cfg, func(ctx context.Context, app *invitations.InvitationsApplication) error {
				created, revoked, err := app.Rotate(ctx, cmd.String("note"), cmd.Duration("expires"))
				if err != nil {
					return err
				}
				warnRegistrationMode(cmd.Root().ErrWriter, *cfg)
				fmt.Fprintf(cmd.Root().Writer, "Revoked codes:   %d\n", revoked)
				printCreated(cmd.Root().Writer, created)
				return nil
			})
		},
	}
}

func listCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List the invitations and their status (the codes are not kept)",
		Flags: getFlags(cfg),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return withApplication(ctx, *cfg, func(ctx context.Context, app *invitations.InvitationsApplication) error {
				list, err := app.List(ctx)
				if err != nil {
					return err
				}
				out := cmd.Root().Writer
				if len(list) == 0 {
					fmt.Fprintln(out, "No invitation.")
					return nil
				}
				now := app.Now()
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tSTATUS\tCREATED\tEXPIRES\tNOTE")
				for _, i := range list {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", i.ID, i.Status(now), formatDate(i.CreatedAt), formatDate(i.ExpiresAt), i.Note)
				}
				return w.Flush()
			})
		},
	}
}

func revokeCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:      "revoke",
		Usage:     "Revoke an invitation that is not used yet",
		ArgsUsage: "<id>",
		Flags:     getFlags(cfg),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			id := cmd.Args().First()
			if id == "" {
				return errors.New("give the id of the invitation, as shown by list")
			}
			return withApplication(ctx, *cfg, func(ctx context.Context, app *invitations.InvitationsApplication) error {
				err := app.Revoke(ctx, id)
				if errors.Is(err, domain.ErrNotFound) {
					return fmt.Errorf("no invitation %s to revoke: unknown, used or already revoked", id)
				}
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.Root().Writer, "Invitation %s revoked.\n", id)
				return nil
			})
		},
	}
}

// withApplication opens the database, checks that its schema is up to date,
// and runs fn on the invitations.
func withApplication(ctx context.Context, cfg config.Config, fn func(ctx context.Context, app *invitations.InvitationsApplication) error) error {
	return withDatabase(ctx, cfg, func(ctx context.Context, env environment) error {
		app := invitations.NewInvitationsApplication(env.log, persistence.NewTransactor(env.db), persistence.NewInvitationPers(env.db))
		app.Audit = env.audit
		return fn(ctx, app)
	})
}

// environment is what a command gets: the database, the logger and the
// audit journal.
type environment struct {
	db    *gorm.DB
	log   zerolog.Logger
	audit *audit.AuditApplication
}

// withDatabase opens the database, checks that its schema is up to date, and
// runs fn with a context whose origin is the CLI and the user of the system.
func withDatabase(ctx context.Context, cfg config.Config, fn func(ctx context.Context, env environment) error) error {
	log := logger.NewLogger(cfg.Logger.Level, cfg.Logger.Pretty, cfg.Version)
	db, err := database.Configure(cfg, log)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := migration.CheckUpToDate(log, db.Db); err != nil {
		return err
	}
	agent := "syncserver " + cfg.Version
	if u, err := user.Current(); err == nil {
		agent += " (" + u.Username + ")"
	}
	ctx = domain.WithOrigin(ctx, domain.Origin{Actor: domain.ActorCLI, Agent: agent})
	env := environment{db: db.Db, log: log}
	// The retention matters only to the purge, done by the server.
	env.audit = audit.NewAuditApplication(log, persistence.NewAuditPers(db.Db), config.DefaultAuditRetention, cfg.Audit.TruncateIP)
	return fn(ctx, env)
}

// printCreated shows a new code. It is the only time: the server keeps its
// hash only.
func printCreated(out io.Writer, created *invitations.Created) {
	fmt.Fprintf(out, "Invitation code: %s\n", created.Code)
	fmt.Fprintf(out, "ID:              %s\n", created.Invitation.ID)
	fmt.Fprintf(out, "Expires:         %s\n", formatDate(created.Invitation.ExpiresAt))
	if created.Invitation.Note != "" {
		fmt.Fprintf(out, "Note:            %s\n", created.Invitation.Note)
	}
	fmt.Fprintln(out, "\nThe code is shown only once: the server keeps only its hash. It opens one account.")
}

// warnRegistrationMode tells when the codes are not asked at registration.
func warnRegistrationMode(out io.Writer, cfg config.Config) {
	if cfg.Registration.Mode != config.RegistrationInvitation {
		fmt.Fprintf(out, "Warning: registration.mode is %q, the codes are asked only when it is %q.\n", cfg.Registration.Mode, config.RegistrationInvitation)
	}
}

func formatDate(t time.Time) string {
	return t.UTC().Format(dateLayout)
}
