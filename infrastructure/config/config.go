package config

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Bounds of the PBKDF2 iterations accepted by the clients (SYNC.md § 3.1).
// A server may be stricter, never looser.
const (
	MinKdfIterations = 600_000
	MaxKdfIterations = 10_000_000
)

// Minimum Argon2id parameters for hashing authKey and recoveryAuthKey
// (OWASP recommendation, SYNC.md § 4.9).
const (
	MinArgon2MemoryMiB = 19
	MinArgon2Time      = 2
	MinArgon2Threads   = 1
)

// Registration modes announced by GET /v1/server (SYNC.md § 4.2).
const (
	RegistrationOpen       = "open"
	RegistrationInvitation = "invitation"
	RegistrationClosed     = "closed"
)

type Config struct {
	// Version of the application
	Version string

	// ConfigFile is the path to the configuration file
	ConfigFile string

	Server struct {
		// Port is the server port
		Port int
		// HttpLogs indicates if HTTP logs are enabled
		HttpLogs bool
		// TrustedProxies lists the IP addresses or CIDR ranges of the reverse
		// proxies allowed to set X-Forwarded-For. Empty: the client IP is the
		// address of the TCP connection.
		TrustedProxies []string
		// MinimumClientVersion makes the server answer 426 unsupported_client
		// to Matcha versions older than this one (header Matcha-Client).
		// Empty: every version is accepted.
		MinimumClientVersion string
	}

	// Logger is the configuration for the zerolog logger.
	// Level is the log level for the logger.
	// Pretty enables or disables pretty printing of logs (non JSON logs).
	Logger struct {
		Level  string
		Pretty bool
	}

	// Database is the configuration for the database connection.
	// Dialect is the database engine (sqlite, postgres).
	// DSN is the Data Source Name for the database connection.
	// The migrations are run by the migration command.
	Database struct {
		Dialect string
		DSN     string
	}

	// Registration is the configuration of account creation.
	// Mode is open, invitation or closed.
	Registration struct {
		Mode string
	}

	// Sync holds the limits announced by GET /v1/server (SYNC.md § 4.1).
	// MaxAccountBytes 0 means no quota.
	Sync struct {
		MaxPayloadBytes   int
		MaxChangesPerPush int
		MaxRequestBytes   int
		MaxBlobBytes      int
		MaxAvatarBytes    int
		MaxAccountBytes   int
	}

	// Kdf bounds the PBKDF2 iterations the server accepts for an account.
	Kdf struct {
		MinimumIterations int
		MaximumIterations int
	}

	// Argon2 holds the parameters used to hash authKey and recoveryAuthKey.
	// Memory is in MiB. MaxConcurrent bounds the hashes computed at the same
	// time, each taking Memory.
	Argon2 struct {
		Memory        int
		Time          int
		Threads       int
		MaxConcurrent int
	}

	// Storage is where the server keeps its files (extension packages,
	// avatars, epoch marker).
	Storage struct {
		DataDir string
	}

	// Admin is the admin console. Listen is its address (host:port); empty
	// disables it.
	Admin struct {
		Listen string
	}

	// Audit is the journal of the security events. Retention is how long an
	// event is kept; TruncateIP keeps only the /24 of an IPv4 address and the
	// /48 of an IPv6 address.
	Audit struct {
		Retention  time.Duration
		TruncateIP bool
	}

	ExportOapi struct {
		FileName string
	}
}

// Validate checks the configuration of the server command.
func (c Config) Validate() error {
	var errs []error

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("http.port must be between 1 and 65535, got %d", c.Server.Port))
	}
	for _, proxy := range c.Server.TrustedProxies {
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				errs = append(errs, fmt.Errorf("http.trusted_proxies: %q is neither an IP address nor a CIDR range", proxy))
			}
		}
	}

	if !slices.Contains([]string{"sqlite", "postgres"}, c.Database.Dialect) {
		errs = append(errs, fmt.Errorf("database.dialect must be sqlite or postgres, got %q", c.Database.Dialect))
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		errs = append(errs, errors.New("database.dsn must be set"))
	}

	if !slices.Contains([]string{RegistrationOpen, RegistrationInvitation, RegistrationClosed}, c.Registration.Mode) {
		errs = append(errs, fmt.Errorf("registration.mode must be open, invitation or closed, got %q", c.Registration.Mode))
	}

	if c.Sync.MaxPayloadBytes <= 0 {
		errs = append(errs, errors.New("sync.max_payload_bytes must be positive"))
	}
	if c.Sync.MaxChangesPerPush <= 0 {
		errs = append(errs, errors.New("sync.max_changes_per_push must be positive"))
	}
	// A push of a single change must fit in a request.
	if c.Sync.MaxRequestBytes < c.Sync.MaxPayloadBytes {
		errs = append(errs, errors.New("sync.max_request_bytes must be at least sync.max_payload_bytes"))
	}
	if c.Sync.MaxBlobBytes <= 0 {
		errs = append(errs, errors.New("sync.max_blob_bytes must be positive"))
	}
	if c.Sync.MaxAvatarBytes <= 0 {
		errs = append(errs, errors.New("sync.max_avatar_bytes must be positive"))
	}
	if c.Sync.MaxAccountBytes < 0 {
		errs = append(errs, errors.New("sync.max_account_bytes must be 0 (no quota) or positive"))
	}

	if c.Kdf.MinimumIterations < MinKdfIterations {
		errs = append(errs, fmt.Errorf("kdf.minimum_iterations must be at least %d", MinKdfIterations))
	}
	if c.Kdf.MaximumIterations > MaxKdfIterations {
		errs = append(errs, fmt.Errorf("kdf.maximum_iterations must be at most %d", MaxKdfIterations))
	}
	if c.Kdf.MinimumIterations > c.Kdf.MaximumIterations {
		errs = append(errs, errors.New("kdf.minimum_iterations must not exceed kdf.maximum_iterations"))
	}

	if c.Argon2.Memory < MinArgon2MemoryMiB {
		errs = append(errs, fmt.Errorf("argon2.memory must be at least %d MiB", MinArgon2MemoryMiB))
	}
	if c.Argon2.Time < MinArgon2Time {
		errs = append(errs, fmt.Errorf("argon2.time must be at least %d", MinArgon2Time))
	}
	if c.Argon2.Threads < MinArgon2Threads || c.Argon2.Threads > 255 {
		errs = append(errs, errors.New("argon2.threads must be between 1 and 255"))
	}
	if c.Argon2.MaxConcurrent < 1 {
		errs = append(errs, errors.New("argon2.max_concurrent must be at least 1"))
	}

	if strings.TrimSpace(c.Storage.DataDir) == "" {
		errs = append(errs, errors.New("storage.data_dir must be set"))
	}

	if c.Admin.Listen != "" {
		if _, port, err := net.SplitHostPort(c.Admin.Listen); err != nil {
			errs = append(errs, fmt.Errorf("admin.listen must be host:port, got %q", c.Admin.Listen))
		} else if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			errs = append(errs, fmt.Errorf("admin.listen: port must be between 1 and 65535, got %q", port))
		}
	}
	if c.Audit.Retention < time.Hour {
		errs = append(errs, errors.New("audit.retention must be at least 1h"))
	}

	return errors.Join(errs...)
}
