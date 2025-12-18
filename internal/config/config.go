// Package config handles configuration loading and validation
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/sanskarpan/db-backup/internal/logger"
)

// Config represents the complete application configuration
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Logging       logger.Config       `mapstructure:"logging"`
	Backup        BackupConfig        `mapstructure:"backup"`
	Storage       StorageConfig       `mapstructure:"storage"`
	Notifications NotificationConfig  `mapstructure:"notifications"`
	Metrics       MetricsConfig       `mapstructure:"metrics"`
	Tracing       TracingConfig       `mapstructure:"tracing"`
	Security      SecurityConfig      `mapstructure:"security"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Host string    `mapstructure:"host"`
	Port int       `mapstructure:"port"`
	Mode string    `mapstructure:"mode"` // development, production
	TLS  TLSConfig `mapstructure:"tls"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// DatabaseConfig holds database configuration for metadata storage
type DatabaseConfig struct {
	Metadata MetadataDBConfig `mapstructure:"metadata"`
	Redis    RedisConfig      `mapstructure:"redis"`
}

// MetadataDBConfig holds metadata database configuration
type MetadataDBConfig struct {
	Type           string `mapstructure:"type"`
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	Name           string `mapstructure:"name"`
	User           string `mapstructure:"user"`
	Password       string `mapstructure:"password"`
	MaxConnections int    `mapstructure:"max_connections"`
	SSLMode        string `mapstructure:"ssl_mode"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// BackupConfig holds backup configuration
type BackupConfig struct {
	DefaultCompression string            `mapstructure:"default_compression"`
	CompressionLevel   int               `mapstructure:"compression_level"`
	Encryption         EncryptionConfig  `mapstructure:"encryption"`
	Retention          RetentionConfig   `mapstructure:"retention"`
	TempDirectory      string            `mapstructure:"temp_directory"`
	MetadataDirectory  string            `mapstructure:"metadata_directory"`
	ParallelOperations int               `mapstructure:"parallel_operations"`
}

// EncryptionConfig holds encryption configuration
type EncryptionConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Algorithm string `mapstructure:"algorithm"`
	KeyFile   string `mapstructure:"key_file"`
}

// RetentionConfig holds backup retention configuration
type RetentionConfig struct {
	Daily   int `mapstructure:"daily"`
	Weekly  int `mapstructure:"weekly"`
	Monthly int `mapstructure:"monthly"`
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	DefaultProvider string                 `mapstructure:"default_provider"`
	Providers       StorageProviders       `mapstructure:"providers"`
}

// StorageProviders holds all storage provider configurations
type StorageProviders struct {
	S3    S3Config    `mapstructure:"s3"`
	GCS   GCSConfig   `mapstructure:"gcs"`
	Azure AzureConfig `mapstructure:"azure"`
	Local LocalConfig `mapstructure:"local"`
}

// S3Config holds AWS S3 configuration
type S3Config struct {
	Enabled       bool   `mapstructure:"enabled"`
	Region        string `mapstructure:"region"`
	Bucket        string `mapstructure:"bucket"`
	AccessKey     string `mapstructure:"access_key"`
	SecretKey     string `mapstructure:"secret_key"`
	Endpoint      string `mapstructure:"endpoint"`
	UsePathStyle  bool   `mapstructure:"use_path_style"`
}

// GCSConfig holds Google Cloud Storage configuration
type GCSConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Project         string `mapstructure:"project"`
	Bucket          string `mapstructure:"bucket"`
	CredentialsFile string `mapstructure:"credentials_file"`
}

// AzureConfig holds Azure Blob Storage configuration
type AzureConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	AccountName string `mapstructure:"account_name"`
	AccountKey  string `mapstructure:"account_key"`
	Container   string `mapstructure:"container"`
}

// LocalConfig holds local storage configuration
type LocalConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// NotificationConfig holds notification configuration
type NotificationConfig struct {
	Slack   SlackConfig   `mapstructure:"slack"`
	Email   EmailConfig   `mapstructure:"email"`
	Webhook WebhookConfig `mapstructure:"webhook"`
}

// SlackConfig holds Slack notification configuration
type SlackConfig struct {
	Enabled    bool     `mapstructure:"enabled"`
	WebhookURL string   `mapstructure:"webhook_url"`
	Channel    string   `mapstructure:"channel"`
	NotifyOn   []string `mapstructure:"notify_on"`
}

// EmailConfig holds email notification configuration
type EmailConfig struct {
	Enabled  bool     `mapstructure:"enabled"`
	SMTPHost string   `mapstructure:"smtp_host"`
	SMTPPort int      `mapstructure:"smtp_port"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
	From     string   `mapstructure:"from"`
	To       []string `mapstructure:"to"`
}

// WebhookConfig holds webhook notification configuration
type WebhookConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	URL     string            `mapstructure:"url"`
	Method  string            `mapstructure:"method"`
	Headers map[string]string `mapstructure:"headers"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled    bool             `mapstructure:"enabled"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
}

