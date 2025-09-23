package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/arencloud/hermes/internal/db"
	"github.com/arencloud/hermes/internal/models"
	"github.com/go-chi/chi/v5"
)

// Lightweight in-memory tracing
// Each request will have a Trace with Events. Stored in a ring buffer.

type TraceEvent struct {
	Time   time.Time      `json:"time"`
	Name   string         `json:"name"`
	Fields map[string]any `json:"fields,omitempty"`
}

type Trace struct {
	ID        string         `json:"id"`
	Method    string         `json:"method"`
	Path      string         `json:"path"`
	Status    int            `json:"status"`
	UserEmail string         `json:"userEmail,omitempty"`
	UserRole  string         `json:"userRole,omitempty"`
	UserAgent string         `json:"userAgent,omitempty"`
	RemoteIP  string         `json:"remoteIp,omitempty"`
	ReqBytes  int64          `json:"reqBytes,omitempty"`
	RespBytes int64          `json:"respBytes,omitempty"`
	Started   time.Time      `json:"started"`
	Ended     time.Time      `json:"ended"`
	Duration  time.Duration  `json:"duration"`
	Tags      map[string]any `json:"tags,omitempty"`
	Events    []TraceEvent   `json:"events"`
}

type traceStore struct {
	mu   sync.RWMutex
	buf  []*Trace
	next int
	size int
}

var traces = &traceStore{buf: make([]*Trace, 1000), size: 1000}

func (s *traceStore) add(t *Trace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf[s.next] = t
	s.next = (s.next + 1) % s.size
}

func (s *traceStore) all(limit int) []*Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > s.size {
		limit = s.size
	}
	out := make([]*Trace, 0, limit)
	// walk ring newest-first
	idx := (s.next - 1 + s.size) % s.size
	for i := 0; i < s.size && len(out) < limit; i++ {
		if s.buf[idx] != nil {
			out = append(out, s.buf[idx])
		}
		idx = (idx - 1 + s.size) % s.size
	}
	return out
}

// persistTrace stores the trace and its events into the database so they survive restarts.
func persistTrace(t *Trace) {
	if t == nil || db.DB == nil {
		return
	}
	row := models.TraceRow{
		ID:         t.ID,
		Method:     t.Method,
		Path:       t.Path,
		Status:     t.Status,
		UserEmail:  t.UserEmail,
		UserRole:   t.UserRole,
		UserAgent:  t.UserAgent,
		RemoteIP:   t.RemoteIP,
		ReqBytes:   t.ReqBytes,
		RespBytes:  t.RespBytes,
		Started:    t.Started,
		Ended:      t.Ended,
		DurationNs: int64(t.Duration),
	}
	_ = db.DB.Save(&row).Error
	// insert events
	for _, ev := range t.Events {
		fieldsBytes, _ := json.Marshal(ev.Fields)
		_ = db.DB.Create(&models.TraceEventRow{TraceID: t.ID, Time: ev.Time, Name: ev.Name, Fields: string(fieldsBytes)}).Error
	}
}

// Context helpers

type ctxKey int

const traceKey ctxKey = 1

func traceFrom(ctx context.Context) *Trace {
	if v := ctx.Value(traceKey); v != nil {
		if t, ok := v.(*Trace); ok {
			return t
		}
	}
	return nil
}

func withTraceCtx(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, traceKey, t)
}

func newTraceID() string { b := make([]byte, 8); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func addEvent(r *http.Request, name string, fields map[string]any) {
	if t := traceFrom(r.Context()); t != nil {
		t.Events = append(t.Events, TraceEvent{Time: time.Now(), Name: name, Fields: fields})
	}
}

// respondError records an error event into the current trace and writes an HTTP error.
func respondError(w http.ResponseWriter, r *http.Request, code int, msg string) {
	addEvent(r, "error", map[string]any{"code": code, "message": msg})
	http.Error(w, msg, code)
}

// HTTP Handlers for trace API

func traceRecent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		// ignore parse errors for simplicity
	}
	// Prefer DB-backed recent traces for durability
	var rows []models.TraceRow
	_ = db.DB.Order("started desc").Limit(limit).Find(&rows).Error
	out := make([]*Trace, 0, len(rows))
	for _, r0 := range rows {
		out = append(out, &Trace{ID: r0.ID, Method: r0.Method, Path: r0.Path, Status: r0.Status, UserEmail: r0.UserEmail, UserRole: r0.UserRole, UserAgent: r0.UserAgent, RemoteIP: r0.RemoteIP, ReqBytes: r0.ReqBytes, RespBytes: r0.RespBytes, Started: r0.Started, Ended: r0.Ended, Duration: time.Duration(r0.DurationNs)})
	}
	json.NewEncoder(w).Encode(out)
}

func traceGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	// Load trace from DB with events
	var tr models.TraceRow
	if err := db.DB.First(&tr, "id = ?", id).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	var evs []models.TraceEventRow
	_ = db.DB.Where("trace_id = ?", id).Order("time asc").Find(&evs).Error
	out := &Trace{ID: tr.ID, Method: tr.Method, Path: tr.Path, Status: tr.Status, UserEmail: tr.UserEmail, UserRole: tr.UserRole, UserAgent: tr.UserAgent, RemoteIP: tr.RemoteIP, ReqBytes: tr.ReqBytes, RespBytes: tr.RespBytes, Started: tr.Started, Ended: tr.Ended, Duration: time.Duration(tr.DurationNs)}
	for _, e := range evs {
		var f map[string]any
		if e.Fields != "" {
			_ = json.Unmarshal([]byte(e.Fields), &f)
		}
		out.Events = append(out.Events, TraceEvent{Time: e.Time, Name: e.Name, Fields: f})
	}
	json.NewEncoder(w).Encode(out)
}
