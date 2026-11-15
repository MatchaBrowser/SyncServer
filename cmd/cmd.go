package main

import (
	"context"
	"log"
	"os"

	"github.com/matchabrowser/syncserver/interfaces/cli/admin"
	genoapi "github.com/matchabrowser/syncserver/interfaces/cli/gen-oapi"
	"github.com/matchabrowser/syncserver/interfaces/cli/migration"
	"github.com/matchabrowser/syncserver/interfaces/cli/server"

	"github.com/urfave/cli/v3"
)

var version = "development"

// main is the entry point of the application.
// It sets up the CLI commands. Each command reads its configuration file
// (--config, config.GenericFlags) through the YAML source of its flags.
func main() {
	cmd := &cli.Command{
		Name:    "SyncServer CLI",
		Version: version,
		Usage:   "SyncServer CLI - Manage SyncServer server and migrations",
		Commands: []*cli.Command{
			server.NewInstance(version),
			migration.NewInstance(version),
			genoapi.NewInstance(version),
			admin.NewInstance(version),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatalf("Error running command: %v", err)
	}
}
