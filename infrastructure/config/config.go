package config

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
		// CorsAllowOrigins is a comma-separated list of allowed CORS origins
		CorsAllowOrigins string
	}

	// Logger is the configuration for the zerolog logger.
	// Level is the log level for the logger.
	// Pretty enables or disables pretty printing of logs (non JSON logs).
	Logger struct {
		Level  string
		Pretty bool
	}

	// Database is the configuration for the database connection.
	// Dialect is the database engine (sqlite, postgres, etc.).
	// DSN is the Data Source Name for the database connection.
	Database struct {
		Dialect string // Database engine (sqlite, postgres, etc.)
		DSN     string
	}

	Session struct {
		SecretKey         string
		ExpirationMinutes int
		Issuer            string
	}

	Auth struct {
		DisableAdminAccount bool
	}

	Registration struct {
		Enabled                  bool     // Enable or disable user registration
		RequireEmailVerification bool     // Require email verification for new registrations
		DomainWhitelist          []string // List of allowed domains for registration
		PasswordMinLength        int      // Minimum password length for registration
		PasswordComplexity       bool     // Require complex passwords (uppercase, lowercase, numbers, symbols)
	}

	SSO struct {
		Enabled      bool
		ClientID     string
		ClientSecret string
		// IssuerURL is the base URL of the OIDC provider. Used to build the userinfo URL
		// and as a fallback when AuthURL/TokenURL are not set.
		IssuerURL string
		// AuthURL and TokenURL can be set explicitly to override OIDC discovery.
		// If left empty, they are auto-discovered from IssuerURL + /.well-known/openid-configuration.
		AuthURL     string
		TokenURL    string
		RedirectURL string
		Scopes      []string
	}

	ExportOapi struct {
		FileName string
	}
}
