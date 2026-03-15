package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/db"
	"investments-portfolio-manager/backend/internal/models"
	"investments-portfolio-manager/backend/internal/services"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func newHandlerFromDB(database *sql.DB) http.Handler {
	svc := services.New(database, config.Config{DefaultUserID: 1})
	return New(svc)
}

func mustDecodeJSON[T any](t *testing.T, body *bytes.Buffer) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return out
}

func seedAssetAndPosition(t *testing.T, database *sql.DB, ticker, assetType string, qty, avgPrice float64, hidden bool) {
	t.Helper()
	ctx := context.Background()
	_, err := database.ExecContext(ctx,
		`INSERT INTO assets(ticker, asset_type, currency) VALUES(?,?,'BRL')`, ticker, assetType)
	if err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	var assetID int64
	if err := database.QueryRowContext(ctx, `SELECT id FROM assets WHERE ticker=?`, ticker).Scan(&assetID); err != nil {
		t.Fatalf("get asset id: %v", err)
	}
	hiddenInt := 0
	if hidden {
		hiddenInt = 1
	}
	_, err = database.ExecContext(ctx,
		`INSERT INTO positions(user_id, asset_id, quantity, avg_price, source, last_updated, hidden) VALUES(1,?,?,?,'b3',datetime('now'),?)`,
		assetID, qty, avgPrice, hiddenInt)
	if err != nil {
		t.Fatalf("insert position: %v", err)
	}
}

func seedImportJob(t *testing.T, database *sql.DB, source, status, detail, createdAt string) {
	t.Helper()
	_, err := database.ExecContext(context.Background(),
		`INSERT INTO import_jobs(source, status, detail, created_at, updated_at) VALUES(?,?,?,?,?)`,
		source, status, detail, createdAt, createdAt)
	if err != nil {
		t.Fatalf("insert import job: %v", err)
	}
}

// ---- GET /portfolio ----

func TestGetPortfolioEmpty(t *testing.T) {
	database := newTestDB(t)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/portfolio", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	resp := mustDecodeJSON[models.PortfolioResponse](t, rec.Body)
	if resp.TotalPositions != 0 {
		t.Errorf("TotalPositions: got %d, want 0", resp.TotalPositions)
	}
	if resp.EstimatedCostBasis != 0 {
		t.Errorf("EstimatedCostBasis: got %f, want 0", resp.EstimatedCostBasis)
	}
}

func TestGetPortfolioWithPositions(t *testing.T) {
	database := newTestDB(t)
	seedAssetAndPosition(t, database, "PETR4", "stock", 10, 30.0, false)
	seedAssetAndPosition(t, database, "VALE3", "stock", 5, 100.0, false)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/portfolio", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	resp := mustDecodeJSON[models.PortfolioResponse](t, rec.Body)
	if resp.TotalPositions != 2 {
		t.Errorf("TotalPositions: got %d, want 2", resp.TotalPositions)
	}
	// PETR4=300 + VALE3=500 = 800
	if resp.EstimatedCostBasis != 800.0 {
		t.Errorf("EstimatedCostBasis: got %f, want 800.0", resp.EstimatedCostBasis)
	}
	if len(resp.Allocations) != 2 {
		t.Errorf("Allocations: got %d, want 2", len(resp.Allocations))
	}
}

// ---- GET /positions ----

