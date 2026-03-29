// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv  string
	AppPort string

	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	DBAutoMigrate bool

	JWTSecret      string
	JWTExpireHours int

	DemoUser     string
	DemoPassword string
}

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
	v.SetDefault("JWT_EXPIRE_HOURS", 24)
	v.SetDefault("DEMO_USER", "admin")
	v.SetDefault("DEMO_PASSWORD", "admin123")

	cfg := Config{
		AppEnv:         v.GetString("APP_ENV"),
		AppPort:        v.GetString("APP_PORT"),
		DBHost:         v.GetString("DB_HOST"),
		DBPort:         v.GetString("DB_PORT"),
		DBUser:         v.GetString("DB_USER"),
		DBPassword:     v.GetString("DB_PASSWORD"),
		DBName:         v.GetString("DB_NAME"),
		DBSSLMode:      v.GetString("DB_SSLMODE"),
		DBAutoMigrate:  v.GetBool("DB_AUTO_MIGRATE"),
		JWTSecret:      v.GetString("JWT_SECRET"),
		JWTExpireHours: v.GetInt("JWT_EXPIRE_HOURS"),
		DemoUser:       v.GetString("DEMO_USER"),
		DemoPassword:   v.GetString("DEMO_PASSWORD"),
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
	if cfg.DemoUser == "" || cfg.DemoPassword == "" {
		return Config{}, fmt.Errorf("DEMO_USER and DEMO_PASSWORD are required")
	}

	return cfg, nil
}
