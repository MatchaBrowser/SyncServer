package config

import (
	"strings"
	"testing"
)

// validConfig returns the defaults of the server command.
func validConfig() Config {
	var c Config
	c.Server.Port = 8080
	c.Database.Dialect = "sqlite"
	c.Database.DSN = "./data/syncserver.sqlite"
	c.Registration.Mode = RegistrationInvitation
	c.Sync.MaxPayloadBytes = 262144
	c.Sync.MaxChangesPerPush = 500
	c.Sync.MaxRequestBytes = 4194304
	c.Sync.MaxBlobBytes = 22020096
	c.Sync.MaxAvatarBytes = 270336
	c.Kdf.MinimumIterations = MinKdfIterations
	c.Kdf.MaximumIterations = MaxKdfIterations
	c.Argon2.Memory = 64
	c.Argon2.Time = 3
	c.Argon2.Threads = 4
	c.Argon2.MaxConcurrent = 4
	c.Storage.DataDir = "./data"
	return c
}

func TestValidate_Defaults(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("the defaults must be valid: %v", err)
	}
}

func TestValidate_TrustedProxies(t *testing.T) {
	c := validConfig()
	c.Server.TrustedProxies = []string{"127.0.0.1", "10.0.0.0/8", "::1", "fd00::/8"}
	if err := c.Validate(); err != nil {
		t.Fatalf("IP addresses and CIDR ranges must be accepted: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"port", func(c *Config) { c.Server.Port = 0 }, "http.port"},
		{"proxy", func(c *Config) { c.Server.TrustedProxies = []string{"nginx"} }, "http.trusted_proxies"},
		{"dialect", func(c *Config) { c.Database.Dialect = "mysql" }, "database.dialect"},
		{"dsn", func(c *Config) { c.Database.DSN = " " }, "database.dsn"},
		{"registration", func(c *Config) { c.Registration.Mode = "public" }, "registration.mode"},
		{"payload", func(c *Config) { c.Sync.MaxPayloadBytes = 0 }, "sync.max_payload_bytes"},
		{"changes", func(c *Config) { c.Sync.MaxChangesPerPush = 0 }, "sync.max_changes_per_push"},
		{"request smaller than a payload", func(c *Config) { c.Sync.MaxRequestBytes = 1024 }, "sync.max_request_bytes"},
		{"blob", func(c *Config) { c.Sync.MaxBlobBytes = 0 }, "sync.max_blob_bytes"},
		{"avatar", func(c *Config) { c.Sync.MaxAvatarBytes = 0 }, "sync.max_avatar_bytes"},
		{"quota", func(c *Config) { c.Sync.MaxAccountBytes = -1 }, "sync.max_account_bytes"},
		{"weak kdf", func(c *Config) { c.Kdf.MinimumIterations = 100_000 }, "kdf.minimum_iterations"},
		{"heavy kdf", func(c *Config) { c.Kdf.MaximumIterations = 20_000_000 }, "kdf.maximum_iterations"},
		{"kdf bounds crossed", func(c *Config) { c.Kdf.MinimumIterations = 2_000_000; c.Kdf.MaximumIterations = 1_000_000 }, "must not exceed"},
		{"argon2 memory", func(c *Config) { c.Argon2.Memory = 8 }, "argon2.memory"},
		{"argon2 time", func(c *Config) { c.Argon2.Time = 1 }, "argon2.time"},
		{"argon2 threads", func(c *Config) { c.Argon2.Threads = 0 }, "argon2.threads"},
		{"argon2 concurrency", func(c *Config) { c.Argon2.MaxConcurrent = 0 }, "argon2.max_concurrent"},
		{"data dir", func(c *Config) { c.Storage.DataDir = "" }, "storage.data_dir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected an error mentioning %q, got %q", tt.want, err)
			}
		})
	}
}
