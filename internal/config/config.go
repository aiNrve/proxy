package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

const (
	defaultPort                = 8080
	defaultLogLevel            = "info"
	defaultConfigPath          = "./ainrve.yaml"
	defaultHealthCheckInterval = 30
)

// ProviderConfig defines per-provider config used by routing and adapters.
type ProviderConfig struct {
	Enabled bool    `mapstructure:"enabled" yaml:"enabled"`
	APIKey  string  `mapstructure:"api_key" yaml:"api_key"`
	BaseURL string  `mapstructure:"base_url" yaml:"base_url"`
	Weight  float64 `mapstructure:"weight" yaml:"weight"`
}

// RoutingConfig defines how provider selection should work.
type RoutingConfig struct {
	DefaultStrategy  string            `mapstructure:"default_strategy" yaml:"default_strategy"`
	TaskOverrides    map[string]string `mapstructure:"task_overrides" yaml:"task_overrides"`
	FallbackProvider string            `mapstructure:"fallback_provider" yaml:"fallback_provider"`
	WeightCost       float64           `mapstructure:"weight_cost" yaml:"weight_cost"`
	WeightLatency    float64           `mapstructure:"weight_latency" yaml:"weight_latency"`
}

// Config is the full runtime configuration consumed by the proxy.
type Config struct {
	Port                int                       `mapstructure:"port" yaml:"port"`
	LogLevel            string                    `mapstructure:"log_level" yaml:"log_level"`
	DatabaseURL         string                    `mapstructure:"database_url" yaml:"database_url"`
	AiNrveAPIKey        string                    `mapstructure:"ainrve_api_key" yaml:"ainrve_api_key"`
	HealthCheckInterval int                       `mapstructure:"health_check_interval" yaml:"health_check_interval"`
	Providers           map[string]ProviderConfig `mapstructure:"providers" yaml:"providers"`
	Routing             RoutingConfig             `mapstructure:"routing" yaml:"routing"`
}

// Loader manages live configuration state and hot reload behavior.
type Loader struct {
	mu  sync.RWMutex
	cfg Config
	vp  *viper.Viper
}

// NewLoader initializes a Loader from file + environment and enables hot reload.
func NewLoader(configPath string) (*Loader, error) {
	vp := viper.New()
	configureViper(vp, configPath)

	l := &Loader{vp: vp}
	if err := l.Reload(); err != nil {
		return nil, err
	}

	vp.OnConfigChange(func(_ fsnotify.Event) {
		_ = l.Reload()
	})
	vp.WatchConfig()

	return l, nil
}

// Reload refreshes runtime config from disk + environment.
func (l *Loader) Reload() error {
	if err := readConfigFile(l.vp); err != nil {
		return err
	}

	cfg := Config{}
	if err := l.vp.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	cfg = normalize(cfg)
	if err := validate(cfg); err != nil {
		return err
	}

	l.mu.Lock()
	l.cfg = cfg
	l.mu.Unlock()

	return nil
}

// Get returns a copy of the current configuration.
func (l *Loader) Get() Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return cloneConfig(l.cfg)
}

