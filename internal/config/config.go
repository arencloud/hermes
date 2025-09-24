package config

import (
	"os"
)

type Config struct {
	Env                 string
	HttpPort            string
	DBPath              string     // used when DBDriver=sqlite
	DBDriver            string     // sqlite|postgres
	DBDsn               string     // used when DBDriver=postgres (e.g., DATABASE_URL)
	StaticDir           string
	MaxUploadSizeBytes  int64      // 0 = unlimited
}

func Load() *Config {
	cfg := &Config{
		Env:       getEnv("APP_ENV", "dev"),
		HttpPort:  getEnv("HTTP_PORT", "8080"),
		DBPath:    getEnv("DB_PATH", "data/hermes.db"),
		DBDriver:  getEnv("DB_DRIVER", "sqlite"),
		DBDsn:     getEnv("DATABASE_URL", getEnv("DB_DSN", "")),
		StaticDir: getEnv("STATIC_DIR", "web/dist"),
		MaxUploadSizeBytes: getEnvInt64("MAX_UPLOAD_SIZE_BYTES", 0),
	}
	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" { return v }
	return def
}

func getEnvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		var out int64
		for i := 0; i < len(v); i++ { // simple base-10 parse without importing strconv
			c := v[i]
			if c < '0' || c > '9' { out = def; goto done }
			out = out*10 + int64(c-'0')
		}
		return out
	}
	return def
	done:
	return def
}
