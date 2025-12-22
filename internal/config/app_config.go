package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	AppPort string
	AppEnv  string
	AppURL  string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	DBMigrate  bool

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	JWTSecret string
	JWTExp    int

	StorageCDNURL     string
	StorageMode       string
	StorageAttachment string
	StorageProfile    string

	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Endpoint  string
}

func LoadAppConfig() *AppConfig {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from system environment variables")
	}

	return &AppConfig{
		AppPort: mustGetEnv("APP_PORT"),
		AppEnv:  mustGetEnv("APP_ENV"),
		AppURL:  getEnv("APP_URL", "http://localhost:8080"),

		DBHost:     mustGetEnv("DB_HOST"),
		DBPort:     mustGetEnv("DB_PORT"),
		DBUser:     mustGetEnv("DB_USER"),
		DBPassword: mustGetEnv("DB_PASSWORD"),
		DBName:     mustGetEnv("DB_NAME"),
		DBSSLMode:  mustGetEnv("DB_SSLMODE"),
		DBMigrate:  mustGetEnvAsBool("DB_MIGRATE"),

		GoogleClientID:     mustGetEnv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: mustGetEnv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  mustGetEnv("GOOGLE_REDIRECT_URL"),

		JWTSecret: mustGetEnv("JWT_SECRET"),
		JWTExp:    mustGetEnvAsInt("JWT_EXP"),

		StorageCDNURL:     getEnv("STORAGE_CDN_URL", ""),
		StorageMode:       mustGetEnv("STORAGE_MODE"),
		StorageAttachment: mustGetEnv("STORAGE_ATTACHMENT"),
		StorageProfile:    mustGetEnv("STORAGE_PROFILE"),

		S3Bucket:    getEnv("S3_BUCKET", ""),
		S3Region:    getEnv("S3_REGION", ""),
		S3AccessKey: getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey: getEnv("S3_SECRET_KEY", ""),
		S3Endpoint:  getEnv("S3_ENDPOINT", ""),
	}
}

func (c *AppConfig) DBConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func mustGetEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		log.Fatalf("Environment variable %s is required but not set", key)
	}
	return value
}

func mustGetEnvAsBool(key string) bool {
	valStr := mustGetEnv(key)
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		log.Fatalf("Environment variable %s must be a boolean (true/false), got: %s", key, valStr)
	}
	return val
}

func mustGetEnvAsInt(key string) int {
	valStr := mustGetEnv(key)
	val, err := strconv.Atoi(valStr)
	if err != nil {
		log.Fatalf("Environment variable %s must be an integer, got: %s", key, valStr)
	}
	return val
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