func TestGetPositionsEmpty(t *testing.T) {
	database := newTestDB(t)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/positions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	// Body should decode as a JSON array (may be null or [])
	var positions []models.PositionResponse
	if err := json.NewDecoder(rec.Body).Decode(&positions); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestGetPositionsWithData(t *testing.T) {
	database := newTestDB(t)
	seedAssetAndPosition(t, database, "KNRI11", "fii", 20, 50.0, false)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/positions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	positions := mustDecodeJSON[[]models.PositionResponse](t, rec.Body)
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	p := positions[0]
	if p.Ticker != "KNRI11" {
		t.Errorf("ticker: got %q, want KNRI11", p.Ticker)
	}
	if p.Quantity != 20 {
		t.Errorf("quantity: got %f, want 20", p.Quantity)
	}
	if p.AvgPrice != 50.0 {
		t.Errorf("avg_price: got %f, want 50.0", p.AvgPrice)
	}
	if p.Hidden {
		t.Errorf("hidden: got true, want false")
	}
}

// ---- PATCH /positions/visibility ----

func TestPatchVisibilityHide(t *testing.T) {
	database := newTestDB(t)
	seedAssetAndPosition(t, database, "PETR4", "stock", 10, 30.0, false)
	handler := newHandlerFromDB(database)

	body := strings.NewReader(`{"visible":false}`)
	req := httptest.NewRequest(http.MethodPatch, "/positions/visibility", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", rec.Code)
	}

	// Confirm positions are hidden
	req2 := httptest.NewRequest(http.MethodGet, "/positions", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	positions := mustDecodeJSON[[]models.PositionResponse](t, rec2.Body)
	for _, p := range positions {
		if !p.Hidden {
			t.Errorf("position %s should be hidden after PATCH visible=false", p.Ticker)
		}
	}
}

func TestPatchVisibilityShow(t *testing.T) {
	database := newTestDB(t)
	seedAssetAndPosition(t, database, "PETR4", "stock", 10, 30.0, true)
	handler := newHandlerFromDB(database)

	body := strings.NewReader(`{"visible":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/positions/visibility", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", rec.Code)
	}

	// Confirm positions are visible
	req2 := httptest.NewRequest(http.MethodGet, "/positions", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	positions := mustDecodeJSON[[]models.PositionResponse](t, rec2.Body)
	for _, p := range positions {
		if p.Hidden {
			t.Errorf("position %s should be visible after PATCH visible=true", p.Ticker)
		}
	}
}

func TestPatchVisibilityInvalidBody(t *testing.T) {
	database := newTestDB(t)
	handler := newHandlerFromDB(database)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPatch, "/positions/visibility", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", rec.Code)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["detail"] == "" {
		t.Errorf("expected non-empty detail in error response")
	}
}

// ---- GET /portfolio/import-jobs/latest ----

func TestGetLatestImportJobNotFound(t *testing.T) {
	database := newTestDB(t)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/portfolio/import-jobs/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["detail"] != "no import jobs found" {
		t.Errorf("detail: got %q, want %q", errResp["detail"], "no import jobs found")
	}
}

func TestGetLatestImportJobFound(t *testing.T) {
	database := newTestDB(t)
	seedImportJob(t, database, "b3", "completed", "Imported 5 positions", "2024-01-01T00:00:00Z")
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/portfolio/import-jobs/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	resp := mustDecodeJSON[models.ImportJobResponse](t, rec.Body)
	if resp.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if resp.Source != "b3" {
		t.Errorf("source: got %q, want b3", resp.Source)
	}
	if resp.Status != "completed" {
		t.Errorf("status: got %q, want completed", resp.Status)
	}
	if resp.CreatedAt == "" {
		t.Errorf("expected non-empty created_at")
	}
}

func TestGetLatestImportJobPicksMostRecent(t *testing.T) {
	database := newTestDB(t)
	seedImportJob(t, database, "b3", "completed", "first", "2024-01-01T00:00:00Z")
	seedImportJob(t, database, "manual_b3_export", "running", "second", "2024-01-02T00:00:00Z")
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/portfolio/import-jobs/latest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	resp := mustDecodeJSON[models.ImportJobResponse](t, rec.Body)
	if resp.Source != "manual_b3_export" {
		t.Errorf("expected most recent job (manual_b3_export), got %s", resp.Source)
	}
}

// ---- CORS tests ----

func TestCORSAllowedOrigin(t *testing.T) {
	database := newTestDB(t)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/portfolio", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin header for allowed origin")
	}
}

func TestCORSDisallowedOrigin(t *testing.T) {
	database := newTestDB(t)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodGet, "/portfolio", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no Access-Control-Allow-Origin header for disallowed origin")
	}
}

func TestCORSPreflight(t *testing.T) {
	database := newTestDB(t)
	handler := newHandlerFromDB(database)

	req := httptest.NewRequest(http.MethodOptions, "/portfolio", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status: got %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Access-Control-Allow-Methods header in preflight response")
	}
}
