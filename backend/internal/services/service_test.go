package services

import (
	"context"
	"database/sql"
	"testing"

	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/db"
	"investments-portfolio-manager/backend/internal/models"
)

// seedFullPosition inserts an asset + position with full column control.
// lastUpdatedOffset is an integer used as seconds offset from now (e.g. -10 means 10 seconds ago).
func seedFullPosition(t *testing.T, svc *Service, ticker, assetType string, qty, avgPrice float64, broker, source string, hidden bool, lastUpdatedOffset int) {
	t.Helper()
	ctx := context.Background()
	_, err := svc.DB.ExecContext(ctx,
		`INSERT INTO assets(ticker, asset_type, currency) VALUES(?,?,'BRL')`,
		ticker, assetType)
	if err != nil {
		t.Fatalf("seedFullPosition insert asset: %v", err)
	}
	var assetID int64
	if err := svc.DB.QueryRowContext(ctx, `SELECT id FROM assets WHERE ticker=?`, ticker).Scan(&assetID); err != nil {
		t.Fatalf("seedFullPosition get asset id: %v", err)
	}
	hiddenInt := 0
	if hidden {
		hiddenInt = 1
	}
	brokerVal := interface{}(nil)
	if broker != "" {
		brokerVal = broker
	}
	_, err = svc.DB.ExecContext(ctx,
		`INSERT INTO positions(user_id, asset_id, quantity, avg_price, broker, source, last_updated, hidden)
		 VALUES(1, ?, ?, ?, ?, ?, datetime('now', ? || ' seconds'), ?)`,
		assetID, qty, avgPrice, brokerVal, source, lastUpdatedOffset, hiddenInt)
	if err != nil {
		t.Fatalf("seedFullPosition insert position: %v", err)
	}
}

func newTestServiceWithConfig(t *testing.T, cfg config.Config) *Service {
	t.Helper()
	database, err := db.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database, cfg)
}

// ---- GetPositions tests ----

func TestGetPositionsEmpty(t *testing.T) {
	svc := newTestService(t)
	positions, err := svc.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 0 {
		t.Fatalf("expected empty slice, got %d", len(positions))
	}
}

func TestGetPositionsReturnsAllFields(t *testing.T) {
	svc := newTestService(t)
	seedFullPosition(t, svc, "PETR4", "stock", 15, 42.5, "XP", "b3", false, 0)

	positions, err := svc.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	p := positions[0]
	if p.Ticker != "PETR4" {
		t.Errorf("ticker: got %q, want %q", p.Ticker, "PETR4")
	}
	if p.AssetType != "stock" {
		t.Errorf("asset_type: got %q, want %q", p.AssetType, "stock")
	}
	if p.Quantity != 15 {
		t.Errorf("quantity: got %f, want 15", p.Quantity)
	}
	if p.AvgPrice != 42.5 {
		t.Errorf("avg_price: got %f, want 42.5", p.AvgPrice)
	}
	if p.Broker != "XP" {
		t.Errorf("broker: got %q, want %q", p.Broker, "XP")
	}
	if p.Source != "b3" {
		t.Errorf("source: got %q, want %q", p.Source, "b3")
	}
	if p.Hidden != false {
		t.Errorf("hidden: got %v, want false", p.Hidden)
	}
}

func TestGetPositionsOrderedByLastUpdatedDesc(t *testing.T) {
	svc := newTestService(t)
	// First inserted but older (offset -60 seconds)
	seedFullPosition(t, svc, "VALE3", "stock", 5, 50.0, "", "b3", false, -60)
	// Second inserted but newer (offset -10 seconds)
	seedFullPosition(t, svc, "ITUB4", "stock", 8, 30.0, "", "b3", false, -10)

	positions, err := svc.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}
	if positions[0].Ticker != "ITUB4" {
		t.Errorf("expected ITUB4 first (newest), got %s", positions[0].Ticker)
	}
	if positions[1].Ticker != "VALE3" {
		t.Errorf("expected VALE3 second (oldest), got %s", positions[1].Ticker)
	}
}

func TestGetPositionsHiddenFlagReflected(t *testing.T) {
	svc := newTestService(t)
	seedFullPosition(t, svc, "BBDC4", "stock", 20, 15.0, "", "b3", true, 0)

	positions, err := svc.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if !positions[0].Hidden {
		t.Errorf("expected Hidden=true, got false")
	}
}

