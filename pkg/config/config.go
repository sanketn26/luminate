package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Server            ServerConfig            `yaml:"server"`
	Auth              AuthConfig              `yaml:"auth"`
	Storage           StorageConfig           `yaml:"storage"`
	RateLimits        RateLimitsConfig        `yaml:"rate_limits"`
	CardinalityLimits CardinalityLimitsConfig `yaml:"cardinality_limits"`
	QueryLimits       QueryLimitsConfig       `yaml:"query_limits"`
	BatchWrite        BatchWriteConfig        `yaml:"batch_write"`
	Retention         RetentionConfig         `yaml:"retention"`
	Logging           LoggingConfig           `yaml:"logging"`
	Metrics           MetricsConfig           `yaml:"metrics"`
}

type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type AuthConfig struct {
	Enabled         bool   `yaml:"enabled"`
	JWTSecret       string `yaml:"jwt_secret"`
	JWTPublicKeyPath string `yaml:"jwt_public_key_path"`
	TokenExpiry     time.Duration `yaml:"token_expiry"`
}

type StorageConfig struct {
	Backend    string              `yaml:"backend"`
	Badger     BadgerConfig        `yaml:"badger"`
	ClickHouse ClickHouseConfig    `yaml:"clickhouse"`
}

type BadgerConfig struct {
	DataDir            string `yaml:"data_dir"`
	ValueLogMaxEntries int    `yaml:"value_log_max_entries"`
}

type ClickHouseConfig struct {
	Addresses       []string      `yaml:"addresses"`
	Database        string        `yaml:"database"`
	Username        string        `yaml:"username"`
	Password        string        `yaml:"password"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type RateLimitsConfig struct {
	WriteRequestsPerSecond int `yaml:"write_requests_per_second"`
	QueryRequestsPerSecond int `yaml:"query_requests_per_second"`
	MetricsPerSecond       int `yaml:"metrics_per_second"`
	ConcurrentQueries      int `yaml:"concurrent_queries"`
}

type CardinalityLimitsConfig struct {
	MaxDimensionsPerMetric    int `yaml:"max_dimensions_per_metric"`
	MaxDimensionKeyLength     int `yaml:"max_dimension_key_length"`
	MaxDimensionValueLength   int `yaml:"max_dimension_value_length"`
	MaxUniqueSeriesPerMetric  int `yaml:"max_unique_series_per_metric"`
	MaxMetricsPerTenant       int `yaml:"max_metrics_per_tenant"`
}

type QueryLimitsConfig struct {
	MaxQueryTimeout         time.Duration `yaml:"max_query_timeout"`
	DefaultQueryTimeout     time.Duration `yaml:"default_query_timeout"`
	MaxQueryTimeRange       time.Duration `yaml:"max_query_time_range"`
	MaxResultPoints         int           `yaml:"max_result_points"`
	MaxGroupByCardinality   int           `yaml:"max_group_by_cardinality"`
}

type BatchWriteConfig struct {
	MaxBatchSize        int  `yaml:"max_batch_size"`
	MaxRequestSizeBytes int  `yaml:"max_request_size_bytes"`
	Compression         bool `yaml:"compression"`
}

type RetentionConfig struct {
	DefaultRetention time.Duration `yaml:"default_retention"`
	CleanupInterval  time.Duration `yaml:"cleanup_interval"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

type MetricsConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Path     string        `yaml:"path"`
	Interval time.Duration `yaml:"interval"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func (c *Config) setDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 30 * time.Second
	}
	if c.Server.ShutdownTimeout == 0 {
		c.Server.ShutdownTimeout = 10 * time.Second
	}

	if c.Storage.Backend == "" {
		c.Storage.Backend = "badger"
	}

	if c.QueryLimits.DefaultQueryTimeout == 0 {
		c.QueryLimits.DefaultQueryTimeout = 10 * time.Second
	}
	if c.QueryLimits.MaxQueryTimeout == 0 {
		c.QueryLimits.MaxQueryTimeout = 30 * time.Second
	}
}

func (c *Config) validate() error {
	if c.Storage.Backend != "badger" && c.Storage.Backend != "clickhouse" {
		return fmt.Errorf("invalid storage backend: %s (must be 'badger' or 'clickhouse')", c.Storage.Backend)
	}

	if c.Auth.Enabled && c.Auth.JWTSecret == "" && c.Auth.JWTPublicKeyPath == "" {
		return fmt.Errorf("auth enabled but no JWT secret or public key provided")
	}

	return nil
}
