package config

import (
	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

func RegistrationFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "registration.mode",
			Usage:       "Account creation: open, invitation (code given by the administrator) or closed",
			Value:       RegistrationInvitation,
			Destination: &cfg.Registration.Mode,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("REGISTRATION_MODE"),
				altsrcyaml.YAML("registration.mode", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
