package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/arencloud/hermes/internal/config"
	"github.com/arencloud/hermes/internal/db"
	"github.com/arencloud/hermes/internal/logging"
	"github.com/arencloud/hermes/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// set up a temporary DB and router for integration-style tests
func setupTestServer(t *testing.T) (*httptest.Server, *config.Config) {
	t.Helper()
	tmp := t.TempDir()
	// minimal static dir
	staticDir := filepath.Join(tmp, "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>ok</html>"), 0o644)
	cfg := &config.Config{Env: "test", HttpPort: "0", DBPath: filepath.Join(tmp, "test.db"), DBDriver: "sqlite", StaticDir: staticDir}
	logger := logging.New("test")
	if err := db.Init(cfg, logger); err != nil {
		t.Fatalf("db init: %v", err)
	}
	h := Router(cfg, logger)
	ts := httptest.NewServer(h)
	return ts, cfg
}

func TestHealthAndVersion(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	// /health
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("/health status=%d", resp.StatusCode)
	}
	// /api/version
	resp, err = http.Get(ts.URL + "/api/version")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("/api/version status=%d", resp.StatusCode)
	}
}

func TestAuthLoginAndMe(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	// Create a test user directly in DB
	pass := "secretpass"
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	u := models.User{Email: "test@example.com", Password: string(hash), Role: "viewer"}
	if err := db.DB.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Login
	body, _ := json.Marshal(map[string]string{"email": u.Email, "password": pass})
	resp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("login status=%d", resp.StatusCode)
	}
	// grab cookie
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "dsess" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatalf("no session cookie returned")
	}
	// /me with cookie
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auth/me", nil)
	req.AddCookie(cookie)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("/me status=%d", resp2.StatusCode)
	}
}
