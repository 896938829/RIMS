package db

import (
	"testing"

	"rims-go/internal/config"
)

func TestBuildDSN(t *testing.T) {
	cfg := config.Config{
		DBHost:     "127.0.0.1",
		DBPort:     "5432",
		DBUser:     "app",
		DBPassword: "secret",
		DBName:     "appdb",
		DBSSLMode:  "disable",
	}
	got := BuildDSN(cfg)
	want := "host=127.0.0.1 port=5432 user=app password=secret dbname=appdb sslmode=disable"
	if got != want {
		t.Fatalf("BuildDSN() = %q, want %q", got, want)
	}
}
