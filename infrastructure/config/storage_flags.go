package config

import (
	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

func StorageFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "storage.data_dir",
			Usage:       "Directory of the server files (extension packages, avatars, epoch marker)",
			Value:       "./data",
			Destination: &cfg.Storage.DataDir,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("STORAGE_DATA_DIR"),
				altsrcyaml.YAML("storage.data_dir", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
