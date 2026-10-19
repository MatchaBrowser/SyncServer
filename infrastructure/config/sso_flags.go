package config

import (
	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

func SSOFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:        "sso.enabled",
			Destination: &cfg.SSO.Enabled,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_ENABLED"),
				altsrcyaml.YAML("sso.enabled", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringFlag{
			Name:        "sso.client_id",
			Destination: &cfg.SSO.ClientID,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_CLIENT_ID"),
				altsrcyaml.YAML("sso.client_id", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringFlag{
			Name:        "sso.client_secret",
			Destination: &cfg.SSO.ClientSecret,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_CLIENT_SECRET"),
				altsrcyaml.YAML("sso.client_secret", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringFlag{
			Name:        "sso.issuer_url",
			Destination: &cfg.SSO.IssuerURL,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_ISSUER_URL"),
				altsrcyaml.YAML("sso.issuer_url", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringFlag{
			Name:        "sso.auth_url",
			Destination: &cfg.SSO.AuthURL,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_AUTH_URL"),
				altsrcyaml.YAML("sso.auth_url", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringFlag{
			Name:        "sso.token_url",
			Destination: &cfg.SSO.TokenURL,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_TOKEN_URL"),
				altsrcyaml.YAML("sso.token_url", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringFlag{
			Name:        "sso.redirect_url",
			Destination: &cfg.SSO.RedirectURL,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_REDIRECT_URL"),
				altsrcyaml.YAML("sso.redirect_url", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.StringSliceFlag{
			Name:        "sso.scopes",
			Destination: &cfg.SSO.Scopes,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SSO_SCOPES"),
				altsrcyaml.YAML("sso.scopes", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
