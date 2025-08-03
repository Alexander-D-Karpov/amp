package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/Alexander-D-Karpov/amp/internal/platform"
)

// Config represents the application configuration
type Config struct {
	Debug bool `mapstructure:"debug"`

	API struct {
		BaseURL   string `mapstructure:"base_url"`
		Token     string `mapstructure:"token"`
		RateLimit struct {
			RequestsPerSecond int `mapstructure:"requests_per_second"`
			BurstSize         int `mapstructure:"burst_size"`
		} `mapstructure:"rate_limit"`
		Timeout   int    `mapstructure:"timeout"`
		Retries   int    `mapstructure:"retries"`
		UserAgent string `mapstructure:"user_agent"`
	} `mapstructure:"api"`

	Storage struct {
		DatabasePath string `mapstructure:"database_path"`
		CacheDir     string `mapstructure:"cache_dir"`
		MaxCacheSize int64  `mapstructure:"max_cache_size"`
		SyncInterval int    `mapstructure:"sync_interval"`
		EnableWAL    bool   `mapstructure:"enable_wal"`
	} `mapstructure:"storage"`

	Audio struct {
		SampleRate    int     `mapstructure:"sample_rate"`
		BufferSize    int     `mapstructure:"buffer_size"`
		DefaultVolume float64 `mapstructure:"default_volume"`
		Crossfade     bool    `mapstructure:"crossfade"`
	} `mapstructure:"audio"`

	UI struct {
		Theme        string `mapstructure:"theme"`
		Language     string `mapstructure:"language"`
		ShowStats    bool   `mapstructure:"show_stats"`
		GridColumns  int    `mapstructure:"grid_columns"`
		WindowWidth  int    `mapstructure:"window_width"`
		WindowHeight int    `mapstructure:"window_height"`
	} `mapstructure:"ui"`

	Search struct {
		MaxResults     int     `mapstructure:"max_results"`
		FuzzyThreshold float64 `mapstructure:"fuzzy_threshold"`
		EnableTyping   bool    `mapstructure:"enable_typing"`
		DebounceMs     int     `mapstructure:"debounce_ms"`
	} `mapstructure:"search"`

	Download struct {
		MaxConcurrent int    `mapstructure:"max_concurrent"`
		ChunkSize     int    `mapstructure:"chunk_size"`
		TempDir       string `mapstructure:"temp_dir"`
		AutoDownload  bool   `mapstructure:"auto_download"`
	} `mapstructure:"download"`

	User struct {
		ID          int    `mapstructure:"id"`
		Username    string `mapstructure:"username"`
		Email       string `mapstructure:"email"`
		Image       string `mapstructure:"image"`
		IsAnonymous bool   `mapstructure:"is_anonymous"`
		AnonymousID string `mapstructure:"anonymous_id"`
	} `mapstructure:"user"`
}

// Load loads configuration from file or uses defaults
func Load(configPath string) (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		configDir, err := platform.GetConfigDir()
		if err != nil {
			return nil, err
		}
		viper.AddConfigPath(configDir)
		viper.AddConfigPath("./configs")
		viper.AddConfigPath(".")
	}

	viper.SetEnvPrefix("AMP")
	viper.AutomaticEnv()

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if err := ensureDirectories(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DefaultMobileConfig returns a default configuration optimized for mobile devices
func DefaultMobileConfig() *Config {
	cfg := &Config{}
	setDefaults()

	dataDir, _ := platform.GetDataDir()
	cacheDir, _ := platform.GetCacheDir()

	cfg.Storage.DatabasePath = filepath.Join(dataDir, "music.db")
	cfg.Storage.CacheDir = cacheDir
	cfg.User.IsAnonymous = true
	cfg.UI.WindowWidth = 400
	cfg.UI.WindowHeight = 800
	cfg.UI.GridColumns = 2

	return cfg
}

func setDefaults() {
	viper.SetDefault("debug", false)

	viper.SetDefault("api.base_url", "https://new.akarpov.ru/api/v1")
	viper.SetDefault("api.rate_limit.requests_per_second", 100)
	viper.SetDefault("api.rate_limit.burst_size", 10)
	viper.SetDefault("api.timeout", 30)
	viper.SetDefault("api.retries", 3)
	viper.SetDefault("api.user_agent", "AMP/1.0.0")

	dataDir, _ := platform.GetDataDir()
	cacheDir, _ := platform.GetCacheDir()

	viper.SetDefault("storage.database_path", filepath.Join(dataDir, "music.db"))
	viper.SetDefault("storage.cache_dir", cacheDir)
	viper.SetDefault("storage.max_cache_size", 1024*1024*1024)
	viper.SetDefault("storage.sync_interval", 300)
	viper.SetDefault("storage.enable_wal", true)

	viper.SetDefault("audio.sample_rate", 44100)
	viper.SetDefault("audio.buffer_size", 8192)
	viper.SetDefault("audio.default_volume", 0.7)
	viper.SetDefault("audio.crossfade", false)

	viper.SetDefault("ui.theme", "dark")
	viper.SetDefault("ui.language", "en")
	viper.SetDefault("ui.show_stats", false)
	viper.SetDefault("ui.grid_columns", 4)
	viper.SetDefault("ui.window_width", 1200)
	viper.SetDefault("ui.window_height", 800)

	viper.SetDefault("search.max_results", 100)
	viper.SetDefault("search.fuzzy_threshold", 0.6)
	viper.SetDefault("search.enable_typing", true)
	viper.SetDefault("search.debounce_ms", 300)

	viper.SetDefault("download.max_concurrent", 3)
	viper.SetDefault("download.chunk_size", 1024*1024)
	viper.SetDefault("download.temp_dir", filepath.Join(cacheDir, "temp"))
	viper.SetDefault("download.auto_download", false)

	viper.SetDefault("user.is_anonymous", true)
}

func ensureDirectories(cfg *Config) error {
	dirs := []string{
		filepath.Dir(cfg.Storage.DatabasePath),
		cfg.Storage.CacheDir,
		cfg.Download.TempDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// Save saves the current configuration to file
func (c *Config) Save() error {
	configDir, err := platform.GetConfigDir()
	if err != nil {
		return err
	}

	configFile := filepath.Join(configDir, "config.yaml")
	return viper.WriteConfigAs(configFile)
}
