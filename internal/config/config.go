package config

import (
	"os"
)

type Config struct {
	Env       string
	HttpPort  string
	DBPath    string     // used when DBDriver=sqlite
	DBDriver  string     // sqlite|postgres
	DBDsn     string     // used when DBDriver=postgres (e.g., DATABASE_URL)
	StaticDir string
}

func Load() *Config {
	cfg := &Config{
		Env:       getEnv("APP_ENV", "dev"),
		HttpPort:  getEnv("HTTP_PORT", "8080"),
		DBPath:    getEnv("DB_PATH", "data/hermes.db"),
		DBDriver:  getEnv("DB_DRIVER", "sqlite"),
		DBDsn:     getEnv("DATABASE_URL", getEnv("DB_DSN", "")),
		StaticDir: getEnv("STATIC_DIR", "web/dist"),
	}
	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" { return v }
	return def
}