func configureViper(vp *viper.Viper, configPath string) {
	if strings.TrimSpace(configPath) == "" {
		if envPath := strings.TrimSpace(os.Getenv("CONFIG_PATH")); envPath != "" {
			configPath = envPath
		} else {
			configPath = defaultConfigPath
		}
	}

	vp.SetConfigFile(configPath)
	vp.SetConfigType("yaml")

	vp.SetDefault("port", defaultPort)
	vp.SetDefault("log_level", defaultLogLevel)
	vp.SetDefault("health_check_interval", defaultHealthCheckInterval)
	vp.SetDefault("routing.default_strategy", "balanced")
	vp.SetDefault("routing.weight_cost", 0.5)
	vp.SetDefault("routing.weight_latency", 0.5)
	vp.SetDefault("routing.task_overrides", map[string]string{})
	vp.SetDefault("providers", map[string]ProviderConfig{})

	vp.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vp.AutomaticEnv()

	_ = vp.BindEnv("port", "PORT")
	_ = vp.BindEnv("log_level", "LOG_LEVEL")
	_ = vp.BindEnv("database_url", "DATABASE_URL")
	_ = vp.BindEnv("ainrve_api_key", "AINRVE_API_KEY")
	_ = vp.BindEnv("health_check_interval", "HEALTH_CHECK_INTERVAL")

	bindProviderEnv(vp, "openai", "OPENAI_API_KEY")
	bindProviderEnv(vp, "anthropic", "ANTHROPIC_API_KEY")
	bindProviderEnv(vp, "groq", "GROQ_API_KEY")
	bindProviderEnv(vp, "together", "TOGETHER_API_KEY")
	bindProviderEnv(vp, "gemini", "GEMINI_API_KEY")
}

func bindProviderEnv(vp *viper.Viper, provider, keyEnv string) {
	_ = vp.BindEnv(fmt.Sprintf("providers.%s.api_key", provider), keyEnv)
}

func readConfigFile(vp *viper.Viper) error {
	if err := vp.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if pathErr, ok := err.(*os.PathError); ok && errors.Is(pathErr.Err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config file: %w", err)
	}
	return nil
}

func normalize(cfg Config) Config {
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	if cfg.HealthCheckInterval <= 0 {
		cfg.HealthCheckInterval = defaultHealthCheckInterval
	}

	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}

	for name, baseURL := range providerDefaults() {
		provider := cfg.Providers[name]
		if provider.BaseURL == "" {
			provider.BaseURL = baseURL
		}
		if provider.Weight == 0 {
			provider.Weight = 1
		}
		if provider.APIKey != "" && !provider.Enabled {
			provider.Enabled = true
		}
		cfg.Providers[name] = provider
	}

	if cfg.Routing.DefaultStrategy == "" {
		cfg.Routing.DefaultStrategy = "balanced"
	}
	if cfg.Routing.TaskOverrides == nil {
		cfg.Routing.TaskOverrides = map[string]string{}
	}
	if cfg.Routing.WeightCost == 0 && cfg.Routing.WeightLatency == 0 {
		cfg.Routing.WeightCost = 0.5
		cfg.Routing.WeightLatency = 0.5
	}

	return cfg
}

func validate(cfg Config) error {
	strategy := strings.ToLower(cfg.Routing.DefaultStrategy)
	switch strategy {
	case "cost", "latency", "balanced":
	default:
		return fmt.Errorf("invalid routing.default_strategy %q", cfg.Routing.DefaultStrategy)
	}

	totalWeight := cfg.Routing.WeightCost + cfg.Routing.WeightLatency
	if totalWeight <= 0 {
		return errors.New("routing weights must be greater than zero")
	}

	return nil
}

func providerDefaults() map[string]string {
	return map[string]string{
		"openai":    "https://api.openai.com",
		"anthropic": "https://api.anthropic.com",
		"groq":      "https://api.groq.com",
		"together":  "https://api.together.ai",
		"gemini":    "https://generativelanguage.googleapis.com",
	}
}

func cloneConfig(cfg Config) Config {
	cloned := cfg
	cloned.Providers = map[string]ProviderConfig{}
	for k, v := range cfg.Providers {
		cloned.Providers[k] = v
	}
	cloned.Routing.TaskOverrides = map[string]string{}
	for k, v := range cfg.Routing.TaskOverrides {
		cloned.Routing.TaskOverrides[k] = v
	}
	return cloned
}

// ResolveConfigPath resolves config paths for diagnostics and startup logs.
func ResolveConfigPath(path string) string {
	if strings.TrimSpace(path) == "" {
		path = defaultConfigPath
	}
	if strings.TrimSpace(os.Getenv("CONFIG_PATH")) != "" {
		path = os.Getenv("CONFIG_PATH")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
