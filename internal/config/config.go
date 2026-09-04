package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Server struct {
		Port         string
		Host         string
		ReadTimeout  time.Duration
		WriteTimeout time.Duration
	}

	Database struct {
		Host     string
		Port     string
		User     string
		Password string
		DBName   string
		SSLMode  string
	}

	JWT struct {
		Secret        string
		TokenExpiry   time.Duration
		RefreshExpiry time.Duration
	}

	Environment string

	Email struct {
		ResendAPIKey string
		FromEmail    string
		FromName     string
	}
}

func Load() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	config := &Config{}

	config.Server.Port = getEnv("SERVER_PORT", "8080")
	config.Server.Host = getEnv("SERVER_HOST", "localhost")
	config.Server.ReadTimeout = getEnvAsDuration("SERVER_READ_TIMEOUT", 15*time.Second)
	config.Server.WriteTimeout = getEnvAsDuration("SERVER_WRITE_TIMEOUT", 15*time.Second)

	config.Environment = getEnv("ENVIRONMENT", "development")

	config.Database.Host = getEnv("DB_HOST", "localhost")
	config.Database.Port = getEnv("DB_PORT", "5432")
	config.Database.DBName = getEnv("DB_NAME", "invo_db")
	config.Database.SSLMode = getEnv("DB_SSLMODE", "disable")

	if config.Environment == "production" {
		config.Database.User = mustGetEnv("DB_USER")
		config.Database.Password = mustGetEnv("DB_PASSWORD")
		config.JWT.Secret = mustGetEnv("JWT_SECRET")
	} else {
		config.Database.User = getEnv("DB_USER", "user")
		config.Database.Password = getEnv("DB_PASSWORD", "password")
		config.JWT.Secret = getEnv("JWT_SECRET", "dev-only-insecure-secret")
	}

	// The mobile app has no refresh-token flow wired up (long-lived session +
	// biometric lock is used instead), so the access token itself needs to
	// outlive a normal session instead of expiring hourly.
	config.JWT.TokenExpiry = getEnvAsDuration("JWT_TOKEN_EXPIRY", 30*24*time.Hour)
	config.JWT.RefreshExpiry = getEnvAsDuration("JWT_REFRESH_EXPIRY", 90*24*time.Hour)

	config.Email.ResendAPIKey = getEnv("RESEND_API_KEY", "")
	config.Email.FromEmail = getEnv("EMAIL_FROM", "")
	config.Email.FromName = getEnv("EMAIL_FROM_NAME", "Invoice App")

	return config
}

// mustGetEnv fails fast on startup instead of silently running production
// with a default/guessable secret or database credential.
func mustGetEnv(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		log.Fatalf("Missing required environment variable %s in production", key)
	}
	return value
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if valueStr, exists := os.LookupEnv(key); exists {
		if value, err := time.ParseDuration(valueStr); err == nil {
			return value
		}
	}
	return defaultValue
}

func (c *Config) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Database.Host,
		c.Database.Port,
		c.Database.User,
		c.Database.Password,
		c.Database.DBName,
		c.Database.SSLMode,
	)
}

func (c *Config) GetDbUrl() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.Database.User,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.DBName,
		c.Database.SSLMode,
	)
}
