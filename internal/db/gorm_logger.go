package db

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/arencloud/hermes/internal/logging"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gormJSONLogger implements gorm's logger.Interface and forwards entries to our
// structured logger so SQL logs are not printed as plain text.
// It intentionally avoids color/format strings and emits fields instead.

type gormJSONLogger struct {
	l     logging.Logger
	level logger.LogLevel
}

func newGormLogger(l logging.Logger, lvl logger.LogLevel) *gormJSONLogger {
	return &gormJSONLogger{l: l, level: lvl}
}

// LogMode updates the log level and returns the logger
func (g *gormJSONLogger) LogMode(l logger.LogLevel) logger.Interface { g.level = l; return g }

func (g *gormJSONLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if g.level < logger.Info {
		return
	}
	g.l.Info("gorm", "msg", msg, "args", toIfaceSlice(data))
}

func (g *gormJSONLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if g.level < logger.Warn {
		return
	}
	g.l.Error("gorm_warn", "msg", msg, "args", toIfaceSlice(data))
}

func (g *gormJSONLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if g.level < logger.Error {
		return
	}
	g.l.Error("gorm_error", "msg", msg, "args", toIfaceSlice(data))
}

// Trace logs each SQL statement with duration, rows affected, and optional error.
func (g *gormJSONLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if g.level <= 0 {
		return
	} // silent
	sql, rows := fc()
	dur := time.Since(begin)
	// Extract caller file:line using runtime; avoid depending on gorm internals
	caller := callerFileLine()
	op, table := summarizeSQL(sql)
	fields := []any{"op", op, "table", table, "rows", rows, "durationMs", float64(dur) / 1e6, "caller", caller}
	// Never include raw SQL to avoid leaking sensitive data
	fields = append(fields, "sqlMasked", true)
	if err != nil {
		// Demote record-not-found to debug and mark notFound=true
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fields = append(fields, "notFound", true)
			if g.level >= logger.Info {
				g.l.Debug("gorm_sql", fields...)
			}
			return
		}
		fields = append(fields, "error", err.Error())
	}
	// Level routing
	if err != nil && g.level >= logger.Error {
		g.l.Error("gorm_sql", fields...)
		return
	}
	if g.level >= logger.Info {
		g.l.Debug("gorm_sql", fields...)
	}
}

func toIfaceSlice(v []interface{}) []any {
	out := make([]any, len(v))
	for i := range v {
		out[i] = v[i]
	}
	return out
}

// callerFileLine returns a best-effort caller file:line string outside of GORM internals.
func callerFileLine() string {
	for i := 2; i < 12; i++ {
		if _, file, line, ok := runtime.Caller(i); ok {
			if !strings.Contains(file, "gorm.io") {
				return file + ":" + itoa(line)
			}
		}
	}
	return ""
}

// fast itoa for small ints
func itoa(i int) string {
	return strconv.Itoa(i)
}

// compactWS compacts whitespace in SQL so the log field stays readable/compact
func compactWS(s string) string {
	b := strings.Builder{}
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// summarizeSQL returns a masked, friendly summary like "SELECT users" or "UPDATE providers" without parameters.
func summarizeSQL(sql string) (op string, table string) {
	q := strings.ToUpper(compactWS(sql))
	parts := strings.Fields(q)
	if len(parts) == 0 {
		return "", ""
	}
	op = parts[0]
	// naive table extraction tolerant to leading operation
	s := q
	if strings.HasPrefix(s, "UPDATE ") {
		// UPDATE <table> SET ...
		s = s[len("UPDATE "):]
	} else if strings.HasPrefix(s, "INSERT INTO ") {
		// INSERT INTO <table> (...)
		s = s[len("INSERT INTO "):]
	} else if strings.HasPrefix(s, "DELETE FROM ") {
		// DELETE FROM <table> ...
		s = s[len("DELETE FROM "):]
	} else if idx := strings.Index(s, " FROM "); idx >= 0 {
		s = s[idx+6:]
	} else if idx := strings.Index(s, " INTO "); idx >= 0 {
		s = s[idx+6:]
	}
	// table name is the next word (strip quotes/backticks)
	ws := strings.Fields(s)
	if len(ws) > 0 {
		table = strings.Trim(ws[0], "`\"")
	}
	return op, strings.ToLower(table)
}
