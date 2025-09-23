package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/arencloud/hermes/internal/config"
	"github.com/arencloud/hermes/internal/logging"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

// Note: no go:embed for assets; we serve from disk only.

type statusRecorder struct {
	http.ResponseWriter
	code  int
	bytes int64
}

func (sr *statusRecorder) WriteHeader(statusCode int) {
	sr.code = statusCode
	sr.ResponseWriter.WriteHeader(statusCode)
}
func (sr *statusRecorder) Write(b []byte) (int, error) {
	n, err := sr.ResponseWriter.Write(b)
	sr.bytes += int64(n)
	return n, err
}

func Router(cfg *config.Config, logger logging.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, AllowedHeaders: []string{"*"}}))
	// simple global request counter (observability)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddUint64(&totalRequests, 1)
			next.ServeHTTP(w, r)
		})
	})
	// tracing middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newTraceID()
			u := currentUser(r)
			t := &Trace{ID: id, Method: r.Method, Path: r.URL.Path, Started: time.Now(), Events: []TraceEvent{}}
			if u != nil {
				t.UserEmail = u.Email
				t.UserRole = u.Role
			}
			t.UserAgent = r.UserAgent()
			if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
				t.RemoteIP = ip
			} else {
				t.RemoteIP = r.RemoteAddr
			}
			if r.ContentLength > 0 {
				t.ReqBytes = r.ContentLength
			}
			w.Header().Set("X-Trace-Id", id)
			w.Header().Set("X-Request-Id", id)
			addEvent(r, "request.start", map[string]any{"method": r.Method, "path": r.URL.Path})
			r = r.WithContext(withTraceCtx(r.Context(), t))
			rec := &statusRecorder{ResponseWriter: w, code: 200}
			next.ServeHTTP(rec, r)
			t.Status = rec.code
			t.Ended = time.Now()
			t.Duration = t.Ended.Sub(t.Started)
			t.RespBytes = rec.bytes
			addEvent(r, "request.end", map[string]any{"status": rec.code, "respBytes": rec.bytes})
			// observability counters
			if t.ReqBytes > 0 {
				atomic.AddUint64(&bytesIn, uint64(t.ReqBytes))
			}
			if t.RespBytes > 0 {
				atomic.AddUint64(&bytesOut, uint64(t.RespBytes))
			}
			atomic.AddUint64(&totalDurationNs, uint64(t.Duration))
			if t.Status >= 500 {
				atomic.AddUint64(&total5xx, 1)
			} else if t.Status >= 400 {
				atomic.AddUint64(&total4xx, 1)
			}
			traces.add(t)
			// persist trace to DB for durability
			persistTrace(t)
			// emit structured request log
			logger.Info("http_request",
				"method", t.Method,
				"path", t.Path,
				"status", t.Status,
				"durationMs", float64(t.Duration)/1e6,
				"user", t.UserEmail,
				"role", t.UserRole,
				"traceId", t.ID,
				"bytesIn", t.ReqBytes,
				"bytesOut", t.RespBytes,
			)
		})
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	// API placeholder groups
	r.Route("/api", func(r chi.Router) {
		r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"hermes","version":"0.1.0"}`))
		})
		r.Route("/v1", func(r chi.Router) {
			registerAPI(r, logger)
		})
	})

	// Serve raw image assets from project /img path under /img URL prefix
	r.Handle("/img/*", http.StripPrefix("/img/", http.FileServer(http.Dir("img"))))

	// Static from disk (no embed). If not found, serve index.html for SPA routing
	fs := http.FileServer(http.Dir(cfg.StaticDir))
	r.Handle("/*", spaHandler(cfg.StaticDir, fs, logger))
	return r
}

type spa struct {
	dir    string
	next   http.Handler
	logger logging.Logger
}

func spaHandler(dir string, next http.Handler, logger logging.Logger) http.Handler {
	return &spa{dir: dir, next: next, logger: logger}
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(s.dir, r.URL.Path)
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		s.next.ServeHTTP(w, r)
		return
	}
	// fallback to index.html
	http.ServeFile(w, r, filepath.Join(s.dir, "index.html"))
}
