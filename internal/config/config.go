package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Modbus   ModbusConfig   `mapstructure:"modbus"`
	Devices  DevicesConfig  `mapstructure:"device_profiles"`
}

type ServerConfig struct {
	GRPCPort        int           `mapstructure:"grpc_port"`
	HTTPPort        int           `mapstructure:"http_port"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	Database       string `mapstructure:"database"`
	User           string `mapstructure:"user"`
	Password       string `mapstructure:"password"`
	MaxConnections int    `mapstructure:"max_connections"`
}

type ModbusConfig struct {
	DefaultTimeout      time.Duration `mapstructure:"default_timeout"`
	DefaultPollInterval time.Duration `mapstructure:"default_poll_interval"`
}

type DevicesConfig struct {
	SearchPaths []string `mapstructure:"search_paths"`
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// Defaults setzen
	viper.SetDefault("server.grpc_port", 50051)
	viper.SetDefault("server.http_port", 8080)
	viper.SetDefault("server.shutdown_timeout", "30s")
	viper.SetDefault("modbus.default_timeout", "1s")
	viper.SetDefault("modbus.default_poll_interval", "100ms")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Database)
}
