package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"investments-portfolio-manager/backend/internal/auth"
	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/services"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	Service *services.Service
	Config  config.Config
}

func New(svc *services.Service, cfg config.Config) http.Handler {
	server := &Server{Service: svc, Config: cfg}
	mux := http.NewServeMux()

	// Unauthenticated routes
	mux.HandleFunc("POST /auth/login", server.handleLogin)
	mux.HandleFunc("POST /auth/logout", server.handleLogout)

	// Authenticated routes
	authed := http.NewServeMux()
	authed.HandleFunc("GET /portfolio", server.handlePortfolio)
	authed.HandleFunc("GET /positions", server.handlePositions)
	authed.HandleFunc("PATCH /positions/visibility", server.handleSetPositionsVisibility)
	authed.HandleFunc("GET /stocks/latest-results", server.handleLatestResults)
	authed.HandleFunc("GET /stocks/{ticker}/sentiment", server.handleTickerSentiment)
	authed.HandleFunc("GET /fiis/latest-results", server.handleLatestFIIResults)
	authed.HandleFunc("GET /portfolio/monte-carlo", server.handleMonteCarlo)
	authed.HandleFunc("GET /portfolio/import-jobs/latest", server.handleGetLatestImportJob)
	authed.HandleFunc("POST /portfolio/import-b3", server.handleImportB3)
	authed.HandleFunc("POST /portfolio/import-file", server.handleImportFile)

	// Mount authed routes under main mux with auth middleware
	mux.Handle("/", server.withAuth(authed))

	return withCORS(mux)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(s.Config.AuthPassword), []byte(body.Password)); err != nil {
		writeErr(w, "invalid password", http.StatusUnauthorized)
		return
	}
	token, err := auth.GenerateToken(s.Config.AuthJWTSecret, s.Config.DefaultUserID, s.Config.AuthJWTExpiry)
	if err != nil {
		writeErr(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(s.Config.AuthJWTExpiry.Seconds()),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			writeErr(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := auth.ValidateToken(s.Config.AuthJWTSecret, cookie.Value); err != nil {
			writeErr(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handlePortfolio(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetPortfolio(r.Context())
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetPositions(r.Context())
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handleSetPositionsVisibility(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Visible bool `json:"visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.Service.SetPositionsVisibility(r.Context(), body.Visible); err != nil {
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLatestResults(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetLatestQuarterlyResults(r.Context())
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handleTickerSentiment(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	sentiment, err := s.Service.GetTickerSentiment(r.Context(), ticker)
	if err != nil {
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sentiment == nil {
		writeErr(w, "ticker not found", http.StatusNotFound)
		return
	}
	writeJSON(w, sentiment, nil, http.StatusOK)
}

func (s *Server) handleLatestFIIResults(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetLatestFIIResults(r.Context())
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handleMonteCarlo(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	params := services.ParseMonteCarloParams(
		query.Get("years"),
		query.Get("simulations"),
		query.Get("expected_return"),
		query.Get("volatility"),
	)
	resp, err := s.Service.GetMonteCarloSimulation(r.Context(), params)
	writeJSON(w, resp, err, http.StatusOK)
}

func (s *Server) handleGetLatestImportJob(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Service.GetLatestImportJob(r.Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, "no import jobs found", http.StatusNotFound)
			return
		}
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp, nil, http.StatusOK)
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
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			} else {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
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