// PrometheusConfig holds Prometheus configuration
type PrometheusConfig struct {
	Port int    `mapstructure:"port"`
	Path string `mapstructure:"path"`
}

// TracingConfig holds tracing configuration
type TracingConfig struct {
	Enabled bool         `mapstructure:"enabled"`
	Jaeger  JaegerConfig `mapstructure:"jaeger"`
}

// JaegerConfig holds Jaeger configuration
type JaegerConfig struct {
	Endpoint    string `mapstructure:"endpoint"`
	ServiceName string `mapstructure:"service_name"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	JWT          JWTConfig          `mapstructure:"jwt"`
	APIKeys      APIKeysConfig      `mapstructure:"api_keys"`
	RateLimiting RateLimitingConfig `mapstructure:"rate_limiting"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	Expiration time.Duration `mapstructure:"expiration"`
}

// APIKeysConfig holds API keys configuration
type APIKeysConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// RateLimitingConfig holds rate limiting configuration
type RateLimitingConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	RequestsPerMinute  int  `mapstructure:"requests_per_minute"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Set config file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Search for config in common locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/db-backup/")
		v.AddConfigPath("$HOME/.db-backup/")
	}

	// Enable environment variable override
	v.SetEnvPrefix("DBBACKUP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, use defaults and environment variables
	}

	// Unmarshal config
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "development")
	v.SetDefault("server.tls.enabled", false)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Backup defaults
	v.SetDefault("backup.default_compression", "zstd")
	v.SetDefault("backup.compression_level", 3)
	v.SetDefault("backup.encryption.enabled", false)
	v.SetDefault("backup.retention.daily", 7)
	v.SetDefault("backup.retention.weekly", 4)
	v.SetDefault("backup.retention.monthly", 12)
	v.SetDefault("backup.temp_directory", "/tmp/backups")
	v.SetDefault("backup.parallel_operations", 4)

	// Storage defaults
	v.SetDefault("storage.default_provider", "local")
	v.SetDefault("storage.providers.local.enabled", true)
	v.SetDefault("storage.providers.local.path", "./backups")

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.prometheus.port", 9090)
	v.SetDefault("metrics.prometheus.path", "/metrics")

	// Security defaults
	v.SetDefault("security.jwt.expiration", "24h")
	// NOTE: JWT secret MUST be set via environment variable DBBACKUP_SECURITY_JWT_SECRET
	// or in config file for security reasons. No default is provided.
	v.SetDefault("security.api_keys.enabled", false)
	v.SetDefault("security.rate_limiting.enabled", true)
	v.SetDefault("security.rate_limiting.requests_per_minute", 100)
}

// validate validates the configuration
func validate(config *Config) error {
	// Validate server config
	if config.Server.Port < 1 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	// Validate TLS config
	if config.Server.TLS.Enabled {
		if config.Server.TLS.CertFile == "" || config.Server.TLS.KeyFile == "" {
			return fmt.Errorf("TLS enabled but cert_file or key_file not specified")
		}
		if _, err := os.Stat(config.Server.TLS.CertFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS cert file not found: %s", config.Server.TLS.CertFile)
		}
		if _, err := os.Stat(config.Server.TLS.KeyFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS key file not found: %s", config.Server.TLS.KeyFile)
		}
	}

	// Validate backup config
	if config.Backup.ParallelOperations < 1 {
		return fmt.Errorf("parallel_operations must be at least 1")
	}

	// Validate temp directory
	if config.Backup.TempDirectory != "" {
		if err := os.MkdirAll(config.Backup.TempDirectory, 0755); err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	// Validate storage config
	hasEnabledProvider := false
	if config.Storage.Providers.S3.Enabled {
		hasEnabledProvider = true
	}
	if config.Storage.Providers.GCS.Enabled {
		hasEnabledProvider = true
	}
	if config.Storage.Providers.Azure.Enabled {
		hasEnabledProvider = true
	}
	if config.Storage.Providers.Local.Enabled {
		hasEnabledProvider = true
		// Create local storage directory if it doesn't exist
		if err := os.MkdirAll(config.Storage.Providers.Local.Path, 0755); err != nil {
			return fmt.Errorf("failed to create local storage directory: %w", err)
		}
	}

	if !hasEnabledProvider {
		return fmt.Errorf("at least one storage provider must be enabled")
	}

	// Validate JWT secret - CRITICAL FOR SECURITY
	if config.Security.JWT.Secret == "" {
		return fmt.Errorf("JWT secret is required. Set via DBBACKUP_SECURITY_JWT_SECRET environment variable or config file")
	}
	if len(config.Security.JWT.Secret) < 32 {
		return fmt.Errorf("JWT secret must be at least 32 characters long for security")
	}
	// Warn about common insecure values
	insecureSecrets := []string{"secret", "password", "test", "changeme", "default", "jwt-secret", "your-secret-key"}
	for _, insecure := range insecureSecrets {
		if config.Security.JWT.Secret == insecure {
			return fmt.Errorf("JWT secret appears to be insecure (%q). Please use a strong random secret", insecure)
		}
	}

	return nil
}
