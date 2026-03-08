package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"investments-portfolio-manager/backend/internal/services"
)

type Server struct {
	Service *services.Service
}

func New(svc *services.Service) http.Handler {
	server := &Server{Service: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /portfolio", server.handlePortfolio)
	mux.HandleFunc("GET /positions", server.handlePositions)
	mux.HandleFunc("GET /stocks/latest-results", server.handleLatestResults)
	mux.HandleFunc("POST /portfolio/import-b3", server.handleImportB3)
	mux.HandleFunc("POST /portfolio/import-file", server.handleImportFile)
	return withCORS(mux)
}

func (s *Server) handlePortfolio(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetPortfolio(r.Context())
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetPositions(r.Context())
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handleLatestResults(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetLatestQuarterlyResults(r.Context())
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handleImportB3(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.ImportB3(r.Context())
	writeJSON(w, resp, err, http.StatusAccepted)
}

func (s *Server) handleImportFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	name := strings.ToLower(header.Filename)
	if !(strings.HasSuffix(name, ".xlsx") || strings.HasSuffix(name, ".xlsm") || strings.HasSuffix(name, ".csv")) {
		writeErr(w, "Only .xlsx, .xlsm, and .csv files are supported.", http.StatusBadRequest)
		return
	}
	resp, err := s.Service.ImportFile(r.Context(), file, header.Filename)
	writeJSON(w, resp, err, http.StatusAccepted)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:3000" || origin == "http://127.0.0.1:3000" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if requestHeaders := r.Header.Get("Access-Control-Request-Headers"); requestHeaders != "" {
				w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			if requestMethod := r.Header.Get("Access-Control-Request-Method"); requestMethod != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			} else {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, payload any, err error, status int) {
	if err != nil {
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErr(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": message})
}
