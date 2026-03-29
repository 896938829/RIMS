package config

import "testing"

func TestLoadReadsEnvironmentOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "app")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "appdb")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("JWT_SECRET", "jwt-secret")
	t.Setenv("JWT_EXPIRE_HOURS", "12")
	t.Setenv("DB_AUTO_MIGRATE", "true")
	t.Setenv("DEMO_USER", "admin")
	t.Setenv("DEMO_PASSWORD", "admin123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppPort != "9090" {
		t.Fatalf("expected AppPort=9090, got %s", cfg.AppPort)
	}
	if cfg.JWTSecret != "jwt-secret" {
		t.Fatalf("expected JWTSecret=jwt-secret, got %s", cfg.JWTSecret)
	}
	if cfg.JWTExpireHours != 12 {
		t.Fatalf("expected JWTExpireHours=12, got %d", cfg.JWTExpireHours)
	}
	if !cfg.DBAutoMigrate {
		t.Fatalf("expected DBAutoMigrate=true")
	}
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("DEMO_USER", "admin")
	t.Setenv("DEMO_PASSWORD", "admin123")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when JWT_SECRET is empty")
	}
}
