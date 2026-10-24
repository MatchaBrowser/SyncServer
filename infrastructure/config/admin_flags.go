package config

import (
	"time"

	altsrc "github.com/urfave/cli-altsrc/v3"
	altsrcyaml "github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

// DefaultAuditRetention is how long the audit events are kept.
const DefaultAuditRetention = 90 * 24 * time.Hour

// AdminFlags configure the admin console (PLAN.md, « Console »).
func AdminFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "admin.listen",
			Usage:       "Address of the admin console (host:port, e.g. 127.0.0.1:9090); empty disables it",
			Destination: &cfg.Admin.Listen,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("ADMIN_LISTEN"),
				altsrcyaml.YAML("admin.listen", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}

// AuditFlags configure the audit journal (PLAN.md, « Journal d'audit »).
func AuditFlags(cfg *Config) []cli.Flag {
	return []cli.Flag{
		&cli.DurationFlag{
			Name:        "audit.retention",
			Usage:       "How long the audit events are kept",
			Value:       DefaultAuditRetention,
			Destination: &cfg.Audit.Retention,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("AUDIT_RETENTION"),
				altsrcyaml.YAML("audit.retention", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
		&cli.BoolFlag{
			Name:        "audit.truncate_ip",
			Usage:       "Keep only the /24 of an IPv4 address and the /48 of an IPv6 address",
			Destination: &cfg.Audit.TruncateIP,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("AUDIT_TRUNCATE_IP"),
				altsrcyaml.YAML("audit.truncate_ip", altsrc.NewStringPtrSourcer(&cfg.ConfigFile)),
			),
		},
	}
}
