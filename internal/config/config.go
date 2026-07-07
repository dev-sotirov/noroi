package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Parameter bounds for /respond endpoint (production safety).
const (
	MaxCPUMs      = 10_000  // 10 seconds max CPU burn
	MaxChunks     = 100_000 // 100k max chunks per request
	MaxDelay      = 1 * time.Hour
	MaxChunkDelay = 1 * time.Hour
)

// ServerConfig holds HTTP server tuning parameters.
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// DefaultsConfig holds the per-request response defaults applied when a
// request does not override them via scenario configuration.
type DefaultsConfig struct {
	Delay      time.Duration `mapstructure:"delay"`
	Jitter     time.Duration `mapstructure:"jitter"`
	StatusCode int           `mapstructure:"status_code"`
	BodySize   int           `mapstructure:"body_size"`  // bytes
	BodyType   string        `mapstructure:"body_type"`  // text | json | binary | zeros
	ErrorRate  float64       `mapstructure:"error_rate"` // 0.0 – 1.0
}

// MetricsConfig controls Prometheus metrics exposure.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// LogLevel enumerates accepted log verbosity levels.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogFormat enumerates accepted log output formats.
type LogFormat string

const (
	LogFormatJSON   LogFormat = "json"
	LogFormatPretty LogFormat = "pretty"
)

// LoggingConfig controls structured log output.
type LoggingConfig struct {
	Level  LogLevel  `mapstructure:"level"`
	Format LogFormat `mapstructure:"format"`
}

// RateLimitConfig controls the optional in-server rate limiter.
type RateLimitConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	RPS            float64       `mapstructure:"rps"`   // requests per second (token refill rate)
	Burst          int           `mapstructure:"burst"` // maximum burst size
	TrustForwarded bool          `mapstructure:"trust_forwarded"`
	TTL            time.Duration `mapstructure:"ttl"` // limiter eviction TTL
}

// Config is the top-level application configuration. It is immutable after
// Load returns — no global state is held.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Defaults  DefaultsConfig  `mapstructure:"defaults"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
}

// Load reads configuration from the YAML file at cfgPath (defaults to
// ".noroi.yaml" when cfgPath is empty), overlays any NOROI_* environment
// variables, validates the result, and returns a fresh *Config.
//
// Environment variable mapping uses underscore-separated keys under the
// NOROI_ prefix, e.g.:
//
//	NOROI_SERVER_PORT=9090
//	NOROI_DEFAULTS_ERROR_RATE=0.05
func Load(cfgPath string) (*Config, error) {
	v := viper.New()

	// ---------------------------------------------------------------------------
	// Defaults
	// ---------------------------------------------------------------------------
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "10s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "60s")

	v.SetDefault("defaults.delay", "0ms")
	v.SetDefault("defaults.jitter", "0ms")
	v.SetDefault("defaults.status_code", 200)
	v.SetDefault("defaults.body_size", 256)
	v.SetDefault("defaults.body_type", "text")
	v.SetDefault("defaults.error_rate", 0.0)

	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")

	v.SetDefault("rate_limit.enabled", false)
	v.SetDefault("rate_limit.rps", 1000.0)
	v.SetDefault("rate_limit.burst", 100)
	v.SetDefault("rate_limit.trust_forwarded", false)
	v.SetDefault("rate_limit.ttl", "5m")

	v.SetDefault("logging.level", string(LogLevelInfo))
	v.SetDefault("logging.format", string(LogFormatJSON))

	// ---------------------------------------------------------------------------
	// Config file
	// ---------------------------------------------------------------------------
	if cfgPath == "" {
		cfgPath = ".noroi.yaml"
	}
	v.SetConfigFile(cfgPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		// A missing file is acceptable; other errors (e.g. YAML parse failures)
		// are fatal.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// viper.SetConfigFile does not produce ConfigFileNotFoundError on
			// missing files — it returns *os.PathError instead. Treat any read
			// error as non-fatal when using the default path so the binary can
			// start without a config file, but propagate errors for explicit paths.
			if cfgPath != ".noroi.yaml" {
				return nil, fmt.Errorf("config: read %q: %w", cfgPath, err)
			}
		}
	}

	// ---------------------------------------------------------------------------
	// Environment variables  (NOROI_SERVER_PORT, NOROI_DEFAULTS_ERROR_RATE, …)
	// ---------------------------------------------------------------------------
	v.SetEnvPrefix("NOROI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// ---------------------------------------------------------------------------
	// Unmarshal
	// ---------------------------------------------------------------------------
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	// ---------------------------------------------------------------------------
	// Validation
	// ---------------------------------------------------------------------------
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config: validation: %w", err)
	}

	return &cfg, nil
}

// validate performs semantic validation of the fully-populated Config.
func validate(cfg *Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port %d is out of range [1, 65535]", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout <= 0 {
		return fmt.Errorf("server.read_timeout must be positive")
	}
	if cfg.Server.WriteTimeout <= 0 {
		return fmt.Errorf("server.write_timeout must be positive")
	}
	if cfg.Server.IdleTimeout <= 0 {
		return fmt.Errorf("server.idle_timeout must be positive")
	}

	if cfg.Defaults.StatusCode < 100 || cfg.Defaults.StatusCode > 599 {
		return fmt.Errorf("defaults.status_code %d is not a valid HTTP status code", cfg.Defaults.StatusCode)
	}
	if cfg.Defaults.BodySize < 0 {
		return fmt.Errorf("defaults.body_size must be >= 0")
	}
	switch cfg.Defaults.BodyType {
	case "text", "json", "binary", "zeros":
		// valid
	default:
		return fmt.Errorf("defaults.body_type %q must be one of: text, json, binary, zeros", cfg.Defaults.BodyType)
	}
	if cfg.Defaults.ErrorRate < 0.0 || cfg.Defaults.ErrorRate > 1.0 {
		return fmt.Errorf("defaults.error_rate %.4f is out of range [0.0, 1.0]", cfg.Defaults.ErrorRate)
	}
	if cfg.Defaults.Delay < 0 {
		return fmt.Errorf("defaults.delay must be >= 0")
	}
	if cfg.Defaults.Jitter < 0 {
		return fmt.Errorf("defaults.jitter must be >= 0")
	}

	if cfg.Metrics.Path == "" || !strings.HasPrefix(cfg.Metrics.Path, "/") {
		return fmt.Errorf("metrics.path %q must be a non-empty absolute path", cfg.Metrics.Path)
	}

	switch cfg.Logging.Level {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		// valid
	default:
		return fmt.Errorf("logging.level %q must be one of: debug, info, warn, error", cfg.Logging.Level)
	}
	switch cfg.Logging.Format {
	case LogFormatJSON, LogFormatPretty:
		// valid
	default:
		return fmt.Errorf("logging.format %q must be one of: json, pretty", cfg.Logging.Format)
	}

	if cfg.RateLimit.Enabled {
		if cfg.RateLimit.RPS <= 0 {
			return fmt.Errorf("rate_limit.rps must be > 0, got %.2f", cfg.RateLimit.RPS)
		}
		if cfg.RateLimit.Burst < 1 {
			return fmt.Errorf("rate_limit.burst must be >= 1, got %d", cfg.RateLimit.Burst)
		}
		if cfg.RateLimit.TTL <= 0 {
			return fmt.Errorf("rate_limit.ttl must be positive")
		}
	}

	return nil
}
