package config

import (
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/services"
	"os"
)

// ToDatabaseConfig converts Config to database.Config
func (c *Config) ToDatabaseConfig() database.Config {
	return database.Config{
		Host:     c.Database.Host,
		Port:     c.Database.Port,
		Username: c.Database.Username,
		Password: c.Database.Password,
		Database: c.Database.Database,
	}
}

// ToLoggingConfig converts Config to logging.Config
func (c *Config) ToLoggingConfig() logging.Config {
	return logging.Config{
		Level:       c.Logging.Level,
		Output:      os.Stdout,
		Prefix:      c.Logging.Prefix,
		EnableColor: c.Logging.EnableColor,
	}
}

// ToEmailConfig converts Config to services.EmailConfig
func (c *Config) ToEmailConfig() services.EmailConfig {
	return services.EmailConfig{
		SMTPHost:     c.Email.SMTPHost,
		SMTPPort:     c.Email.SMTPPort,
		SMTPUsername: c.Email.SMTPUsername,
		SMTPPassword: c.Email.SMTPPassword,
		FromEmail:    c.Email.FromEmail,
		FromName:     c.Email.FromName,
	}
}

// ShouldLogToFile returns whether file logging is enabled
func (c *Config) ShouldLogToFile() bool {
	return c.Logging.EnableFile
}

// GetLogDir returns the log directory path
func (c *Config) GetLogDir() string {
	return c.Logging.LogDir
}