// ---- GetPortfolio tests ----

func TestGetPortfolioEmpty(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.GetPortfolio(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalPositions != 0 {
		t.Errorf("TotalPositions: got %d, want 0", resp.TotalPositions)
	}
	if resp.EstimatedCostBasis != 0 {
		t.Errorf("EstimatedCostBasis: got %f, want 0", resp.EstimatedCostBasis)
	}
	if len(resp.Allocations) != 0 {
		t.Errorf("Allocations: got %d items, want 0", len(resp.Allocations))
	}
}

func TestGetPortfolioSinglePosition(t *testing.T) {
	svc := newTestService(t)
	seedFullPosition(t, svc, "PETR4", "stock", 10, 30.0, "", "b3", false, 0)

	resp, err := svc.GetPortfolio(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalPositions != 1 {
		t.Errorf("TotalPositions: got %d, want 1", resp.TotalPositions)
	}
	if resp.EstimatedCostBasis != 300.0 {
		t.Errorf("EstimatedCostBasis: got %f, want 300.0", resp.EstimatedCostBasis)
	}
	if len(resp.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(resp.Allocations))
	}
	if resp.Allocations[0].Weight != 1.0 {
		t.Errorf("Weight: got %f, want 1.0", resp.Allocations[0].Weight)
	}
}

func TestGetPortfolioWeightsAndOrdering(t *testing.T) {
	svc := newTestService(t)
	// PETR4: qty=10 * price=30 = 300
	seedFullPosition(t, svc, "PETR4", "stock", 10, 30.0, "", "b3", false, 0)
	// VALE3: qty=5 * price=100 = 500
	seedFullPosition(t, svc, "VALE3", "stock", 5, 100.0, "", "b3", false, -1)

	resp, err := svc.GetPortfolio(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalPositions != 2 {
		t.Errorf("TotalPositions: got %d, want 2", resp.TotalPositions)
	}
	total := 800.0
	if resp.EstimatedCostBasis != total {
		t.Errorf("EstimatedCostBasis: got %f, want %f", resp.EstimatedCostBasis, total)
	}

	// Weights should sum to ~1.0
	weightSum := 0.0
	for _, alloc := range resp.Allocations {
		weightSum += alloc.Weight
	}
	if weightSum < 0.999 || weightSum > 1.001 {
		t.Errorf("weights sum: got %f, want ~1.0", weightSum)
	}

	// Larger value (VALE3=500) should come first
	if resp.Allocations[0].Ticker != "VALE3" {
		t.Errorf("expected VALE3 first (larger value), got %s", resp.Allocations[0].Ticker)
	}
}

func TestScrapeFundsExplorerFIIParsesCurrentTextLayout(t *testing.T) {
	const sampleHTML = `
	<html>
		<body>
			<div>Liquidez Media Diaria</div>
			<div>1,6 M</div>
			<div>Ultimo rendimento</div>
			<div>R$ 0,80</div>
			<div>Dividend Yield</div>
			<div>11,05 %</div>
			<div>Patrimonio Liquido</div>
			<div>P/VP</div>
			<div>0,90</div>
			<div>Vale a pena investir</div>
		</body>
	</html>`

	text := normalizeStatusInvestText(sampleHTML)

	assertFloatPtr(t, scrapeFundsExplorerPercent(text, "dividend yield", "patrimonio liquido"), 11.05, "dividend yield")
	assertFloatPtr(t, scrapeFundsExplorerNumber(text, "p/vp", "vale a pena investir"), 0.9, "p/vp")
	assertFloatPtr(t, scrapeFundsExplorerCurrency(text, "ultimo rendimento", "dividend yield"), 0.8, "dividend per unit")
	assertFloatPtr(t, scrapeFundsExplorerAbbrevCurrency(text, "liquidez media diaria", "ultimo rendimento"), 1600000, "avg daily volume")
}

func TestScrapeFundsExplorerReturnsNilForMissingValue(t *testing.T) {
	const sampleHTML = `<div>Liquidez Media Diaria</div><div>-</div><div>Ultimo rendimento</div>`

	text := normalizeStatusInvestText(sampleHTML)

	if got := scrapeFundsExplorerAbbrevCurrency(text, "liquidez media diaria", "ultimo rendimento"); got != nil {
		t.Fatalf("expected nil liquidity, got %v", *got)
	}
}

func assertFloatPtr(t *testing.T, got *float64, want float64, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: got nil, want %f", label, want)
	}
	if *got != want {
		t.Fatalf("%s: got %f, want %f", label, *got, want)
	}
}

// ---- SetPositionsVisibility tests ----

func TestSetPositionsVisibility(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(svc *Service)
		visible    bool
		wantHidden bool
	}{
		{
			name: "hide-all",
			setup: func(svc *Service) {
				seedFullPosition(t, svc, "PETR4", "stock", 10, 30.0, "", "b3", false, 0)
				seedFullPosition(t, svc, "VALE3", "stock", 5, 100.0, "", "b3", false, -1)
			},
			visible:    false,
			wantHidden: true,
		},
		{
			name: "show-all",
			setup: func(svc *Service) {
				seedFullPosition(t, svc, "PETR4", "stock", 10, 30.0, "", "b3", true, 0)
				seedFullPosition(t, svc, "VALE3", "stock", 5, 100.0, "", "b3", true, -1)
			},
			visible:    true,
			wantHidden: false,
		},
		{
			name: "no-op hide already hidden",
			setup: func(svc *Service) {
				seedFullPosition(t, svc, "PETR4", "stock", 10, 30.0, "", "b3", true, 0)
			},
			visible:    false,
			wantHidden: true,
		},
		{
			name: "no-op show already visible",
			setup: func(svc *Service) {
				seedFullPosition(t, svc, "PETR4", "stock", 10, 30.0, "", "b3", false, 0)
			},
			visible:    true,
			wantHidden: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			tc.setup(svc)

			err := svc.SetPositionsVisibility(context.Background(), tc.visible)
			if err != nil {
				t.Fatalf("SetPositionsVisibility: %v", err)
			}

			positions, err := svc.GetPositions(context.Background())
			if err != nil {
				t.Fatalf("GetPositions: %v", err)
			}
			for _, p := range positions {
				if p.Hidden != tc.wantHidden {
					t.Errorf("position %s: Hidden=%v, want %v", p.Ticker, p.Hidden, tc.wantHidden)
				}
			}
		})
	}
}

