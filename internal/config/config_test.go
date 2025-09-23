package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T){
	// Clear envs that Load reads
	os.Unsetenv("APP_ENV")
	os.Unsetenv("HTTP_PORT")
	os.Unsetenv("DB_PATH")
	os.Unsetenv("DB_DRIVER")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("DB_DSN")
	os.Unsetenv("STATIC_DIR")
	cfg := Load()
	if cfg.Env != "dev" { t.Fatalf("expected dev, got %s", cfg.Env) }
	if cfg.HttpPort != "8080" { t.Fatalf("expected 8080, got %s", cfg.HttpPort) }
	if cfg.DBPath == "" { t.Fatalf("expected default DBPath, got empty") }
	if cfg.DBDriver != "sqlite" { t.Fatalf("expected sqlite, got %s", cfg.DBDriver) }
	if cfg.StaticDir == "" { t.Fatalf("expected StaticDir, got empty") }
}

func TestLoadEnvOverride(t *testing.T){
	os.Setenv("APP_ENV", "prod")
	os.Setenv("HTTP_PORT", "9999")
	os.Setenv("DB_PATH", "/tmp/x.db")
	os.Setenv("DB_DRIVER", "postgres")
	os.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	os.Setenv("STATIC_DIR", "/srv/www")
	t.Cleanup(func(){ os.Unsetenv("APP_ENV"); os.Unsetenv("HTTP_PORT"); os.Unsetenv("DB_PATH"); os.Unsetenv("DB_DRIVER"); os.Unsetenv("DATABASE_URL"); os.Unsetenv("STATIC_DIR") })
	cfg := Load()
	if cfg.Env != "prod" { t.Fatalf("env override failed") }
	if cfg.HttpPort != "9999" { t.Fatalf("port override failed") }
	if cfg.DBDriver != "postgres" { t.Fatalf("driver override failed") }
	if cfg.DBDsn == "" { t.Fatalf("DATABASE_URL should be set") }
	if cfg.StaticDir != "/srv/www" { t.Fatalf("static dir override failed") }
}
