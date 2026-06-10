// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration loaded from environment.
type Config struct {
	AppEnv  string
	AppPort string

	// Database
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	DBAutoMigrate bool

	// Migrations
	MigrationsDir string

	// JWT
	JWTSecret      string
	JWTExpireHours int

	// File upload
	UploadDir   string
	MaxUploadMB int
	AllowedExts string

	// Maintenance cleanup
	IdempotencyKeyTTLHours   int
	FileDeletedRetentionDays int
	AuditLogRetentionDays    int
	CleanupBatchSize         int

	// Logging
	LogLevel  string
	LogFormat string

	// HTTP server
	ReadTimeout  int
	WriteTimeout int

	// CORS
	CORSOrigins string
}

// Load reads configuration from .env files and environment variables.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigType("env")
	v.SetConfigName(".env")
	v.AddConfigPath(".")
	v.AddConfigPath("..")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.ReadInConfig()

	v.SetDefault("APP_ENV", "dev")
	v.SetDefault("APP_PORT", "8080")
	v.SetDefault("DB_HOST", "127.0.0.1")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_USER", "app")
	v.SetDefault("DB_NAME", "appdb")
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("DB_AUTO_MIGRATE", true)
	v.SetDefault("MIGRATIONS_DIR", "./migrations")
	v.SetDefault("JWT_EXPIRE_HOURS", 24)
	v.SetDefault("UPLOAD_DIR", "./uploads")
	v.SetDefault("MAX_UPLOAD_MB", 10)
	v.SetDefault("ALLOWED_EXTS", ".jpg,.jpeg,.png,.gif,.xlsx,.csv,.pdf")
	v.SetDefault("IDEMPOTENCY_KEY_TTL_HOURS", 24)
	v.SetDefault("FILE_DELETED_RETENTION_DAYS", 30)
	v.SetDefault("AUDIT_LOG_RETENTION_DAYS", 0)
	v.SetDefault("CLEANUP_BATCH_SIZE", 1000)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "text")
	v.SetDefault("READ_TIMEOUT", 30)
	v.SetDefault("WRITE_TIMEOUT", 30)
	v.SetDefault("CORS_ORIGINS", "*")

	cfg := Config{
		AppEnv:                   v.GetString("APP_ENV"),
		AppPort:                  v.GetString("APP_PORT"),
		DBHost:                   v.GetString("DB_HOST"),
		DBPort:                   v.GetString("DB_PORT"),
		DBUser:                   v.GetString("DB_USER"),
		DBPassword:               v.GetString("DB_PASSWORD"),
		DBName:                   v.GetString("DB_NAME"),
		DBSSLMode:                v.GetString("DB_SSLMODE"),
		DBAutoMigrate:            v.GetBool("DB_AUTO_MIGRATE"),
		MigrationsDir:            v.GetString("MIGRATIONS_DIR"),
		JWTSecret:                v.GetString("JWT_SECRET"),
		JWTExpireHours:           v.GetInt("JWT_EXPIRE_HOURS"),
		UploadDir:                v.GetString("UPLOAD_DIR"),
		MaxUploadMB:              v.GetInt("MAX_UPLOAD_MB"),
		AllowedExts:              v.GetString("ALLOWED_EXTS"),
		IdempotencyKeyTTLHours:   v.GetInt("IDEMPOTENCY_KEY_TTL_HOURS"),
		FileDeletedRetentionDays: v.GetInt("FILE_DELETED_RETENTION_DAYS"),
		AuditLogRetentionDays:    v.GetInt("AUDIT_LOG_RETENTION_DAYS"),
		CleanupBatchSize:         v.GetInt("CLEANUP_BATCH_SIZE"),
		LogLevel:                 v.GetString("LOG_LEVEL"),
		LogFormat:                v.GetString("LOG_FORMAT"),
		ReadTimeout:              v.GetInt("READ_TIMEOUT"),
		WriteTimeout:             v.GetInt("WRITE_TIMEOUT"),
		CORSOrigins:              v.GetString("CORS_ORIGINS"),
	}

	if cfg.DBPassword == "" {
		return Config{}, fmt.Errorf("DB_PASSWORD is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.JWTExpireHours <= 0 {
		return Config{}, fmt.Errorf("JWT_EXPIRE_HOURS must be > 0")
	}

	return cfg, nil
}
