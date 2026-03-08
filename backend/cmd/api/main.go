package main

import (
	"log"
	"net/http"
	"os"

	"portfolio-manager/backend/internal/config"
	"portfolio-manager/backend/internal/db"
	"portfolio-manager/backend/internal/httpapi"
	"portfolio-manager/backend/internal/services"
)

func main() {
	cfg := config.Load()
	_ = os.MkdirAll(cfg.UploadDir, 0o755)
	_ = os.MkdirAll(cfg.DataCacheDir, 0o755)

	database, err := db.OpenSQLite(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatal(err)
	}

	service := services.New(database, cfg)
	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.New(service),
	}

	log.Printf("backend listening on http://%s", cfg.Addr)
	log.Fatal(server.ListenAndServe())
}
