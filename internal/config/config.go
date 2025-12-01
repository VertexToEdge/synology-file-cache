package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config represents the entire application configuration
type Config struct {
	Synology SynologyConfig `mapstructure:"synology"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Sync     SyncConfig     `mapstructure:"sync"`
	HTTP     HTTPConfig     `mapstructure:"http"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

// SynologyConfig contains Synology API configuration
type SynologyConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	Username       string `mapstructure:"username"`
	Password       string `mapstructure:"password"`
	SkipTLSVerify  bool   `mapstructure:"skip_tls_verify"`
}

// CacheConfig contains cache settings
type CacheConfig struct {
	RootDir               string `mapstructure:"root_dir"`
	MaxSizeGB             int    `mapstructure:"max_size_gb"`
	MaxDiskUsagePercent   int    `mapstructure:"max_disk_usage_percent"`
	RecentModifiedDays    int    `mapstructure:"recent_modified_days"`
	RecentAccessedDays    int    `mapstructure:"recent_accessed_days"`
}

// SyncConfig contains synchronization settings
type SyncConfig struct {
	FullScanInterval    string `mapstructure:"full_scan_interval"`
	IncrementalInterval string `mapstructure:"incremental_interval"`
	PrefetchInterval    string `mapstructure:"prefetch_interval"`
}

// HTTPConfig contains HTTP server configuration
type HTTPConfig struct {
	BindAddr string `mapstructure:"bind_addr"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load loads configuration from the specified file path
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set defaults
	viper.SetDefault("synology.skip_tls_verify", false)
	viper.SetDefault("cache.root_dir", "/var/lib/synology-file-cache")
	viper.SetDefault("cache.max_size_gb", 50)
	viper.SetDefault("cache.max_disk_usage_percent", 50)
	viper.SetDefault("cache.recent_modified_days", 30)
	viper.SetDefault("cache.recent_accessed_days", 30)
	viper.SetDefault("sync.full_scan_interval", "1h")
	viper.SetDefault("sync.incremental_interval", "1m")
	viper.SetDefault("sync.prefetch_interval", "30s")
	viper.SetDefault("http.bind_addr", "0.0.0.0:8080")
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate Synology config
	if c.Synology.BaseURL == "" {
		return fmt.Errorf("synology.base_url is required")
	}
	if c.Synology.Username == "" {
		return fmt.Errorf("synology.username is required")
	}
	if c.Synology.Password == "" {
		return fmt.Errorf("synology.password is required")
	}

	// Validate cache config
	if c.Cache.MaxSizeGB <= 0 {
		return fmt.Errorf("cache.max_size_gb must be positive")
	}
	if c.Cache.MaxDiskUsagePercent <= 0 || c.Cache.MaxDiskUsagePercent > 100 {
		return fmt.Errorf("cache.max_disk_usage_percent must be between 1 and 100")
	}

	// Validate sync intervals
	if _, err := time.ParseDuration(c.Sync.FullScanInterval); err != nil {
		return fmt.Errorf("invalid sync.full_scan_interval: %w", err)
	}
	if _, err := time.ParseDuration(c.Sync.IncrementalInterval); err != nil {
		return fmt.Errorf("invalid sync.incremental_interval: %w", err)
	}
	if _, err := time.ParseDuration(c.Sync.PrefetchInterval); err != nil {
		return fmt.Errorf("invalid sync.prefetch_interval: %w", err)
	}

	// Validate logging config
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
		// Valid levels
	default:
		return fmt.Errorf("invalid logging.level: %s", c.Logging.Level)
	}

	switch c.Logging.Format {
	case "json", "text":
		// Valid formats
	default:
		return fmt.Errorf("invalid logging.format: %s", c.Logging.Format)
	}

	return nil
}

// GetFullScanInterval returns the full scan interval as time.Duration
func (c *SyncConfig) GetFullScanInterval() time.Duration {
	d, _ := time.ParseDuration(c.FullScanInterval)
	return d
}

// GetIncrementalInterval returns the incremental interval as time.Duration
func (c *SyncConfig) GetIncrementalInterval() time.Duration {
	d, _ := time.ParseDuration(c.IncrementalInterval)
	return d
}

// GetPrefetchInterval returns the prefetch interval as time.Duration
func (c *SyncConfig) GetPrefetchInterval() time.Duration {
	d, _ := time.ParseDuration(c.PrefetchInterval)
	return d
}