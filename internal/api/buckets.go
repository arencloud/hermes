package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arencloud/hermes/internal/db"
	"github.com/arencloud/hermes/internal/models"
	"github.com/arencloud/hermes/internal/s3"

	"github.com/go-chi/chi/v5"
)

// countingWriter is a tiny io.Writer that invokes a callback with the number of bytes written.
// It allows us to use io.TeeReader to count bytes as they stream without extra buffers/goroutines.
type countingWriter struct{ on func(int) }

func (w countingWriter) Write(p []byte) (int, error) {
	if w.on != nil {
		w.on(len(p))
	}
	return len(p), nil
}

func registerBuckets(r chi.Router) {
	r.Get("/providers/{id}/buckets/db", listBucketsFromDB)
	r.Get("/providers/{id}/buckets", listBuckets)
	// Mutating bucket operations require editor/admin
	r.Group(func(gr chi.Router) {
		gr.Use(requireEditorOrAdmin)
		gr.Post("/providers/{id}/buckets", createBucket)
		gr.Delete("/providers/{id}/buckets/{name}", deleteBucket)
		// objects (mutating)
		gr.Delete("/providers/{id}/buckets/{name}/objects", deleteObject)
		gr.Post("/providers/{id}/buckets/{name}/upload", uploadObject)
		// copy/move between buckets (same provider)
		gr.Post("/providers/{id}/buckets/{name}/move", moveObject)
		gr.Post("/providers/{id}/buckets/{name}/copy", copyObject)
	})
	// Read-only routes available to all authenticated users
	r.Get("/providers/{id}/buckets/{name}/objects", listObjects)
	r.Get("/providers/{id}/buckets/{name}/download", downloadObject)
}

func getClient(id int) (*s3.Client, *models.Provider, error) {
	if id <= 0 {
		return nil, nil, http.ErrNoLocation
	}
	var p models.Provider
	if err := db.DB.First(&p, id).Error; err != nil {
		return nil, nil, err
	}
	c, err := s3.NewFromProvider(p)
	return c, &p, err
}

func listBuckets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	addEvent(r, "buckets.list", nil)
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		respondError(w, r, 400, "invalid provider id")
		return
	}
	c, _, err := getClient(pid)
	if err != nil {
		respondError(w, r, 404, "provider not found")
		return
	}
	items, err := c.ListBuckets(r.Context())
	if err != nil {
		respondError(w, r, 500, err.Error())
		return
	}
	// Sync into DB (upsert live items, prune stale)
	var names = map[string]struct{}{}
	for _, b := range items {
		names[b.Name] = struct{}{}
		var rec models.Bucket
		if err := db.DB.Where("provider_id = ? AND name = ?", pid, b.Name).First(&rec).Error; err == nil {
			// update region if changed; CreatedAt auto-managed
			if rec.Region == "" {
				rec.Region = ""
			}
			db.DB.Save(&rec)
		} else {
			_ = db.DB.Create(&models.Bucket{ProviderID: uint(pid), Name: b.Name}).Error
		}
	}
	// prune buckets in DB that are not present live
	var dbBuckets []models.Bucket
	if err := db.DB.Where("provider_id = ?", pid).Find(&dbBuckets).Error; err == nil {
		for _, rb := range dbBuckets {
			if _, ok := names[rb.Name]; !ok {
				_ = db.DB.Delete(&models.Bucket{}, rb.ID).Error
			}
		}
	}
	json.NewEncoder(w).Encode(items)
}

// listBucketsFromDB returns persisted buckets for provider id. It adapts fields for UI compatibility.
func listBucketsFromDB(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		http.Error(w, "invalid provider id", 400)
		return
	}
	var rows []models.Bucket
	if err := db.DB.Where("provider_id = ?", pid).Order("name asc").Find(&rows).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Create a light DTO compatible with UI buckets rendering: Name and CreationDate
	type dto struct {
		Name         string    `json:"Name"`
		CreationDate time.Time `json:"CreationDate"`
	}
	out := make([]dto, 0, len(rows))
	for _, b := range rows {
		out = append(out, dto{Name: b.Name, CreationDate: b.CreatedAt})
	}
	json.NewEncoder(w).Encode(out)
}

