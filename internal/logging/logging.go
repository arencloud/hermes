package logging

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ConfigFromEnv builds a zap configuration from environment variables.
//
// Supported env vars:
// - HERMES_LOG_LEVEL: debug|info|warn|error (default: info)
// - HERMES_LOG_FORMAT: json|console (default: json)
// - HERMES_LOG_TIME: rfc3339|epoch (default: rfc3339)
func ConfigFromEnv() zap.Config {
	levelStr := strings.ToLower(strings.TrimSpace(os.Getenv("HERMES_LOG_LEVEL")))
	if levelStr == "" {
		levelStr = "info"
	}
	var lvl zapcore.Level
	switch levelStr {
	case "debug":
		lvl = zapcore.DebugLevel
	case "warn", "warning":
		lvl = zapcore.WarnLevel
	case "error":
		lvl = zapcore.ErrorLevel
	default:
		lvl = zapcore.InfoLevel
	}

	format := strings.ToLower(strings.TrimSpace(os.Getenv("HERMES_LOG_FORMAT")))
	if format == "" {
		format = "json"
	}

	timeFmt := strings.ToLower(strings.TrimSpace(os.Getenv("HERMES_LOG_TIME")))
	if timeFmt == "" {
		timeFmt = "rfc3339"
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.MessageKey = "msg"
	encCfg.LevelKey = "level"
	encCfg.TimeKey = "ts"
	encCfg.CallerKey = "caller"
	encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	encCfg.EncodeTime = func(t time.Time, pae zapcore.PrimitiveArrayEncoder) {
		if timeFmt == "epoch" {
			pae.AppendFloat64(float64(t.UnixNano()) / 1e9)
			return
		}
		pae.AppendString(t.UTC().Format(time.RFC3339Nano))
	}
	encCfg.EncodeCaller = zapcore.ShortCallerEncoder

	return zap.Config{
		Level:            zap.NewAtomicLevelAt(lvl),
		Development:      false,
		Encoding:         format,
		EncoderConfig:    encCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		InitialFields:    map[string]interface{}{},
	}
}

// NewLogger initializes a zap.Logger based on ConfigFromEnv.
func NewLogger() (*zap.Logger, error) {
	cfg := ConfigFromEnv()
	// Ensure JSON encoder is used even if Encoding says json; Config.Build respects it.
	logger, err := cfg.Build(zap.AddStacktrace(zapcore.ErrorLevel))
	if err != nil {
		return nil, err
	}
	return logger, nil
}
