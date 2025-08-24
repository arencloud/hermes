package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"hermes/internal/auth"
)

type viewData map[string]any

// helper to format bytes in Go templates
func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n)
	for i, u := range units {
		v = v / 1024
		if v < 1024 || i == len(units)-1 {
			return fmt.Sprintf("%.2f %s", v, u)
		}
	}
	return fmt.Sprintf("%d B", n)
}

// render parses base.html + page template and executes base with provided data
func render(c *gin.Context, page string, data viewData) {
	// ensure common fields
	if data == nil {
		data = viewData{}
	}
	if uVal, ok := c.Get("user"); ok && uVal != nil {
		u := uVal.(*auth.User)
		data["User"] = u.Username
		data["Role"] = u.Role
	}

	funcs := template.FuncMap{
		"now":         time.Now,
		"formatBytes": formatBytes,
	}
	base := filepath.Join("templates", "base.html")
	pagePath := filepath.Join("templates", page)
	t, err := template.New("base.html").Funcs(funcs).ParseFiles(base, pagePath)
	if err != nil {
		c.String(http.StatusInternalServerError, "template error: %v", err)
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(c.Writer, "base.html", data); err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// cancellableCtx returns a context with timeout for setup tasks
func cancellableCtx(d time.Duration) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), d)
	return ctx
}

