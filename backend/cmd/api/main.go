package main

import (
	"context"
	"log"
	"net/http"
	"os"

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
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("startup failed: migrate: %v", err)
	}

	service := services.New(database, cfg)
	service.WarmTesouroDireto()
	service.StartSyncScheduler(context.Background())
	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.New(service, cfg),
	}

	log.Printf("backend listening on http://%s", cfg.Addr)
	log.Fatalf("server failed: %v", server.ListenAndServe())
}
