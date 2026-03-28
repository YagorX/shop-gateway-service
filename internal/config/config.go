package config

import (
	"flag"
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

	HTTPTLS      HTTPTLSConfig `yaml:"http_tls"`
	HTTP         HTTPConfig    `yaml:"http"`
	CatalogGRPC  GRPCConfig    `yaml:"catalog_grpc"`
	AuthGRPC     GRPCConfig    `yaml:"auth_grpc"`
	OTLP         OTLPConfig    `yaml:"otlp"`
	AuthTLS      TLSConfig     `yaml:"auth_tls"`
	Swagger      SwaggerConfig `yaml:"swagger"`
	TemplatePath string        `yaml:"template_path" env-default:""`
}

type SwaggerConfig struct {
	UIPath   string `yaml:"ui_path" env-default:""`
	SpecPath string `yaml:"spec_path" env-default:""`
}

type HTTPConfig struct {
	Port    int           `yaml:"port" env-default:"8083"`
	Timeout time.Duration `yaml:"timeout" env-default:"5s"`
}

type HTTPTLSConfig struct {
	Enabled  bool   `yaml:"enabled" env-default:"false"`
	CertFile string `yaml:"cert_file" env-default:""`
	KeyFile  string `yaml:"key_file" env-default:""`
}

type GRPCConfig struct {
	Addr    string        `yaml:"addr"`
	Timeout time.Duration `yaml:"timeout" env-default:"3s"`
}

type OTLPConfig struct {
	Endpoint string `yaml:"endpoint" env-default:"localhost:4317"`
}

type TLSConfig struct {
	Enabled        bool   `yaml:"enabled" env-default:"false"`
	CAFile         string `yaml:"ca_file" env-default:""`
	ServerName     string `yaml:"server_name" env-default:""`
	ClientCertFile string `yaml:"client_cert_file" env-default:""`
	ClientKeyFile  string `yaml:"client_key_file" env-default:""`
}

func MustLoad() *Config {
	path := fetchConfigPath()
	if path == "" {
		panic("config path is empty")
	}

	return mustLoadByPath(path)
}

func MustLoadByPath(path string) *Config {
	return mustLoadByPath(path)
}

func mustLoadByPath(path string) *Config {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		panic("config file does not exist: " + path)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		panic("failed to read config: " + err.Error())
	}

	if err := cfg.Validate(); err != nil {
		panic("invalid config: " + err.Error())
	}

	return &cfg
}

func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.Env == "" {
		return fmt.Errorf("env is required")
	}
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.LogLevel == "" {
		return fmt.Errorf("log_level is required")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown_timeout must be > 0")
	}

	if c.HTTP.Port <= 0 {
		return fmt.Errorf("http.port must be > 0")
	}
	if c.HTTP.Timeout <= 0 {
		return fmt.Errorf("http.timeout must be > 0")
	}

	if c.CatalogGRPC.Addr == "" {
		return fmt.Errorf("catalog_grpc.addr is required")
	}
	if c.CatalogGRPC.Timeout <= 0 {
		return fmt.Errorf("catalog_grpc.timeout must be > 0")
	}

	if c.AuthGRPC.Addr == "" {
		return fmt.Errorf("auth_grpc.addr is required")
	}
	if c.AuthGRPC.Timeout <= 0 {
		return fmt.Errorf("auth_grpc.timeout must be > 0")
	}

	if c.OTLP.Endpoint == "" {
		return fmt.Errorf("otlp.endpoint is required")
	}

	if c.AuthTLS.Enabled {
		if c.AuthTLS.CAFile == "" {
			return fmt.Errorf("auth_tls.ca_file is required when auth_tls.enabled=true")
		}
		if c.AuthTLS.ServerName == "" {
			return fmt.Errorf("auth_tls.server_name is required when auth_tls.enabled=true")
		}
		if c.AuthTLS.ClientCertFile == "" {
			return fmt.Errorf("auth_tls.client_cert_file is required when auth_tls.enabled=true")
		}
		if c.AuthTLS.ClientKeyFile == "" {
			return fmt.Errorf("auth_tls.client_key_file is required when auth_tls.enabled=true")
		}
	}

	if c.HTTPTLS.Enabled {
		if c.HTTPTLS.CertFile == "" {
			return fmt.Errorf("http_tls.cert_file is required when http_tls.enabled=true")
		}
		if c.HTTPTLS.KeyFile == "" {
			return fmt.Errorf("http_tls.key_file is required when http_tls.enabled=true")
		}
	}

	return nil
}

func (c *Config) HTTPAddr() string {
	return fmt.Sprintf(":%d", c.HTTP.Port)
}

func fetchConfigPath() string {
	var path string

	flag.StringVar(&path, "config", "", "path to config file")
	flag.Parse()

	if path == "" {
		path = os.Getenv("CONFIG_PATH")
	}

	return path
}