func main() {
	r := gin.Default()

	// Sessions
	secret := os.Getenv("HERMES_SESSION_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me"
	}
	store := cookie.NewStore([]byte(secret))
	r.Use(sessions.Sessions("hermes_sess", store))
	r.Use(auth.SessionMiddleware())

	// Database (optional). If DATABASE_URL is set, use PostgreSQL backend.
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			log.Printf("postgres open error: %v (falling back to in-memory store)", err)
		} else {
			if err := db.Ping(); err != nil {
				log.Printf("postgres ping error: %v (falling back to in-memory store)", err)
			} else {
				auth.InitPostgres(db)
				if err := auth.MigratePostgres(cancellableCtx(10 * time.Second)); err != nil {
					log.Printf("postgres migrate error: %v", err)
				}
			}
		}
	}

	// Ensure default admin exists
	_ = auth.GetStore().CreateUser("admin", "password", "admin", "default")

	// Root -> dashboard or login
	r.GET("/", func(c *gin.Context) {
		if _, ok := c.Get("user"); ok {
			c.Redirect(http.StatusFound, "/dashboard")
			return
		}
		c.Redirect(http.StatusFound, "/login")
	})

	// Login page
	r.GET("/login", func(c *gin.Context) {
		render(c, "login.html", viewData{"Title": "Login"})
	})

	// JSON login endpoint expected by UI
	r.POST("/api/v1/auth/login", func(c *gin.Context) {
		var req struct {
			Tenant   string `json:"tenant"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}
		u, err := auth.GetStore().Authenticate(req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		// establish session for server-rendered pages
		auth.Login(c, u.Username)
		// return dummy tokens to satisfy UI expectations
		c.JSON(http.StatusOK, gin.H{
			"access_token":  "dummy-access-token",
			"refresh_token": "dummy-refresh-token",
		})
	})

	// Logout page clears session and shows logout screen
	r.GET("/logout", func(c *gin.Context) {
		auth.Logout(c)
		render(c, "logout.html", viewData{"Title": "Logout"})
	})

	// Dashboard
	r.GET("/dashboard", auth.RequireAuth(), func(c *gin.Context) {
		vd := viewData{
			"Title":       "Dashboard",
			"CurrentPage": "dashboard",
			"Tenant": map[string]any{
				"Name":        "Default",
				"Domain":      "default.local",
				"Status":      "active",
				"CreatedAt":   time.Now().AddDate(0, -1, 0),
				"Description": "",
			},
			"Stats": map[string]any{
				"bucket_count":        0,
				"active_bucket_count": 0,
				"user_count":          1,
			},
			"Buckets":  []any{},
			"UserID":   1,
			"TenantID": 1,
		}
		render(c, "dashboard.html", vd)
	})

	// Buckets list
	r.GET("/buckets", auth.RequireAuth(), func(c *gin.Context) {
		vd := viewData{
			"Title":       "Buckets",
			"CurrentPage": "buckets",
			"Total":       0,
			"Stats": map[string]any{
				"active_bucket_count": 0,
				"user_count":          1,
			},
			"Buckets": []any{},
		}
		render(c, "buckets.html", vd)
	})

	// Bucket detail (mock)
	r.GET("/buckets/:id", auth.RequireAuth(), func(c *gin.Context) {
		idStr := c.Param("id")
		id, _ := strconv.Atoi(idStr)
		vd := viewData{
			"Title":       "Bucket Details",
			"CurrentPage": "buckets",
			"Bucket": map[string]any{
				"ID":        id,
				"Name":      fmt.Sprintf("bucket-%s", idStr),
				"Provider":  "minio",
				"Region":    "",
				"Endpoint":  "https://minio.local",
				"Status":    "active",
				"CreatedAt": time.Now().AddDate(0, -1, 0),
			},
			"Objects":  []any{},
			"UserRole": "admin",
		}
		render(c, "bucket_detail.html", vd)
	})

	// Upload page for a bucket
	r.GET("/buckets/:id/upload", auth.RequireAuth(), func(c *gin.Context) {
		idStr := c.Param("id")
		id, _ := strconv.Atoi(idStr)
		vd := viewData{
			"Title":       "Upload",
			"CurrentPage": "buckets",
			"Bucket": map[string]any{
				"ID":   id,
				"Name": fmt.Sprintf("bucket-%s", idStr),
			},
		}
		render(c, "upload.html", vd)
	})

	// Tenants page (mock list)
	r.GET("/tenants", auth.RequireAuth(), func(c *gin.Context) {
		vd := viewData{
			"Title":       "Tenants",
			"CurrentPage": "tenants",
			"Tenants":     []any{},
		}
		render(c, "tenants.html", vd)
	})

	// Users page: render list
	r.GET("/users", auth.RequireAuth(), auth.RequireAdmin(), func(c *gin.Context) {
		vd := viewData{
			"Title":       "Users",
			"CurrentPage": "users",
			"Users":       auth.GetStore().List(),
		}
		render(c, "users.html", vd)
	})

	// Users API (CRUD)
	api := r.Group("/api/v1", auth.RequireAuth(), auth.RequireAdmin())
	{
		api.GET("/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"users": auth.GetStore().List()})
		})
		api.POST("/users", func(c *gin.Context) {
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Role     string `json:"role"`
				Tenant   string `json:"tenant"`
			}
			if err := c.BindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
				return
			}
			if err := auth.GetStore().CreateUser(req.Username, req.Password, req.Role, req.Tenant); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			u, _ := auth.GetStore().Get(req.Username)
			c.JSON(http.StatusCreated, u)
		})
		api.PUT("/users/:username", func(c *gin.Context) {
			username := c.Param("username")
			var req struct {
				Role     string `json:"role"`
				Tenant   string `json:"tenant"`
				Password string `json:"password"`
			}
			if err := c.BindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
				return
			}
			if err := auth.GetStore().UpdateUser(username, req.Role, req.Tenant, req.Password); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			u, _ := auth.GetStore().Get(username)
			c.JSON(http.StatusOK, u)
		})
		api.DELETE("/users/:username", func(c *gin.Context) {
			username := c.Param("username")
			if username == "admin" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete admin"})
				return
			}
			if err := auth.GetStore().DeleteUser(username); err != nil {
				status := http.StatusNotFound
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
			c.Status(http.StatusNoContent)
		})
	}

	// Profile page
	r.GET("/profile", auth.RequireAuth(), func(c *gin.Context) {
		vd := viewData{
			"Title":       "Profile",
			"CurrentPage": "profile",
			"UserID":      1,
			"TenantID":    1,
		}
		render(c, "profile.html", vd)
	})

	// Error page demo
	r.GET("/error", func(c *gin.Context) {
		render(c, "error.html", viewData{"Title": "Error", "Error": "An example error page"})
	})

	// Health endpoint for container orchestration
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("Starting Hermes server on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
