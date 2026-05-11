package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
}

type ServerConfig struct {
	Host          string `yaml:"host"`
	HTTPPort      int    `yaml:"http_port"`
	TCPPort       int    `yaml:"tcp_port"`
	UDPPort       int    `yaml:"udp_port"`
	GRPCPort      int    `yaml:"grpc_port"`
	WebSocketPort int    `yaml:"websocket_port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:          "localhost",
			HTTPPort:      8080,
			TCPPort:       9090,
			UDPPort:       9091,
			GRPCPort:      9092,
			WebSocketPort: 9093,
		},
		Database: DatabaseConfig{
			Path: "./data/mangahub.db",
		},
		Auth: AuthConfig{
			JWTSecret: "mangahub-default-secret-change-me",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
