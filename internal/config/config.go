package config

import (
	"fmt"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	ServiceName     string        `yaml:"service_name" env-default:"gateway-service"`
	Env             string        `yaml:"env" env-default:"local"`
	Version         string        `yaml:"version" env-default:"dev"`
	LogLevel        string        `yaml:"log_level" env-default:"info"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env-default:"10s"`
	CatalogGRPC     CatalogGRPC   `yaml:"catalog_grpc"`
	HTTP            HTTPConfig    `yaml:"http"`
	OTLP            OTLPConfig    `yaml:"otlp"`
}

type CatalogGRPC struct {
	Addr    string        `yaml:"addr" env-default:"localhost:9091"`
	Timeout time.Duration `yaml:"timeout" env-default:"5s"`
}

type HTTPConfig struct {
	Port    int           `yaml:"port" env-default:"8082"`
	Timeout time.Duration `yaml:"timeout" env-default:"5s"`
}

type OTLPConfig struct {
	Endpoint string `yaml:"endpoint" env:"OTLP_ENDPOINT" env-default:"jaeger:4317"`
}

// Load reads config from YAML and validates the result.
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config path is empty")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", path)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func MustLoad(path string) *Config {
	var cfg Config
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		panic("failed to load config: " + err.Error())
	}
	return &cfg
}

// MustLoadByPath keeps bootstrap code short in main().
func MustLoadByPath(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}

	return cfg
}

func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.Env == "" {
		return fmt.Errorf("env is required")
	}
	if c.HTTP.Port <= 0 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port is required")
	}
	if c.HTTP.Timeout <= 0 {
		return fmt.Errorf("http.timeout must be > 0")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown_timeout must be > 0")
	}
	if c.OTLP.Endpoint == "" {
		return fmt.Errorf("otlp.endpoint is required")
	}

	return nil
}

func (c Config) HTTPAddr() string {
	return fmt.Sprintf(":%d", c.HTTP.Port)
}
