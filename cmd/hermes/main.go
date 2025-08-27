package main

import (
	"html/template"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/requestid"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/arencloud/hermes/internal/database"
	"github.com/arencloud/hermes/internal/logging"
	"github.com/arencloud/hermes/internal/routes"
	"github.com/arencloud/hermes/internal/version"
)

func main() {
	// Set Gin to release mode by default for production
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize structured logger
	logger, err := logging.NewLogger()
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	// Setup Gin with custom middleware
	app := gin.New()
	app.Use(ginzap.Ginzap(logger, time.RFC3339, true))
	app.Use(ginzap.RecoveryWithZap(logger, true))
	app.Use(requestid.New())

	// Health endpoint
	app.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// OpenAPI docs endpoints
	routes.RegisterDocsRoutes(app)

	// Database
	db, err := database.Connect()
	if err != nil {
		logger.Fatal("database connection error", zap.Error(err))
	}
	if err := database.AutoMigrate(db); err != nil {
		logger.Fatal("auto-migrate error", zap.Error(err))
	}

	// API routes
	routes.RegisterAPIRoutes(app, db)

	// Web routes and templates
	app.SetFuncMap(template.FuncMap{
		"nowYear":    func() int { return time.Now().Year() },
		"appName":    func() string { return version.AppName },
		"appVersion": func() string { return version.AppVersion },
		"appDisplay": func() string { return version.AppName + " v" + version.AppVersion },
	})
	app.LoadHTMLGlob("web/templates/*.html")
	routes.RegisterWebRoutes(app, db)

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	logger.Info("hermes API listening", zap.String("addr", addr))
	if err := app.Run(addr); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}
