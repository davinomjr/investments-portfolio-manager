package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/db"
	"investments-portfolio-manager/backend/internal/httpapi"
	"investments-portfolio-manager/backend/internal/logging"
	"investments-portfolio-manager/backend/internal/services"
)

func main() {
	logging.Setup()
	cfg := config.Load()
	_ = os.MkdirAll(cfg.UploadDir, 0o755)
	_ = os.MkdirAll(cfg.DataCacheDir, 0o755)

	database, err := db.OpenSQLite(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("startup failed: open database: %v", err)
	}

	if err := db.Migrate(database); err != nil {
		log.Fatalf("startup failed: migrate: %v", err)
	}

	// Railway stops the previous deployment with SIGTERM once a new one is
	// live. Exiting cleanly (status 0) keeps that from being reported as a
	// crash, and lets in-flight requests and the SQLite volume wind down.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	service := services.New(database, cfg)
	service.WarmTesouroDireto()
	service.StartSyncScheduler(ctx)
	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.New(service, cfg),
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("backend listening on http://%s", cfg.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	case <-ctx.Done():
		log.Printf("shutdown signal received, draining requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown incomplete: %v", err)
		}
	}
	if err := database.Close(); err != nil {
		log.Printf("database close: %v", err)
	}
	log.Printf("backend stopped")
}
