package config

import (
	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

// SyncFlags are the limits announced by GET /v1/server (SYNC.md § 4.1).
func SyncFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "sync.max_payload_bytes",
			Usage:       "Largest encrypted record, decoded from base 64",
			Value:       262144, // 256 KiB
			Destination: &cfg.Sync.MaxPayloadBytes,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SYNC_MAX_PAYLOAD_BYTES"),
				altsrcyaml.YAML("sync.max_payload_bytes", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "sync.max_changes_per_push",
			Usage:       "Most changes in one push",
			Value:       500,
			Destination: &cfg.Sync.MaxChangesPerPush,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SYNC_MAX_CHANGES_PER_PUSH"),
				altsrcyaml.YAML("sync.max_changes_per_push", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "sync.max_request_bytes",
			Usage:       "Largest JSON request body, as received",
			Value:       4194304, // 4 MiB
			Destination: &cfg.Sync.MaxRequestBytes,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SYNC_MAX_REQUEST_BYTES"),
				altsrcyaml.YAML("sync.max_request_bytes", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "sync.max_blob_bytes",
			Usage:       "Largest encrypted extension package",
			Value:       22020096, // 21 MiB: a 20 MiB package plus its padding
			Destination: &cfg.Sync.MaxBlobBytes,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SYNC_MAX_BLOB_BYTES"),
				altsrcyaml.YAML("sync.max_blob_bytes", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "sync.max_avatar_bytes",
			Usage:       "Largest encrypted avatar of an account",
			Value:       270336, // 264 KiB: a 256 KiB picture plus its padding
			Destination: &cfg.Sync.MaxAvatarBytes,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SYNC_MAX_AVATAR_BYTES"),
				altsrcyaml.YAML("sync.max_avatar_bytes", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "sync.max_account_bytes",
			Usage:       "Quota of an account (records, packages and avatar); 0 means no quota",
			Value:       0,
			Destination: &cfg.Sync.MaxAccountBytes,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("SYNC_MAX_ACCOUNT_BYTES"),
				altsrcyaml.YAML("sync.max_account_bytes", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