// ---- GetLatestImportJob tests ----

func TestGetLatestImportJobNoRows(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.GetLatestImportJob(context.Background())
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestGetLatestImportJobReturnsMostRecent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.DB.ExecContext(ctx,
		`INSERT INTO import_jobs(source, status, detail, created_at, updated_at) VALUES('b3','completed','first','2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert job 1: %v", err)
	}
	_, err = svc.DB.ExecContext(ctx,
		`INSERT INTO import_jobs(source, status, detail, created_at, updated_at) VALUES('manual_b3_export','running','second','2024-01-02T00:00:00Z','2024-01-02T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert job 2: %v", err)
	}

	resp, err := svc.GetLatestImportJob(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != "manual_b3_export" {
		t.Errorf("expected most recent job (manual_b3_export), got %s", resp.Source)
	}
	if resp.Detail != "second" {
		t.Errorf("expected detail 'second', got %s", resp.Detail)
	}
}

func TestGetLatestImportJobNullDetail(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.DB.ExecContext(ctx,
		`INSERT INTO import_jobs(source, status, detail, created_at, updated_at) VALUES('b3','pending',NULL,'2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	resp, err := svc.GetLatestImportJob(ctx)
	if err != nil {
		t.Fatalf("unexpected scan error with NULL detail: %v", err)
	}
	if resp.Detail != "" {
		t.Errorf("expected empty detail for NULL, got %q", resp.Detail)
	}
}

// ---- upsertHoldings tests ----

func TestUpsertHoldingsInsertsNew(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()

	holdings := []models.HoldingPayload{
		{Ticker: "PETR4", Quantity: 10, AveragePrice: 30.0, Broker: "XP", AssetType: "stock", Currency: "BRL"},
	}
	if err := svc.upsertHoldings(ctx, holdings); err != nil {
		t.Fatalf("upsertHoldings: %v", err)
	}

	positions, err := svc.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	p := positions[0]
	if p.Ticker != "PETR4" {
		t.Errorf("ticker: got %q, want PETR4", p.Ticker)
	}
	if p.Quantity != 10 {
		t.Errorf("quantity: got %f, want 10", p.Quantity)
	}
	if p.AvgPrice != 30.0 {
		t.Errorf("avg_price: got %f, want 30.0", p.AvgPrice)
	}
	if p.Broker != "XP" {
		t.Errorf("broker: got %q, want XP", p.Broker)
	}
}

func TestUpsertHoldingsUpdatesExisting(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()

	first := []models.HoldingPayload{
		{Ticker: "PETR4", Quantity: 10, AveragePrice: 30.0, Broker: "XP", AssetType: "stock", Currency: "BRL"},
	}
	if err := svc.upsertHoldings(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := []models.HoldingPayload{
		{Ticker: "PETR4", Quantity: 20, AveragePrice: 35.0, Broker: "Clear", AssetType: "stock", Currency: "BRL"},
	}
	if err := svc.upsertHoldings(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	positions, err := svc.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position (no duplicate), got %d", len(positions))
	}
	p := positions[0]
	if p.Quantity != 20 {
		t.Errorf("quantity: got %f, want 20 (updated)", p.Quantity)
	}
	if p.AvgPrice != 35.0 {
		t.Errorf("avg_price: got %f, want 35.0 (updated)", p.AvgPrice)
	}
	if p.Broker != "Clear" {
		t.Errorf("broker: got %q, want Clear (updated)", p.Broker)
	}
}

func TestUpsertHoldingsMultiple(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()

	holdings := []models.HoldingPayload{
		{Ticker: "PETR4", Quantity: 10, AveragePrice: 30.0, AssetType: "stock", Currency: "BRL"},
		{Ticker: "VALE3", Quantity: 5, AveragePrice: 100.0, AssetType: "stock", Currency: "BRL"},
		{Ticker: "KNRI11", Quantity: 20, AveragePrice: 50.0, AssetType: "fii", Currency: "BRL"},
	}
	if err := svc.upsertHoldings(ctx, holdings); err != nil {
		t.Fatalf("upsertHoldings: %v", err)
	}

	positions, err := svc.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(positions))
	}
}

func TestUpsertHoldingsDefaultCurrency(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()

	holdings := []models.HoldingPayload{
		{Ticker: "PETR4", Quantity: 10, AveragePrice: 30.0, AssetType: "stock", Currency: ""},
	}
	if err := svc.upsertHoldings(ctx, holdings); err != nil {
		t.Fatalf("upsertHoldings: %v", err)
	}

	var currency string
	err := svc.DB.QueryRowContext(ctx, `SELECT currency FROM assets WHERE ticker='PETR4'`).Scan(&currency)
	if err != nil {
		t.Fatalf("query currency: %v", err)
	}
	if currency != "BRL" {
		t.Errorf("currency: got %q, want BRL (default)", currency)
	}
}

func TestUpsertHoldingsPreservesMetadata(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()

	// First upsert with company name
	first := []models.HoldingPayload{
		{Ticker: "PETR4", Quantity: 10, AveragePrice: 30.0, AssetType: "stock", Currency: "BRL", CompanyName: "PETROBRAS", TaxID: "12345"},
	}
	if err := svc.upsertHoldings(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second upsert with blank company name — existing metadata must be preserved
	second := []models.HoldingPayload{
		{Ticker: "PETR4", Quantity: 15, AveragePrice: 32.0, AssetType: "stock", Currency: "BRL", CompanyName: "", TaxID: ""},
	}
	if err := svc.upsertHoldings(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var companyName string
	err := svc.DB.QueryRowContext(ctx,
		`SELECT COALESCE(m.company_name,'') FROM asset_metadata m JOIN assets a ON a.id=m.asset_id WHERE a.ticker='PETR4'`,
	).Scan(&companyName)
	if err != nil {
		t.Fatalf("query metadata: %v", err)
	}
	if companyName != "PETROBRAS" {
		t.Errorf("company_name: got %q, want PETROBRAS (preserved)", companyName)
	}
}
