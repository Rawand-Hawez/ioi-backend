package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	Postgres  PostgresConfig
	GoTrue    GoTrueConfig
	Dragonfly DragonflyConfig
	MinIO     MinIOConfig
	API       APIConfig
}

// PostgresConfig holds PostgreSQL configuration
type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
}

// GoTrueConfig holds GoTrue authentication configuration
type GoTrueConfig struct {
	JWTSecret string
	SiteURL   string
}

// DragonflyConfig holds Dragonfly (Redis) configuration
type DragonflyConfig struct {
	Host     string
	Port     string
	Password string
}

// MinIOConfig holds MinIO storage configuration
type MinIOConfig struct {
	Host      string
	Port      string
	AccessKey string
	SecretKey string
}

// APIConfig holds API server configuration
type APIConfig struct {
	FiberPort   string
	RESTPort    string
	CORSOrigins string
}

var appConfig *Config

// Load initializes the configuration from environment variables
func Load() {
	// Load .env file if it exists (fails gracefully in Docker)
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found; using environment variables from system")
	}

	appConfig = &Config{
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     getEnvRequired("POSTGRES_USER"),
			Password: getEnvRequired("POSTGRES_PASSWORD"),
			Database: getEnv("POSTGRES_DB", "app_database"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		GoTrue: GoTrueConfig{
			JWTSecret: getEnvRequired("GOTRUE_JWT_SECRET"),
			SiteURL:   getEnv("GOTRUE_SITE_URL", "http://localhost:3000"),
		},
		Dragonfly: DragonflyConfig{
			Host:     getEnv("DRAGONFLY_HOST", "localhost"),
			Port:     getEnv("DRAGONFLY_PORT", "6379"),
			Password: getEnv("DRAGONFLY_PASSWORD", ""),
		},
		MinIO: MinIOConfig{
			Host:      getEnv("MINIO_HOST", "localhost"),
			Port:      getEnv("MINIO_PORT", "9000"),
			AccessKey: getEnv("MINIO_ROOT_USER", "minioadmin"),
			SecretKey: getEnv("MINIO_ROOT_PASSWORD", "minioadmin"),
		},
		API: APIConfig{
			FiberPort:   getEnv("API_PORT_FIBER", "8080"),
			RESTPort:    getEnv("API_PORT_REST", "3000"),
			CORSOrigins: getEnv("CORS_ALLOWED_ORIGINS", "*"),
		},
	}

	// Validate configuration
	if err := appConfig.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	log.Println("Configuration loaded and validated successfully")
}

// Get returns the loaded configuration
func Get() *Config {
	return appConfig
}

// Validate checks that all required configuration values are present and valid
func (c *Config) Validate() error {
	// Validate PostgreSQL configuration
	if c.Postgres.User == "" {
		return fmt.Errorf("POSTGRES_USER is required")
	}
	if c.Postgres.Password == "" {
		return fmt.Errorf("POSTGRES_PASSWORD is required")
	}

	// Validate GoTrue JWT secret
	if c.GoTrue.JWTSecret == "" {
		return fmt.Errorf("GOTRUE_JWT_SECRET is required")
	}
	if len(c.GoTrue.JWTSecret) < 32 {
		return fmt.Errorf("GOTRUE_JWT_SECRET must be at least 32 characters for security")
	}

	// Validate port numbers
	if !isValidPort(c.Postgres.Port) {
		return fmt.Errorf("invalid POSTGRES_PORT: %s", c.Postgres.Port)
	}
	if !isValidPort(c.API.FiberPort) {
		return fmt.Errorf("invalid API_PORT_FIBER: %s", c.API.FiberPort)
	}
	if !isValidPort(c.API.RESTPort) {
		return fmt.Errorf("invalid API_PORT_REST: %s", c.API.RESTPort)
	}

	return nil
}

// GetPostgresDSN returns the PostgreSQL connection string
func (c *Config) GetPostgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.Postgres.User,
		c.Postgres.Password,
		c.Postgres.Host,
		c.Postgres.Port,
		c.Postgres.Database,
		c.Postgres.SSLMode,
	)
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvRequired gets an environment variable that must be set
func getEnvRequired(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("FATAL: %s environment variable is required but not set", key)
	}
	return value
}

// isValidPort checks if a string is a valid port number
func isValidPort(port string) bool {
	p, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return p > 0 && p <= 65535
}
