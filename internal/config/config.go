package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
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

// Auth Configuration
type AuthConfig struct {
	JWTSecretEnv           string        `mapstructure:"jwt_secret_env"`
	AccessTokenTTL         time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL        time.Duration `mapstructure:"refresh_token_ttl"`
	MaxFailedLoginAttempts int           `mapstructure:"max_failed_login_attempts"`
	AccountLockDuration    time.Duration `mapstructure:"account_lock_duration"`
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

	// Auth Defaults
	viper.SetDefault("auth.jwt_secret_env", "JWT_SECRET")
	viper.SetDefault("auth.access_token_ttl", "60m")
	viper.SetDefault("auth.refresh_token_ttl", "168h")
	viper.SetDefault("auth.max_failed_login_attempts", 5)
	viper.SetDefault("auth.account_lock_duration", "15m")

	// Environment Variables automatisch binden (Viper Feature)
	viper.AutomaticEnv()
	viper.SetEnvPrefix("OMC") // Environment Variables mit Prefix OMC_

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

// JWT Secret aus Environment Variable laden
func (a *AuthConfig) GetJWTSecret() string {
	envVar := a.JWTSecretEnv
	if envVar == "" {
		envVar = "JWT_SECRET" // Fallback
	}

	secret := os.Getenv(envVar)
	if secret == "" {
		// Development Fallback (MIT WARNING!)
		return "dev-secret-change-in-production-min-32-chars"
	}
	return secret
}

// Helper um zu prüfen ob Production-Ready
func (a *AuthConfig) IsProductionReady() bool {
	secret := a.GetJWTSecret()
	return secret != "dev-secret-change-in-production-min-32-chars" && len(secret) >= 32
}
