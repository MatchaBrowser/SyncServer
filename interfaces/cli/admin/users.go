package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	appadmin "github.com/matchabrowser/syncserver/application/admin"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"

	"github.com/urfave/cli/v3"
)

// usersCommand manages the administrators of the console (admin.listen).
func usersCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "user",
		Usage: "Manage the administrators of the console (admin.listen)",
		Commands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "Add an administrator, with a drawn password shown once",
				ArgsUsage: "<name>",
				Flags:     userFlags(cfg),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name, err := nameArg(cmd)
					if err != nil {
						return err
					}
					return withAdmins(ctx, *cfg, func(ctx context.Context, app *appadmin.AdminApplication) error {
						created, err := app.Create(ctx, name)
						if errors.Is(err, domain.ErrDuplicate) {
							return fmt.Errorf("an administrator is already named %s: reset-password draws a new password", appadmin.NormalizeName(name))
						}
						if err != nil {
							return err
						}
						warnConsoleDisabled(cmd.Root().ErrWriter, *cfg)
						printPassword(cmd.Root().Writer, created)
						return nil
					})
				},
			},
			{
				Name:      "reset-password",
				Usage:     "Draw a new password for an administrator, and close its sessions",
				ArgsUsage: "<name>",
				Flags:     userFlags(cfg),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name, err := nameArg(cmd)
					if err != nil {
						return err
					}
					return withAdmins(ctx, *cfg, func(ctx context.Context, app *appadmin.AdminApplication) error {
						created, err := app.ResetPassword(ctx, name)
						if errors.Is(err, domain.ErrNotFound) {
							return fmt.Errorf("no administrator named %s", appadmin.NormalizeName(name))
						}
						if err != nil {
							return err
						}
						printPassword(cmd.Root().Writer, created)
						return nil
					})
				},
			},
			{
				Name:  "list",
				Usage: "List the administrators",
				Flags: userFlags(cfg),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return withAdmins(ctx, *cfg, func(ctx context.Context, app *appadmin.AdminApplication) error {
						admins, err := app.List(ctx)
						if err != nil {
							return err
						}
						out := cmd.Root().Writer
						if len(admins) == 0 {
							fmt.Fprintln(out, "No administrator.")
							return nil
						}
						w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
						fmt.Fprintln(w, "NAME\tCREATED\tLAST LOGIN")
						for _, a := range admins {
							lastLogin := "never"
							if a.LastLoginAt != nil {
								lastLogin = formatDate(*a.LastLoginAt)
							}
							fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, formatDate(a.CreatedAt), lastLogin)
						}
						return w.Flush()
					})
				},
			},
			{
				Name:      "delete",
				Usage:     "Remove an administrator and close its sessions",
				ArgsUsage: "<name>",
				Flags:     userFlags(cfg),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name, err := nameArg(cmd)
					if err != nil {
						return err
					}
					return withAdmins(ctx, *cfg, func(ctx context.Context, app *appadmin.AdminApplication) error {
						err := app.Delete(ctx, name)
						if errors.Is(err, domain.ErrNotFound) {
							return fmt.Errorf("no administrator named %s", appadmin.NormalizeName(name))
						}
						if err != nil {
							return err
						}
						fmt.Fprintf(cmd.Root().Writer, "Administrator %s deleted.\n", appadmin.NormalizeName(name))
						return nil
					})
				},
			},
		},
	}
}

// userFlags add the Argon2id parameters, which hash the passwords, and the
// address of the console, to warn when it is disabled.
func userFlags(cfg *config.Config) []cli.Flag {
	list := getFlags(cfg)
	list = append(list, config.Argon2Flags(cfg)...)
	list = append(list, config.AdminFlags(cfg)...)
	return list
}

func nameArg(cmd *cli.Command) (string, error) {
	name := cmd.Args().First()
	if name == "" {
		return "", errors.New("give the name of the administrator")
	}
	return name, nil
}

// withAdmins runs fn on the administrators.
func withAdmins(ctx context.Context, cfg config.Config, fn func(ctx context.Context, app *appadmin.AdminApplication) error) error {
	hasher, err := secrets.NewHasher(cfg.Argon2.Memory, cfg.Argon2.Time, cfg.Argon2.Threads, cfg.Argon2.MaxConcurrent)
	if err != nil {
		return err
	}
	return withDatabase(ctx, cfg, func(ctx context.Context, env environment) error {
		app := appadmin.NewAdminApplication(env.log, persistence.NewAdminPers(env.db), hasher)
		app.Audit = env.audit
		return fn(ctx, app)
	})
}

// printPassword shows a password. It is the only time: the server keeps its
// hash only.
func printPassword(out io.Writer, created *appadmin.Created) {
	fmt.Fprintf(out, "Administrator: %s\n", created.Admin.Name)
	fmt.Fprintf(out, "Password:      %s\n", created.Password)
	fmt.Fprintln(out, "\nThe password is shown only once: the server keeps only its hash.")
}

// warnConsoleDisabled tells when the console does not listen.
func warnConsoleDisabled(out io.Writer, cfg config.Config) {
	if cfg.Admin.Listen == "" {
		fmt.Fprintln(out, "Warning: admin.listen is empty, the console is disabled. Set it (e.g. 127.0.0.1:9090) and restart the server.")
	}
}
