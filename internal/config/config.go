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

	config.Database.Host = getEnv("DB_HOST", "localhost")
	config.Database.Port = getEnv("DB_PORT", "5432")
	config.Database.User = getEnv("DB_USER", "user")
	config.Database.Password = getEnv("DB_PASSWORD", "password")
	config.Database.DBName = getEnv("DB_NAME", "invo_db")
	config.Database.SSLMode = getEnv("DB_SSLMODE", "disable")

	config.JWT.Secret = getEnv("JWT_SECRET", "supersecretkey")
	config.JWT.TokenExpiry = getEnvAsDuration("JWT_TOKEN_EXPIRY", time.Hour)
	config.JWT.RefreshExpiry = getEnvAsDuration("JWT_REFRESH_EXPIRY", 24*time.Hour)

	config.Environment = getEnv("ENVIRONMENT", "development")

	return config
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
