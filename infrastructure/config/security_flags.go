package config

import (
	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

// KdfFlags bound the PBKDF2 iterations of the accounts (SYNC.md § 3.1).
func KdfFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "kdf.minimum_iterations",
			Value:       MinKdfIterations,
			Destination: &cfg.Kdf.MinimumIterations,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("KDF_MINIMUM_ITERATIONS"),
				altsrcyaml.YAML("kdf.minimum_iterations", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "kdf.maximum_iterations",
			Value:       MaxKdfIterations,
			Destination: &cfg.Kdf.MaximumIterations,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("KDF_MAXIMUM_ITERATIONS"),
				altsrcyaml.YAML("kdf.maximum_iterations", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}

// Argon2Flags are the parameters used to hash authKey and recoveryAuthKey
// (SYNC.md § 4.9). The defaults follow RFC 9106.
func Argon2Flags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "argon2.memory",
			Usage:       "Memory of one Argon2id hash, in MiB",
			Value:       64,
			Destination: &cfg.Argon2.Memory,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("ARGON2_MEMORY"),
				altsrcyaml.YAML("argon2.memory", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "argon2.time",
			Usage:       "Number of Argon2id passes",
			Value:       3,
			Destination: &cfg.Argon2.Time,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("ARGON2_TIME"),
				altsrcyaml.YAML("argon2.time", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "argon2.threads",
			Usage:       "Argon2id parallelism",
			Value:       4,
			Destination: &cfg.Argon2.Threads,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("ARGON2_THREADS"),
				altsrcyaml.YAML("argon2.threads", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.IntFlag{
			Name:        "argon2.max_concurrent",
			Usage:       "Most Argon2id hashes computed at the same time (each takes argon2.memory)",
			Value:       4,
			Destination: &cfg.Argon2.MaxConcurrent,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("ARGON2_MAX_CONCURRENT"),
				altsrcyaml.YAML("argon2.max_concurrent", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
