package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/arencloud/hermes/internal/db"
	"github.com/arencloud/hermes/internal/logging"
	"github.com/arencloud/hermes/internal/models"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type apiServer struct{ logger logging.Logger }

var appStart = time.Now()
var totalRequests uint64
var total4xx uint64
var total5xx uint64
var bytesIn uint64
var bytesOut uint64
var totalDurationNs uint64

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	uptime := time.Since(appStart).Seconds()
	tr := atomic.LoadUint64(&totalRequests)
	dn := atomic.LoadUint64(&totalDurationNs)
	avgMs := 0.0
	if tr > 0 {
		avgMs = float64(dn) / float64(tr) / 1e6
	}
	json.NewEncoder(w).Encode(map[string]any{
		"uptimeSec":     uptime,
		"uptimeHuman":   (time.Duration(uptime) * time.Second).String(),
		"startedAt":     appStart.Format(time.RFC3339),
		"goroutines":    runtime.NumGoroutine(),
		"heapAlloc":     m.HeapAlloc,
		"heapSys":       m.HeapSys,
		"lastGCUnix":    m.LastGC,
		"gcNum":         m.NumGC,
		"totalRequests": tr,
		"total4xx":      atomic.LoadUint64(&total4xx),
		"total5xx":      atomic.LoadUint64(&total5xx),
		"bytesIn":       atomic.LoadUint64(&bytesIn),
		"bytesOut":      atomic.LoadUint64(&bytesOut),
		"avgDurationMs": avgMs,
	})
}

// errorsHandler returns recent traces with errors (status >= 400) and the last error event message.
func errorsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// fetch recent error traces from DB (status>=400)
	var trs []models.TraceRow
	if err := db.DB.Where("status >= ?", 400).Order("started desc").Limit(200).Find(&trs).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := make([]map[string]any, 0, len(trs))
	for _, t := range trs {
		// find last error event for this trace
		var ev models.TraceEventRow
		_ = db.DB.Where("trace_id = ? AND name = ?", t.ID, "error").Order("time desc").First(&ev).Error
		msg := ""
		if ev.Fields != "" {
			var f map[string]any
			_ = json.Unmarshal([]byte(ev.Fields), &f)
			if s, ok := f["message"].(string); ok {
				msg = s
			}
		}
		out = append(out, map[string]any{
			"id":         t.ID,
			"method":     t.Method,
			"path":       t.Path,
			"status":     t.Status,
			"durationMs": float64(t.DurationNs) / 1e6,
			"userEmail":  t.UserEmail,
			"message":    msg,
			"started":    t.Started,
		})
	}
	json.NewEncoder(w).Encode(out)
}

// logsRecent returns recent structured logs; now sourced from DB to survive restarts.
func logsRecent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			limit = i
		}
	}
	var rows []models.LogEntry
	if err := db.DB.Order("time desc").Limit(limit).Find(&rows).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Decode fields JSON into maps to keep UI compatibility
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var f map[string]any
		if r.Fields != "" {
			_ = json.Unmarshal([]byte(r.Fields), &f)
		}
		out = append(out, map[string]any{"time": r.Time, "level": r.Level, "msg": r.Msg, "fields": f})
	}
	json.NewEncoder(w).Encode(out)
}

// logsDownload returns recent logs as NDJSON for easy download
func logsDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	limit := 1000
	if v := r.URL.Query().Get("limit"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			limit = i
		}
	}
	enc := json.NewEncoder(w)
	for _, e := range logging.Recent(limit) {
		_ = enc.Encode(e)
	}
}

// logsGetLevel returns current log level
func logsGetLevel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"level": logging.GetLevel()})
}

// logsSetLevel updates global log level
func logsSetLevel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var in struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if in.Level == "" {
		http.Error(w, "level required", 400)
		return
	}
	logging.SetLevel(in.Level)
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "level": logging.GetLevel()})
}

// logsStream streams logs via Server-Sent Events
func logsStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	// optional level filter
	qLevel := r.URL.Query().Get("level")
	write := func(e any) {
		b, _ := json.Marshal(e)
		w.Write([]byte("data: "))
		w.Write(b)
		w.Write([]byte("\n\n"))
		fl.Flush()
	}
	// send a small backlog first
	for _, e := range logging.Recent(50) {
		if qLevel == "" || e.Level == qLevel {
			write(e)
		}
	}
	ch, cancel := logging.Subscribe()
	defer cancel()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if qLevel == "" || e.Level == qLevel {
				write(e)
			}
		}
	}
}

