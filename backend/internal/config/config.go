package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                            string
	DatabaseURL                     string
	ProjectRoot                     string
	WorkerDir                       string
	WorkerPython                    string
	WorkerModule                    string
	WorkerCommand                   string
	UploadDir                       string
	DataCacheDir                    string
	CVMITRBaseURL                   string
	DefaultUserID                   int64
	SentimentEnabled                bool
	SentimentTTLHours               int
	SentimentNewsLookbackDays       int
	SentimentTranscriptLookbackDays int
	SentimentMaxSourcesPerTicker    int
	SentimentUserAgent              string
	AuthPassword                    string
	AuthJWTSecret                   string
	AuthJWTExpiry                   time.Duration
	IBKRFlexToken                   string
	IBKRFlexQueryID                 string
	BrowserWorkerSecret             string
	CORSOrigins                     []string
}

func Load() Config {
	cwd, _ := os.Getwd()
	projectRoot := filepath.Dir(cwd)
	workerPython := env("WORKER_PYTHON", defaultWorkerPython(projectRoot))
	cfg := Config{
		Addr:                            env("ADDR", railwayAddr()),
		DatabaseURL:                     env("DATABASE_URL", filepath.Join(projectRoot, "database", "portfolio.db")),
		ProjectRoot:                     projectRoot,
		WorkerDir:                       env("WORKER_DIR", filepath.Join(projectRoot, "worker")),
		WorkerPython:                    workerPython,
		WorkerModule:                    env("WORKER_MODULE", "app.main"),
		WorkerCommand:                   os.Getenv("WORKER_COMMAND"),
		UploadDir:                       env("UPLOAD_DIR", filepath.Join(projectRoot, "backend", "uploads")),
		DataCacheDir:                    env("DATA_CACHE_DIR", filepath.Join(projectRoot, "backend", "data-cache")),
		CVMITRBaseURL:                   env("CVM_ITR_BASE_URL", "https://dados.cvm.gov.br/dados/cia_aberta/DOC/ITR/DADOS"),
		DefaultUserID:                   1,
		SentimentEnabled:                boolEnv("SENTIMENT_ENABLED", true),
		SentimentTTLHours:               intEnv("SENTIMENT_TTL_HOURS", 24),
		SentimentNewsLookbackDays:       intEnv("SENTIMENT_NEWS_LOOKBACK_DAYS", 14),
		SentimentTranscriptLookbackDays: intEnv("SENTIMENT_TRANSCRIPT_LOOKBACK_DAYS", 45),
		SentimentMaxSourcesPerTicker:    intEnv("SENTIMENT_MAX_SOURCES_PER_TICKER", 10),
		SentimentUserAgent:              env("SENTIMENT_USER_AGENT", "Mozilla/5.0 (compatible; PortfolioManagerBot/1.0; +https://localhost)"),
		AuthPassword:                    os.Getenv("AUTH_PASSWORD"),
		AuthJWTSecret:                   os.Getenv("AUTH_JWT_SECRET"),
		AuthJWTExpiry:                   durationEnv("AUTH_JWT_EXPIRY", 168*time.Hour),
		IBKRFlexToken:                   os.Getenv("IBKR_FLEX_TOKEN"),
		IBKRFlexQueryID:                 os.Getenv("IBKR_FLEX_QUERY_ID"),
		BrowserWorkerSecret:             os.Getenv("BROWSER_WORKER_SECRET"),
		CORSOrigins:                     parseCORSOrigins(env("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")),
	}
	return cfg
}

// railwayAddr returns the listen address, respecting Railway's injected PORT env var.
func railwayAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return "0.0.0.0:" + port
	}
	return "127.0.0.1:8000"
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseCORSOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if o := strings.TrimSpace(p); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
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
