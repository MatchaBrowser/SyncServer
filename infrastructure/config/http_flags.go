package config

import (
	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

func ServerFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "http.port",
			Aliases:     []string{"p"},
			Value:       8080,
			Destination: &cfg.Server.Port,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("HTTP_PORT"),
				altsrcyaml.YAML("http.port", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.BoolFlag{
			Name:        "http.logs",
			Value:       false,
			Destination: &cfg.Server.HttpLogs,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("HTTP_LOGS"),
				altsrcyaml.YAML("http.logs", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringSliceFlag{
			Name:        "http.trusted_proxies",
			Usage:       "IP addresses or CIDR ranges of the reverse proxies allowed to set X-Forwarded-For (comma separated)",
			Value:       []string{},
			Destination: &cfg.Server.TrustedProxies,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("HTTP_TRUSTED_PROXIES"),
				altsrcyaml.YAML("http.trusted_proxies", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringFlag{
			Name:        "http.minimum_client_version",
			Usage:       "Oldest Matcha version accepted (header Matcha-Client); empty accepts every version",
			Destination: &cfg.Server.MinimumClientVersion,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("HTTP_MINIMUM_CLIENT_VERSION"),
				altsrcyaml.YAML("http.minimum_client_version", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
