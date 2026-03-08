package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Addr          string
	DatabaseURL   string
	ProjectRoot   string
	WorkerDir     string
	WorkerPython  string
	WorkerModule  string
	WorkerCommand string
	UploadDir     string
	DataCacheDir  string
	CVMITRBaseURL string
	DefaultUserID int64
}

func Load() Config {
	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(cwd)
	workerPython := env("WORKER_PYTHON", defaultWorkerPython(projectRoot))
	cfg := Config{
		Addr:          env("ADDR", "127.0.0.1:8000"),
		DatabaseURL:   env("DATABASE_URL", filepath.Join(projectRoot, "database", "portfolio.db")),
		ProjectRoot:   projectRoot,
		WorkerDir:     env("WORKER_DIR", filepath.Join(projectRoot, "worker")),
		WorkerPython:  workerPython,
		WorkerModule:  env("WORKER_MODULE", "app.main"),
		WorkerCommand: os.Getenv("WORKER_COMMAND"),
		UploadDir:     env("UPLOAD_DIR", filepath.Join(projectRoot, "backend", "uploads")),
		DataCacheDir:  env("DATA_CACHE_DIR", filepath.Join(projectRoot, "backend", "data-cache")),
		CVMITRBaseURL: env("CVM_ITR_BASE_URL", "https://dados.cvm.gov.br/dados/cia_aberta/DOC/ITR/DADOS"),
		DefaultUserID: 1,
	}
	return cfg
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func defaultWorkerPython(projectRoot string) string {
	candidates := []string{
		filepath.Join(projectRoot, ".venv", "bin", "python"),
		filepath.Join(projectRoot, ".venv", "bin", "python3"),
		filepath.Join(projectRoot, "worker", ".venv", "bin", "python"),
		filepath.Join(projectRoot, "worker", ".venv", "bin", "python3"),
		filepath.Join(projectRoot, "backend-python", ".venv", "bin", "python"),
		filepath.Join(projectRoot, "backend-python", ".venv", "bin", "python3"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "python3"
}
