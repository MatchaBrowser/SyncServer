package config

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func GenericFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "config",
			Aliases:     []string{"c"},
			Value:       "config.yaml",
			Usage:       "Path to the configuration file",
			Destination: &cfg.ConfigFile,
			// The default file may be missing (flags and environment variables
			// only); a file given on the command line must exist, or every
			// setting would silently fall back to its default.
			Action: func(_ context.Context, _ *cli.Command, path string) error {
				return checkConfigFile(path)
			},
		},
	}
}

// checkConfigFile fails when the configuration file given by --config cannot
// be read.
func checkConfigFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("could not load config file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("could not load config file %s: it is a directory", path)
	}
	return nil
}
