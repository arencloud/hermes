package logging

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
	Fatal(msg string, kv ...any)
}

type entry struct {
	Time   time.Time         `json:"time"`
	Level  string            `json:"level"`
	Msg    string            `json:"msg"`
	Fields map[string]any    `json:"fields,omitempty"`
}

type stdLogger struct {
	json bool
	mu   sync.Mutex
}

var (
	bufMu   sync.RWMutex
	recent  = make([]*entry, 1000)
	nextIdx = 0
	// global log level (debug|info|error|fatal)
	levelMu   sync.RWMutex
	logLevel  = "info"
	// live subscribers for streaming
	subMu      sync.RWMutex
	subscribers = map[chan *entry]struct{}{}
	// optional persistence hook
	persistMu sync.RWMutex
	persistFn func(any) error
)

// New creates a logger; honors env vars LOG_LEVEL (debug|info|error), LOG_JSON (true|false).
func New(env string) Logger {
	lvl := os.Getenv("LOG_LEVEL")
	if lvl == "" { lvl = "info" }
	SetLevel(lvl)
	j := true
	if v := os.Getenv("LOG_JSON"); v == "false" { j = false }
	return &stdLogger{json: j}
}

// Allow external packages to register a persistence callback
func SetPersist(fn func(any) error) {
	persistMu.Lock(); defer persistMu.Unlock()
	persistFn = fn
}

// Level control
func SetLevel(lvl string) {
	levelMu.Lock(); defer levelMu.Unlock()
	switch lvl {
	case "debug", "info", "error", "fatal":
		logLevel = lvl
	default:
		logLevel = "info"
	}
}

func GetLevel() string { levelMu.RLock(); defer levelMu.RUnlock(); return logLevel }

func shouldLog(lvl string) bool {
	levelMu.RLock(); defer levelMu.RUnlock()
	order := map[string]int{"debug":0, "info":1, "error":2, "fatal":3}
	cur := order[logLevel]
	lv := order[lvl]
	return lv >= cur
}

func broadcast(e *entry){
	subMu.RLock(); defer subMu.RUnlock()
	for ch := range subscribers {
		select { case ch <- e: default: /* drop if slow */ }
	}
}

func appendBuf(e *entry){
	bufMu.Lock();
	recent[nextIdx] = e
	nextIdx = (nextIdx + 1) % len(recent)
	bufMu.Unlock()
	broadcast(e)
	// persist asynchronously if configured
	persistMu.RLock(); fn := persistFn; persistMu.RUnlock()
	if fn != nil { go fn(e) }
}

func fieldsFromKV(kv []any) map[string]any {
	if len(kv) == 0 { return nil }
	m := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		if i+1 >= len(kv) { break }
		k, ok := kv[i].(string)
		if !ok { continue }
		m[k] = kv[i+1]
	}
	return m
}

func (l *stdLogger) write(level, msg string, kv ...any) {
	if !shouldLog(level) { return }
	e := &entry{Time: time.Now(), Level: level, Msg: msg, Fields: fieldsFromKV(kv)}
	appendBuf(e)
	l.mu.Lock(); defer l.mu.Unlock()
	if l.json {
		b, _ := json.Marshal(e)
		log.Println(string(b))
		return
	}
	// text fallback
	args := []any{"["+e.Time.Format(time.RFC3339)+"]", level+":", msg}
	for k, v := range e.Fields { args = append(args, k, v) }
	log.Println(args...)
}

func (l *stdLogger) Debug(msg string, kv ...any) { l.write("debug", msg, kv...) }
func (l *stdLogger) Info(msg string, kv ...any)  { l.write("info", msg, kv...) }
func (l *stdLogger) Error(msg string, kv ...any) { l.write("error", msg, kv...) }
func (l *stdLogger) Fatal(msg string, kv ...any) { l.write("fatal", msg, kv...); os.Exit(1) }

// Recent returns up to n most recent log entries (newest-first).
func Recent(n int) []*entry {
	bufMu.RLock(); defer bufMu.RUnlock()
	if n <= 0 || n > len(recent) { n = len(recent) }
	out := make([]*entry, 0, n)
	i := (nextIdx - 1 + len(recent)) % len(recent)
	for c := 0; c < len(recent) && len(out) < n; c++ {
		if recent[i] != nil { out = append(out, recent[i]) }
		i = (i - 1 + len(recent)) % len(recent)
	}
	return out
}

// Subscribe returns a channel that will receive new log entries. Call the returned cancel func to unsubscribe.
func Subscribe() (<-chan *entry, func()) {
	ch := make(chan *entry, 100)
	subMu.Lock(); subscribers[ch] = struct{}{}; subMu.Unlock()
	cancel := func(){ subMu.Lock(); delete(subscribers, ch); close(ch); subMu.Unlock() }
	return ch, cancel
}
