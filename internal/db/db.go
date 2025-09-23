package db

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arencloud/hermes/internal/config"
	"github.com/arencloud/hermes/internal/logging"
	"github.com/arencloud/hermes/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init(cfg *config.Config, logger logging.Logger) error {
	// Configure GORM to use our structured logger so SQL logs are not plain text
	var gormLevel gormlogger.LogLevel
	switch strings.ToLower(logging.GetLevel()) {
	case "debug":
		gormLevel = gormlogger.Info // log SQL traces at debug level
	case "error", "fatal":
		gormLevel = gormlogger.Error
	default:
		gormLevel = gormlogger.Warn
	}
	gormLogger := newGormLogger(logger, gormLevel)

	var dialector gorm.Dialector
	driver := strings.ToLower(strings.TrimSpace(cfg.DBDriver))
	if driver == "postgres" || driver == "postgresql" {
		// Use PostgreSQL via DATABASE_URL / DB_DSN
		if cfg.DBDsn == "" {
			return &os.PathError{Op: "open", Path: "DATABASE_URL/DB_DSN", Err: os.ErrInvalid}
		}
		dialector = postgres.Open(cfg.DBDsn)
		logger.Info("db connect", "driver", "postgres")
	} else {
		// Default to sqlite
		if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
			return err
		}
		dialector = sqlite.Open(cfg.DBPath)
		logger.Info("db connect", "driver", "sqlite", "path", cfg.DBPath)
	}

	gdb, err := gorm.Open(dialector, &gorm.Config{Logger: gormLogger})
	if err != nil {
		return err
	}
	if err := gdb.AutoMigrate(&models.User{}, &models.Provider{}, &models.Bucket{}, &models.AuthConfig{}, &models.LogEntry{}, &models.TraceRow{}, &models.TraceEventRow{}); err != nil {
		return err
	}
	DB = gdb
	// Hook logging persistence into DB (non-blocking)
	logging.SetPersist(func(e any) error {
		// accept logging.entry via json marshal/unmarshal path
		b, _ := json.Marshal(e)
		var tmp struct {
			Time   time.Time      `json:"time"`
			Level  string         `json:"level"`
			Msg    string         `json:"msg"`
			Fields map[string]any `json:"fields"`
		}
		if err := json.Unmarshal(b, &tmp); err != nil {
			return nil
		}
		fieldsBytes, _ := json.Marshal(tmp.Fields)
		le := models.LogEntry{Time: tmp.Time, Level: tmp.Level, Msg: tmp.Msg, Fields: string(fieldsBytes)}
		return DB.Create(&le).Error
	})
	// Ensure there is exactly one auth config row
	var ac models.AuthConfig
	if err := DB.First(&ac).Error; err != nil {
		ac = models.AuthConfig{Mode: "local", Enabled: false, OIDCScope: "openid email profile", DefaultRole: "viewer"}
		_ = DB.Create(&ac).Error
	}
	// Bootstrap default admin if no users exist
	var count int64
	if err := DB.Model(&models.User{}).Count(&count).Error; err == nil && count == 0 {
		// generate temp password
		tmp := make([]byte, 12)
		if _, err := rand.Read(tmp); err == nil {
			// hex-encode to readable
			tmpPass := hex.EncodeToString(tmp)
			hash, _ := bcrypt.GenerateFromPassword([]byte(tmpPass), bcrypt.DefaultCost)
			admin := models.User{Email: "admin@local", Password: string(hash), Role: "admin", MustChangePassword: true}
			if err := DB.Create(&admin).Error; err == nil {
				logger.Info("default admin created", "email", admin.Email, "tempPassword", tmpPass)
			} else {
				logger.Error("failed to create default admin", "error", err)
			}
		}
	}
	return nil
}