func createBucket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	addEvent(r, "bucket.create", nil)
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		respondError(w, r, 400, "invalid provider id")
		return
	}
	c, p, err := getClient(pid)
	if err != nil {
		respondError(w, r, 404, "provider not found")
		return
	}
	var in struct {
		Name   string `json:"name"`
		Region string `json:"region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondError(w, r, 400, err.Error())
		return
	}
	if in.Region == "" {
		in.Region = p.Region
	}
	if err := c.CreateBucket(r.Context(), in.Name, in.Region); err != nil {
		respondError(w, r, 500, err.Error())
		return
	}
	// upsert into DB immediately
	var rec models.Bucket
	if err := db.DB.Where("provider_id = ? AND name = ?", pid, in.Name).First(&rec).Error; err == nil {
		if rec.Region != in.Region {
			rec.Region = in.Region
			db.DB.Save(&rec)
		}
	} else {
		_ = db.DB.Create(&models.Bucket{ProviderID: uint(pid), Name: in.Name, Region: in.Region}).Error
	}
	w.WriteHeader(201)
}

func deleteBucket(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		respondError(w, r, 400, "invalid provider id")
		return
	}
	name := chi.URLParam(r, "name")
	c, _, err := getClient(pid)
	if err != nil {
		respondError(w, r, 404, "provider not found")
		return
	}
	if err := c.DeleteBucket(r.Context(), name); err != nil {
		respondError(w, r, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func listObjects(w http.ResponseWriter, r *http.Request) {
	addEvent(r, "objects.list", map[string]any{"bucket": chi.URLParam(r, "name")})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		respondError(w, r, 400, "invalid provider id")
		return
	}
	bucket := chi.URLParam(r, "name")
	if bucket == "" {
		respondError(w, r, 400, "bucket is required")
		return
	}
	prefix := r.URL.Query().Get("prefix")
	recursive := r.URL.Query().Get("recursive") == "true"
	c, _, err := getClient(pid)
	if err != nil {
		respondError(w, r, 404, "provider not found")
		return
	}
	items, err := c.ListObjects(r.Context(), bucket, prefix, recursive)
	if err != nil {
		// Map common not-found errors to 404 for better UX
		msg := err.Error()
		if msg != "" {
			if containsNoSuchBucket(msg) {
				respondError(w, r, 404, "bucket not found")
				return
			}
		}
		respondError(w, r, 500, msg)
		return
	}
	json.NewEncoder(w).Encode(items)
}

func deleteObject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		respondError(w, r, 400, "invalid provider id")
		return
	}
	bucket := chi.URLParam(r, "name")
	if bucket == "" {
		respondError(w, r, 400, "bucket is required")
		return
	}
	key := r.URL.Query().Get("key")
	c, _, err := getClient(pid)
	if err != nil {
		respondError(w, r, 404, "provider not found")
		return
	}
	if err := c.DeleteObject(r.Context(), bucket, key); err != nil {
		respondError(w, r, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func uploadObject(w http.ResponseWriter, r *http.Request) {
	addEvent(r, "object.upload", map[string]any{"bucket": chi.URLParam(r, "name")})
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		respondError(w, r, 400, "invalid provider id")
		return
	}
	bucket := chi.URLParam(r, "name")
	if bucket == "" {
		respondError(w, r, 400, "bucket is required")
		return
	}
	c, _, err := getClient(pid)
	if err != nil {
		respondError(w, r, 404, "provider not found")
		return
	}
	// Enforce configurable maximum upload size to avoid memory pressure/DoS
	if maxUploadSizeBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSizeBytes)
	}
	mr, err := r.MultipartReader()
	if err != nil {
		// Handle too large error specifically
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "request body too large") || strings.Contains(strings.ToLower(msg), "http: request body too large") {
			respondError(w, r, 413, "payload too large")
			return
		}
		respondError(w, r, 400, "expecting multipart form-data")
		return
	}
	var key string
	var info any
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			respondError(w, r, 400, err.Error())
			return
		}
		name := part.FormName()
		if name == "key" {
			b, _ := io.ReadAll(part)
			key = string(b)
			continue
		}
		if name == "file" {
			if key == "" {
				key = part.FileName()
			}
			ct := part.Header.Get("Content-Type")
			// Size may be unknown in streaming; minio supports -1 for unknown length
			uploadInfo, err := c.Upload(r.Context(), bucket, key, part, -1, ct)
			if err != nil {
				respondError(w, r, 500, err.Error())
				return
			}
			info = uploadInfo
			addEvent(r, "object.upload.done", map[string]any{"bucket": bucket, "key": key})
			// drain remaining parts but ignore
		}
	}
	if info == nil {
		respondError(w, r, 400, "no file provided")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func downloadObject(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		respondError(w, r, 400, "invalid provider id")
		return
	}
	bucket := chi.URLParam(r, "name")
	key := r.URL.Query().Get("key")
	c, _, err := getClient(pid)
	if err != nil {
		respondError(w, r, 404, "provider not found")
		return
	}
	rc, err := c.Download(r.Context(), bucket, key)
	if err != nil {
		respondError(w, r, 500, err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Disposition", "attachment; filename=\""+key+"\"")
	io.Copy(w, rc)
}

// copyObject copies an object from the current bucket (name) to a destination bucket/key.
// If dstProviderId is provided and differs from {id}, it will stream-copy across providers.
func copyObject(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		http.Error(w, "invalid provider id", 400)
		return
	}
	srcBucket := chi.URLParam(r, "name")
	var in struct {
		SrcKey        string `json:"srcKey"`
		DstBucket     string `json:"dstBucket"`
		DstKey        string `json:"dstKey"`
		DstProviderID int    `json:"dstProviderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if in.SrcKey == "" || in.DstBucket == "" {
		http.Error(w, "srcKey and dstBucket are required", 400)
		return
	}
	if in.DstKey == "" {
		in.DstKey = in.SrcKey
	}

	// We will stream progress updates to the client as NDJSON
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering
	fl, _ := w.(http.Flusher)
	write := func(obj map[string]any) {
		b, _ := json.Marshal(obj)
		w.Write(b)
		w.Write([]byte("\n"))
		if fl != nil {
			fl.Flush()
		}
	}

	// Resolve clients (support cross-provider)
	srcClient, _, err := getClient(pid)
	if err != nil {
		http.Error(w, "source provider not found", 404)
		return
	}
	dstPid := in.DstProviderID
	if dstPid == 0 {
		dstPid = pid
	}
	dstClient, _, err := getClient(dstPid)
	if err != nil {
		http.Error(w, "destination provider not found", 404)
		return
	}

	// Determine size and get reader (best-effort total via DownloadWithInfo)
	rc, total, err := srcClient.DownloadWithInfo(r.Context(), srcBucket, in.SrcKey)
	if err != nil {
		write(map[string]any{"error": err.Error()})
		return
	}
	defer rc.Close()
	write(map[string]any{"status": "starting", "total": total})

	// Counter
	doneCh := make(chan struct{})
	var transferred int64 = 0
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-r.Context().Done():
				return
			case <-doneCh:
				return
			case <-ticker.C:
				pct := 0
				if total > 0 {
					pct = int((float64(transferred) / float64(total)) * 100)
				}
				write(map[string]any{"progress": pct, "bytes": transferred, "total": total})
			}
		}
	}()

	// Wrap reader to count bytes without additional buffering/goroutines
	tee := io.TeeReader(rc, countingWriter{on: func(n int){ transferred += int64(n) }})
	ct := "application/octet-stream"
	if _, err := dstClient.Upload(r.Context(), in.DstBucket, in.DstKey, tee, -1, ct); err != nil {
		write(map[string]any{"error": err.Error()})
		close(doneCh)
		return
	}
	close(doneCh)
	write(map[string]any{"progress": 100, "bytes": transferred, "total": total, "done": true})
	addEvent(r, "object.copy.end", map[string]any{"ok": true})
}

