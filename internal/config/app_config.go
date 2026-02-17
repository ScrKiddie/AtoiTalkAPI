package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	AppPort               string
	AppEnv                string
	AppURL                string
	AppCorsAllowedOrigins []string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	DBMigrate  bool

	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	JWTSecret string
	JWTExp    int

	S3BucketPublic  string
	S3BucketPrivate string
	S3Region        string
	S3AccessKey     string
	S3SecretKey     string
	S3Endpoint      string
	S3PublicDomain  string

	SMTPHost      string
	SMTPPort      int
	SMTPUser      string
	SMTPPassword  string
	SMTPFromEmail string
	SMTPFromName  string
	SMTPAsync     bool

	OTPExp              int
	OTPRateLimitSeconds int

	OTPSecret string

	TurnstileSecretKey string

	SoftDeleteRetentionDays int
	MediaRetentionDays      float64

	EntityCleanupCron      string
	PrivateChatCleanupCron string
	MediaCleanupCron       string
}

func LoadAppConfig() *AppConfig {
	if err := godotenv.Load(); err != nil {
		slog.Info("No .env file found, reading from system environment variables")
	}

	return &AppConfig{
		AppPort:               mustGetEnv("APP_PORT"),
		AppEnv:                mustGetEnv("APP_ENV"),
		AppURL:                getEnv("APP_URL", "http://localhost:8080"),
		AppCorsAllowedOrigins: strings.Split(getEnv("APP_CORS_ALLOWED_ORIGINS", "*"), ","),

		DBHost:     mustGetEnv("DB_HOST"),
		DBPort:     mustGetEnv("DB_PORT"),
		DBUser:     mustGetEnv("DB_USER"),
		DBPassword: mustGetEnv("DB_PASSWORD"),
		DBName:     mustGetEnv("DB_NAME"),
		DBSSLMode:  mustGetEnv("DB_SSLMODE"),
		DBMigrate:  mustGetEnvAsBool("DB_MIGRATE"),

		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		GoogleClientID:     mustGetEnv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: mustGetEnv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  mustGetEnv("GOOGLE_REDIRECT_URL"),

		JWTSecret: mustGetEnv("JWT_SECRET"),
		JWTExp:    mustGetEnvAsInt("JWT_EXP"),

		S3BucketPublic:  mustGetEnv("S3_BUCKET_PUBLIC"),
		S3BucketPrivate: mustGetEnv("S3_BUCKET_PRIVATE"),
		S3Region:        getEnv("S3_REGION", ""),
		S3AccessKey:     getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:     getEnv("S3_SECRET_KEY", ""),
		S3Endpoint:      getEnv("S3_ENDPOINT", ""),
		S3PublicDomain:  getEnv("S3_PUBLIC_DOMAIN", ""),

		SMTPHost:      getEnv("SMTP_HOST", ""),
		SMTPPort:      getEnvAsInt("SMTP_PORT", 587),
		SMTPUser:      getEnv("SMTP_USER", ""),
		SMTPPassword:  getEnv("SMTP_PASSWORD", ""),
		SMTPFromEmail: getEnv("SMTP_FROM_EMAIL", ""),
		SMTPFromName:  getEnv("SMTP_FROM_NAME", ""),
		SMTPAsync:     getEnvAsBool("SMTP_ASYNC", true),

		OTPExp:              getEnvAsInt("OTP_EXP", 300),
		OTPRateLimitSeconds: getEnvAsInt("OTP_RATE_LIMIT_SECONDS", 60),
		OTPSecret:           mustGetEnv("OTP_SECRET"),

		TurnstileSecretKey: getEnv("TURNSTILE_SECRET_KEY", ""),

		SoftDeleteRetentionDays: getEnvAsInt("SOFT_DELETE_RETENTION_DAYS", 30),
		MediaRetentionDays:      getEnvAsFloat("MEDIA_RETENTION_DAYS", 7.0),

		EntityCleanupCron:      getEnv("ENTITY_CLEANUP_CRON", "0 2 * * *"),
		PrivateChatCleanupCron: getEnv("PRIVATE_CHAT_CLEANUP_CRON", "30 2 * * *"),
		MediaCleanupCron:       getEnv("MEDIA_CLEANUP_CRON", "0 3 * * *"),
	}
}

func (c *AppConfig) DBConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func mustGetEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		slog.Error("Environment variable is required but not set", "key", key)
		os.Exit(1)
	}
	return value
}

func mustGetEnvAsBool(key string) bool {
	valStr := mustGetEnv(key)
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		slog.Error("Environment variable must be a boolean (true/false)", "key", key, "value", valStr)
		os.Exit(1)
	}
	return val
}

func mustGetEnvAsInt(key string) int {
	valStr := mustGetEnv(key)
	val, err := strconv.Atoi(valStr)
	if err != nil {
		slog.Error("Environment variable must be an integer", "key", key, "value", valStr)
		os.Exit(1)
	}
	return val
}

func getEnvAsFloat(key string, fallback float64) float64 {
	valStr, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		slog.Warn("Environment variable must be a float, using fallback", "key", key, "value", valStr, "fallback", fallback)
		return fallback
	}
	return val
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valStr, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		slog.Warn("Environment variable must be an integer, using fallback", "key", key, "value", valStr, "fallback", fallback)
		return fallback
	}
	return val
}

func getEnvAsBool(key string, fallback bool) bool {
	valStr, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		slog.Warn("Environment variable must be a boolean, using fallback", "key", key, "value", valStr, "fallback", fallback)
		return fallback
	}
	return val
}
