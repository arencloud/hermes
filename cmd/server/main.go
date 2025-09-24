package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/arencloud/hermes/internal/api"
	"github.com/arencloud/hermes/internal/config"
	"github.com/arencloud/hermes/internal/db"
	"github.com/arencloud/hermes/internal/logging"
	"github.com/arencloud/hermes/internal/middleware"
)

func main() {
	cfg := config.Load()
	logger := logging.New(cfg.Env)

	if err := db.Init(cfg, logger); err != nil {
		logger.Fatal("failed to init db", "error", err)
	}

	r := api.Router(cfg, logger)

	srv := &http.Server{
		Addr:              ":" + cfg.HttpPort,
		Handler:           middleware.Recoverer(r, logger),
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       0, // allow long-running uploads/downloads; rely on LB timeouts
		WriteTimeout:      0,
		MaxHeaderBytes:    1 << 20, // 1MB headers
	}
	logger.Info("server starting", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Println("server error:", err)
		os.Exit(1)
	}
}
