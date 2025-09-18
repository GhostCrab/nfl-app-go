package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"nfl-app-go/logging"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// Server configuration
	Server ServerConfig `json:"server"`

	// Database configuration
	Database DatabaseConfig `json:"database"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`

	// Email configuration
	Email EmailConfig `json:"email"`

	// Authentication configuration
	Auth AuthConfig `json:"auth"`

	// Application configuration
	App AppConfig `json:"app"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port        string `json:"port"`
	Host        string `json:"host"`
	UseTLS      bool   `json:"use_tls"`
	BehindProxy bool   `json:"behind_proxy"`
	CertFile    string `json:"cert_file"`
	KeyFile     string `json:"key_file"`
	Environment string `json:"environment"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string        `json:"host"`
	Port     string        `json:"port"`
	Username string        `json:"username"`
	Password string        `json:"password"`
	Database string        `json:"database"`
	Timeout  time.Duration `json:"timeout"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level       string `json:"level"`
	Prefix      string `json:"prefix"`
	EnableColor bool   `json:"enable_color"`
}

// EmailConfig holds SMTP configuration
type EmailConfig struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     string `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	FromEmail    string `json:"from_email"`
	FromName     string `json:"from_name"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret string `json:"jwt_secret"`
}

// AppConfig holds application-specific configuration
type AppConfig struct {
	CurrentSeason int  `json:"current_season"`
	IsDevelopment bool `json:"is_development"`
}

// Load loads configuration from environment variables and .env file
func Load() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// Don't treat missing .env as an error
		logging.Warnf("Could not load .env file: %v", err)
	}

	config := &Config{
		Server: ServerConfig{
			Port:        getEnv("SERVER_PORT", "8080"),
			Host:        getEnv("SERVER_HOST", "0.0.0.0"),
			UseTLS:      getBoolEnv("USE_TLS", true),
			BehindProxy: getBoolEnv("BEHIND_PROXY", false),
			CertFile:    getEnv("TLS_CERT_FILE", "server.crt"),
			KeyFile:     getEnv("TLS_KEY_FILE", "server.key"),
			Environment: getEnv("ENVIRONMENT", "development"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "p5server"),
			Port:     getEnv("DB_PORT", "27017"),
			Username: getEnv("DB_USERNAME", "nflapp"),
			Password: getEnv("DB_PASSWORD", ""),
			Database: getEnv("DB_NAME", "nfl_app"),
			Timeout:  getDurationEnv("DB_TIMEOUT", 10*time.Second),
		},
		Logging: LoggingConfig{
			Level:       getEnv("LOG_LEVEL", "debug"),
			Prefix:      getEnv("LOG_PREFIX", "nfl-app"),
			EnableColor: getBoolEnv("LOG_COLOR", true),
		},
		Email: EmailConfig{
			SMTPHost:     getEnv("SMTP_HOST", ""),
			SMTPPort:     getEnv("SMTP_PORT", "587"),
			SMTPUsername: getEnv("SMTP_USERNAME", ""),
			SMTPPassword: getEnv("SMTP_PASSWORD", ""),
			FromEmail:    getEnv("FROM_EMAIL", ""),
			FromName:     getEnv("FROM_NAME", "NFL Games"),
		},
		Auth: AuthConfig{
			JWTSecret: getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
		},
		App: AppConfig{
			CurrentSeason: getIntEnv("CURRENT_SEASON", 2025),
			IsDevelopment: strings.ToLower(getEnv("ENVIRONMENT", "development")) == "development",
		},
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// Validate validates the configuration for required fields and sensible values
func (c *Config) Validate() error {
	// Validate server configuration
	if c.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}

	if c.Server.UseTLS && !c.Server.BehindProxy {
		if c.Server.CertFile == "" || c.Server.KeyFile == "" {
			return fmt.Errorf("TLS certificate and key files are required when USE_TLS=true")
		}

		// Check if certificate files exist
		if _, err := os.Stat(c.Server.CertFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS certificate file not found: %s", c.Server.CertFile)
		}
		if _, err := os.Stat(c.Server.KeyFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS key file not found: %s", c.Server.KeyFile)
		}
	}

	// Validate database configuration
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Port == "" {
		return fmt.Errorf("database port is required")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}

	// Validate authentication
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("JWT secret is required")
	}
	if c.Auth.JWTSecret == "your-secret-key-change-in-production" && !c.App.IsDevelopment {
		return fmt.Errorf("JWT secret must be changed in production")
	}

	// Validate app configuration
	if c.App.CurrentSeason < 2020 || c.App.CurrentSeason > 2030 {
		return fmt.Errorf("current season must be between 2020 and 2030, got: %d", c.App.CurrentSeason)
	}

	return nil
}

// IsEmailConfigured returns true if email service is configured
func (c *Config) IsEmailConfigured() bool {
	return c.Email.SMTPHost != "" &&
		c.Email.SMTPUsername != "" &&
		c.Email.SMTPPassword != "" &&
		c.Email.FromEmail != ""
}

// GetServerAddress returns the full server address
func (c *Config) GetServerAddress() string {
	return c.Server.Host + ":" + c.Server.Port
}

// GetMongoURI returns the MongoDB connection URI
func (c *Config) GetMongoURI() string {
	if c.Database.Username != "" && c.Database.Password != "" {
		return fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=%s",
			c.Database.Username, c.Database.Password,
			c.Database.Host, c.Database.Port,
			c.Database.Database, c.Database.Database)
	}
	return fmt.Sprintf("mongodb://%s:%s/%s",
		c.Database.Host, c.Database.Port, c.Database.Database)
}

// LogConfiguration logs the current configuration (without sensitive data)
func (c *Config) LogConfiguration() {
	logging.Info("=== Application Configuration ===")
	logging.Infof("Server: %s (TLS: %t, Behind Proxy: %t, Environment: %s)",
		c.GetServerAddress(), c.Server.UseTLS, c.Server.BehindProxy, c.Server.Environment)
	logging.Infof("Database: %s:%s/%s (Username: %s, Auth: %t)",
		c.Database.Host, c.Database.Port, c.Database.Database,
		c.Database.Username, c.Database.Password != "")
	logging.Infof("Logging: Level=%s, Prefix=%s, Color=%t",
		c.Logging.Level, c.Logging.Prefix, c.Logging.EnableColor)
	logging.Infof("Email: Configured=%t, Host=%s, From=%s",
		c.IsEmailConfigured(), c.Email.SMTPHost, c.Email.FromEmail)
	logging.Infof("App: Season=%d, Development=%t",
		c.App.CurrentSeason, c.App.IsDevelopment)
	logging.Info("================================")
}

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
