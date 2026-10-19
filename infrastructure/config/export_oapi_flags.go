package config

import (
	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

func ExportOapiFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "export-oapi.filename",
			Usage:       "The output file name for the OpenAPI export",
			Aliases:     []string{"export.oapi.filename", "f"},
			Value:       "openapi.yaml",
			Destination: &cfg.ExportOapi.FileName,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("EXPORT_OAPI_FILENAME"),
				altsrcyaml.YAML("export-oapi.filename", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