// moveObject moves an object from the current bucket (name) to a destination bucket/key.
// If dstProviderId differs, it will copy across providers then delete the source.
func moveObject(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || pid <= 0 {
		http.Error(w, "invalid provider id", 400)
		return
	}
	srcBucket := chi.URLParam(r, "name")
	var in struct {
		SrcKey        string `json:"srcKey"`
		DstBucket     string `json:"dstBucket"`
		DstKey        string `json:"dstKey"`
		DstProviderID int    `json:"dstProviderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if in.SrcKey == "" || in.DstBucket == "" {
		http.Error(w, "srcKey and dstBucket are required", 400)
		return
	}
	if in.DstKey == "" {
		in.DstKey = in.SrcKey
	}

	// We will stream progress updates to the client as NDJSON
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering
	fl, _ := w.(http.Flusher)
	write := func(obj map[string]any) {
		b, _ := json.Marshal(obj)
		w.Write(b)
		w.Write([]byte("\n"))
		if fl != nil {
			fl.Flush()
		}
	}

	// Resolve clients (support cross-provider)
	srcClient, _, err := getClient(pid)
	if err != nil {
		http.Error(w, "source provider not found", 404)
		return
	}
	dstPid := in.DstProviderID
	if dstPid == 0 {
		dstPid = pid
	}
	dstClient, _, err := getClient(dstPid)
	if err != nil {
		http.Error(w, "destination provider not found", 404)
		return
	}

	// Determine size and get reader (best-effort total via DownloadWithInfo)
	rc, total, err := srcClient.DownloadWithInfo(r.Context(), srcBucket, in.SrcKey)
	if err != nil {
		write(map[string]any{"error": err.Error()})
		return
	}
	defer rc.Close()
	write(map[string]any{"status": "starting", "total": total})

	// Counter
	doneCh := make(chan struct{})
	var transferred int64 = 0
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-r.Context().Done():
				return
			case <-doneCh:
				return
			case <-ticker.C:
				pct := 0
				if total > 0 {
					pct = int((float64(transferred) / float64(total)) * 100)
				}
				write(map[string]any{"progress": pct, "bytes": transferred, "total": total})
			}
		}
	}()

	// Wrap reader to count bytes without additional buffering/goroutines
	tee := io.TeeReader(rc, countingWriter{on: func(n int){ transferred += int64(n) }})
	ct := "application/octet-stream"
	if _, err := dstClient.Upload(r.Context(), in.DstBucket, in.DstKey, tee, -1, ct); err != nil {
		write(map[string]any{"error": err.Error()})
		close(doneCh)
		return
	}
	// For move, delete the source after upload
	if err := srcClient.DeleteObject(r.Context(), srcBucket, in.SrcKey); err != nil {
		write(map[string]any{"error": err.Error()})
		close(doneCh)
		return
	}
	close(doneCh)
	write(map[string]any{"progress": 100, "bytes": transferred, "total": total, "done": true})
	addEvent(r, "object.move.end", map[string]any{"ok": true})
}

// containsNoSuchBucket reports whether the error message indicates the bucket is missing.
func containsNoSuchBucket(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "nosuchbucket") || strings.Contains(m, "bucket does not exist")
}
