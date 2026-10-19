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
		&cli.StringFlag{
			Name:        "http.cors_allow_origins",
			Value:       "*",
			Destination: &cfg.Server.CorsAllowOrigins,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("HTTP_CORS_ALLOW_ORIGINS"),
				altsrcyaml.YAML("http.cors_allow_origins", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