// obsSummary returns aggregated observability insights computed from in-memory traces.
func obsSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Use DB-backed traces for aggregation so data survives restarts
	var trs []models.TraceRow
	_ = db.DB.Order("started desc").Limit(500).Find(&trs).Error
	lat := make([]float64, 0, len(trs))
	statusCounts := map[string]int{"2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0}
	// per-minute buckets (last 12 minutes)
	now := time.Now().UTC()
	buckets := map[int64]*struct {
		Count  int `json:"count"`
		Errors int `json:"errors"`
	}{}
	for i := 0; i < 12; i++ {
		m := now.Add(-time.Duration(i) * time.Minute).Truncate(time.Minute).Unix()
		buckets[m] = &struct {
			Count  int `json:"count"`
			Errors int `json:"errors"`
		}{0, 0}
	}
	// per-path aggregates
	type pAgg struct {
		Count      int
		SumMs      float64
		Lats       []float64
		Errs       int
		LastMsg    string
		LastStatus int
		SampleID   string
	}
	paths := map[string]*pAgg{}
	for _, t := range trs {
		ms := float64(t.DurationNs) / 1e6
		if ms < 0 {
			ms = 0
		}
		lat = append(lat, ms)
		// status bucket
		s := t.Status
		sKey := "2xx"
		if s >= 500 {
			sKey = "5xx"
		} else if s >= 400 {
			sKey = "4xx"
		} else if s >= 300 {
			sKey = "3xx"
		}
		statusCounts[sKey]++
		// per-minute
		m := t.Started.UTC().Truncate(time.Minute).Unix()
		if b, ok := buckets[m]; ok {
			b.Count++
			if t.Status >= 400 {
				b.Errors++
			}
		}
		// per-path
		pa := paths[t.Path]
		if pa == nil {
			pa = &pAgg{}
			paths[t.Path] = pa
		}
		pa.Count++
		pa.SumMs += ms
		if len(pa.Lats) < 100 {
			pa.Lats = append(pa.Lats, ms)
		}
		if t.Status >= 400 {
			pa.Errs++
			// find last error message from events table
			var ev models.TraceEventRow
			if err := db.DB.Where("trace_id = ? AND name = ?", t.ID, "error").Order("time desc").First(&ev).Error; err == nil {
				if ev.Fields != "" {
					var f map[string]any
					_ = json.Unmarshal([]byte(ev.Fields), &f)
					if v, ok := f["message"].(string); ok {
						pa.LastMsg = v
					}
				}
			}
			pa.LastStatus = t.Status
			if pa.SampleID == "" {
				pa.SampleID = t.ID
			}
		}
	}
	// helpers
	percentile := func(vals []float64, p float64) float64 {
		if len(vals) == 0 {
			return 0
		}
		vv := append([]float64(nil), vals...)
		// simple sort
		for i := 1; i < len(vv); i++ {
			x := vv[i]
			j := i - 1
			for j >= 0 && vv[j] > x {
				vv[j+1] = vv[j]
				j--
			}
			vv[j+1] = x
		}
		idx := int(p / 100.0 * float64(len(vv)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(vv) {
			idx = len(vv) - 1
		}
		return vv[idx]
	}
	// build topSlow and topErrors
	topSlow := make([]map[string]any, 0)
	topErrors := make([]map[string]any, 0)
	for path, ag := range paths {
		if ag.Count == 0 {
			continue
		}
		avg := ag.SumMs / float64(ag.Count)
		p95 := percentile(ag.Lats, 95)
		if ag.Count >= 3 {
			topSlow = append(topSlow, map[string]any{"path": path, "count": ag.Count, "avgMs": avg, "p95Ms": p95})
		}
		if ag.Errs > 0 {
			topErrors = append(topErrors, map[string]any{"path": path, "count": ag.Errs, "lastMessage": ag.LastMsg, "lastStatus": ag.LastStatus, "sampleTraceId": ag.SampleID})
		}
	}
	// sort slices (simple bubble-like selection due to no extra imports)
	sortBy := func(arr []map[string]any, key string) {
		for i := 0; i < len(arr); i++ {
			mi := i
			for j := i + 1; j < len(arr); j++ {
				if arr[j][key].(float64) > arr[mi][key].(float64) {
					mi = j
				}
			}
			arr[i], arr[mi] = arr[mi], arr[i]
		}
	}
	// sort by p95 descending
	sortBy(topSlow, "p95Ms")
	if len(topSlow) > 5 {
		topSlow = topSlow[:5]
	}
	// for errors, we want count desc; normalize count to float
	for _, it := range topErrors {
		if c, ok := it["count"].(int); ok {
			it["count"] = float64(c)
		}
	}
	sortBy(topErrors, "count")
	if len(topErrors) > 8 {
		topErrors = topErrors[:8]
	}
	// per-minute array sorted ascending by time
	perMinute := make([]map[string]any, 0, len(buckets))
	for ts, b := range buckets {
		perMinute = append(perMinute, map[string]any{"ts": ts, "count": b.Count, "errors": b.Errors})
	}
	// simple ascending sort by ts
	for i := 0; i < len(perMinute); i++ {
		mi := i
		for j := i + 1; j < len(perMinute); j++ {
			if perMinute[j]["ts"].(int64) < perMinute[mi]["ts"].(int64) {
				mi = j
			}
		}
		perMinute[i], perMinute[mi] = perMinute[mi], perMinute[i]
	}
	json.NewEncoder(w).Encode(map[string]any{
		"recentLatencies": lat,
		"statusCounts":    statusCounts,
		"perMinute":       perMinute,
		"topSlow":         topSlow,
		"topErrors":       topErrors,
	})
}

func openapiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Minimal OpenAPI 3.0 spec describing the primary Hermes API endpoints
	spec := map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]any{"title": "Hermes API", "version": "0.1.0", "description": "S3-compatible storage manager API (Providers, Buckets, Objects, Users, Auth, Observability, Tracing, Logging)"},
		"servers": []any{map[string]any{"url": "/api/v1"}},
		"paths": map[string]any{
			"/auth/login": map[string]any{"post": map[string]any{"summary": "Login", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string"}, "password": map[string]any{"type": "string"}}, "required": []any{"email", "password"}}}}}, "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
			"/auth/me":    map[string]any{"get": map[string]any{"summary": "Current user", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
			"/providers": map[string]any{
				"get":  map[string]any{"summary": "List providers", "responses": map[string]any{"200": map[string]any{"description": "OK"}}},
				"post": map[string]any{"summary": "Create provider", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Provider"}}}}, "responses": map[string]any{"201": map[string]any{"description": "Created"}}},
			},
			"/providers/{id}": map[string]any{
				"parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer"}}},
				"get":        map[string]any{"summary": "Get provider", "responses": map[string]any{"200": map[string]any{"description": "OK"}}},
				"put":        map[string]any{"summary": "Update provider", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Provider"}}}}, "responses": map[string]any{"200": map[string]any{"description": "OK"}}},
				"delete":     map[string]any{"summary": "Delete provider", "responses": map[string]any{"204": map[string]any{"description": "No Content"}}},
			},
			"/providers/{id}/buckets": map[string]any{
				"get":  map[string]any{"summary": "List buckets", "responses": map[string]any{"200": map[string]any{"description": "OK"}}},
				"post": map[string]any{"summary": "Create bucket", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "region": map[string]any{"type": "string"}}, "required": []any{"name"}}}}}, "responses": map[string]any{"201": map[string]any{"description": "Created"}}},
			},
			"/providers/{id}/buckets/{name}/objects": map[string]any{
				"get":    map[string]any{"summary": "List objects", "parameters": []any{map[string]any{"name": "prefix", "in": "query", "schema": map[string]any{"type": "string"}}, map[string]any{"name": "recursive", "in": "query", "schema": map[string]any{"type": "boolean"}}}, "responses": map[string]any{"200": map[string]any{"description": "OK"}}},
				"delete": map[string]any{"summary": "Delete object", "parameters": []any{map[string]any{"name": "key", "in": "query", "required": true, "schema": map[string]any{"type": "string"}}}, "responses": map[string]any{"204": map[string]any{"description": "No Content"}}},
			},
			"/providers/{id}/buckets/{name}/upload": map[string]any{
				"post": map[string]any{"summary": "Upload object", "requestBody": map[string]any{"required": true, "content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"file": map[string]any{"type": "string", "format": "binary"}, "key": map[string]any{"type": "string"}}, "required": []any{"file"}}}}}, "responses": map[string]any{"200": map[string]any{"description": "OK"}}},
			},
			"/providers/{id}/buckets/{name}/download": map[string]any{"get": map[string]any{"summary": "Download object", "parameters": []any{map[string]any{"name": "key", "in": "query", "required": true, "schema": map[string]any{"type": "string"}}}, "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
			"/providers/{id}/buckets/{name}/copy":     map[string]any{"post": map[string]any{"summary": "Copy object", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"srcKey": map[string]any{"type": "string"}, "dstBucket": map[string]any{"type": "string"}, "dstKey": map[string]any{"type": "string"}, "dstProviderId": map[string]any{"type": "integer"}}, "required": []any{"srcKey", "dstBucket"}}}}}, "responses": map[string]any{"200": map[string]any{"description": "OK (NDJSON progress)"}}}},
			"/providers/{id}/buckets/{name}/move":     map[string]any{"post": map[string]any{"summary": "Move object", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"srcKey": map[string]any{"type": "string"}, "dstBucket": map[string]any{"type": "string"}, "dstKey": map[string]any{"type": "string"}, "dstProviderId": map[string]any{"type": "integer"}}, "required": []any{"srcKey", "dstBucket"}}}}}, "responses": map[string]any{"200": map[string]any{"description": "OK (NDJSON progress)"}}}},
			"/users/":                                 map[string]any{"get": map[string]any{"summary": "List users (admin)", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}, "post": map[string]any{"summary": "Create user (admin)", "responses": map[string]any{"201": map[string]any{"description": "Created"}}}},
			"/users/{id}":                             map[string]any{"put": map[string]any{"summary": "Update user (admin)", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}, "delete": map[string]any{"summary": "Delete user (admin)", "responses": map[string]any{"204": map[string]any{"description": "No Content"}}}},
			"/obs/metrics":                            map[string]any{"get": map[string]any{"summary": "Server metrics", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
			"/obs/summary":                            map[string]any{"get": map[string]any{"summary": "Observability summary", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
			"/obs/errors":                             map[string]any{"get": map[string]any{"summary": "Recent error traces", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
			"/trace/recent":                           map[string]any{"get": map[string]any{"summary": "Recent traces", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
			"/trace/{id}":                             map[string]any{"get": map[string]any{"summary": "Trace detail", "parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}, "responses": map[string]any{"200": map[string]any{"description": "OK"}}}},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"Provider": map[string]any{"type": "object", "properties": map[string]any{
					"id":        map[string]any{"type": "integer"},
					"name":      map[string]any{"type": "string"},
					"type":      map[string]any{"type": "string"},
					"endpoint":  map[string]any{"type": "string"},
					"accessKey": map[string]any{"type": "string"},
					"secretKey": map[string]any{"type": "string"},
					"region":    map[string]any{"type": "string"},
					"useSSL":    map[string]any{"type": "boolean"},
				}, "required": []any{"name", "endpoint"}},
			},
		},
	}
	json.NewEncoder(w).Encode(spec)
}

func registerAPI(r chi.Router, logger logging.Logger) {
	s := &apiServer{logger: logger}
	registerAuth(r, logger)
	// protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(requireAuth)
		// observability (lightweight metrics), visible to any authenticated user
		pr.Get("/obs/metrics", metricsHandler)
		pr.Get("/obs/errors", errorsHandler)
		pr.Get("/obs/summary", obsSummary)
		// OpenAPI (Swagger) spec — restricted to editor/admin
		pr.With(requireEditorOrAdmin).Get("/openapi.json", openapiHandler)
		// tracing endpoints
		pr.Get("/trace/recent", traceRecent)
		pr.Get("/trace/{id}", traceGet)
		// logging endpoints
		pr.Get("/logs/recent", logsRecent)
		pr.Get("/logs/download", logsDownload)
		pr.Get("/logs/level", logsGetLevel)
		pr.Put("/logs/level", logsSetLevel)
		pr.Get("/logs/stream", logsStream)
		pr.Route("/users", func(r chi.Router) {
			r.Use(requireAdmin)
			r.Get("/", s.listUsers)
			r.Post("/", s.createUser)
			r.Put("/{id}", s.updateUser)
			r.Delete("/{id}", s.deleteUser)
		})
		registerProviders(pr)
		registerBuckets(pr)
	})
}

func (s *apiServer) listUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var users []models.User
	if err := db.DB.Find(&users).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(users)
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func (s *apiServer) createUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if in.Email == "" || !emailRe.MatchString(in.Email) {
		http.Error(w, "invalid email", 400)
		return
	}
	if len(in.Password) < 8 {
		http.Error(w, "password too short", 400)
		return
	}
	role := in.Role
	if role == "" {
		role = "admin"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to hash password", 500)
		return
	}
	u := models.User{Email: in.Email, Password: string(hash), Role: role}
	if err := db.DB.Create(&u).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(u)
}

func (s *apiServer) updateUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid user id", 400)
		return
	}
	var u models.User
	if err := db.DB.First(&u, id).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if v, ok := in["email"].(string); ok {
		if v == "" || !emailRe.MatchString(v) {
			http.Error(w, "invalid email", 400)
			return
		}
		u.Email = v
	}
	if v, ok := in["password"].(string); ok {
		if len(v) < 8 {
			http.Error(w, "password too short", 400)
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(v), bcrypt.DefaultCost)
		u.Password = string(hash)
		u.MustChangePassword = true
	}
	if v, ok := in["role"].(string); ok {
		u.Role = v
	}
	if err := db.DB.Save(&u).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(u)
}

func (s *apiServer) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid user id", 400)
		return
	}
	if err := db.DB.Delete(&models.User{}, id).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
