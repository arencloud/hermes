package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/arencloud/hermes/internal/db"
	"github.com/arencloud/hermes/internal/models"
	"github.com/go-chi/chi/v5"
)

func registerProviders(r chi.Router) {
	// Read-only provider endpoints available to any authenticated user (viewers need these to select provider)
	r.Get("/providers", listProviders)
	r.Get("/providers/{id}", getProvider)
	// Mutating provider endpoints require editor or admin
	r.Group(func(gr chi.Router) {
		gr.Use(requireEditorOrAdmin)
		gr.Post("/providers", createProvider)
		gr.Put("/providers/{id}", updateProvider)
		gr.Delete("/providers/{id}", deleteProvider)
	})
}

func listProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var items []models.Provider
	if err := db.DB.Find(&items).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(items)
}

func createProvider(w http.ResponseWriter, r *http.Request) {
	addEvent(r, "provider.create", nil)
	w.Header().Set("Content-Type", "application/json")
	var p models.Provider
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Basic validation
	if p.Name == "" || p.Endpoint == "" {
		http.Error(w, "name and endpoint are required", 400)
		return
	}
	if err := db.DB.Create(&p).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(p)
}

func getProvider(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid provider id", 400)
		return
	}
	var p models.Provider
	if err := db.DB.First(&p, id).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	json.NewEncoder(w).Encode(p)
}

func updateProvider(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid provider id", 400)
		return
	}
	var p models.Provider
	if err := db.DB.First(&p, id).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Patch-like update: apply provided fields only
	if name, ok := in["name"].(string); ok {
		p.Name = name
	}
	if typ, ok := in["type"].(string); ok {
		p.Type = typ
	}
	if ep, ok := in["endpoint"].(string); ok {
		p.Endpoint = ep
	}
	if ak, ok := in["accessKey"].(string); ok {
		p.AccessKey = ak
	}
	if sk, ok := in["secretKey"].(string); ok {
		p.SecretKey = sk
	}
	if rg, ok := in["region"].(string); ok {
		p.Region = rg
	}
	if ussl, ok := in["useSSL"].(bool); ok {
		p.UseSSL = ussl
	}
	// Validate required fields after merge
	if p.Name == "" || p.Endpoint == "" {
		http.Error(w, "name and endpoint are required", 400)
		return
	}
	if err := db.DB.Save(&p).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(p)
}

func deleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid provider id", 400)
		return
	}
	if err := db.DB.Delete(&models.Provider{}, id).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
