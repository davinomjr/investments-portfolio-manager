package services

import (
	"context"
	"testing"

	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/db"
)

func seedPosition(t *testing.T, svc *Service) {
	t.Helper()
	ctx := context.Background()
	_, err := svc.DB.ExecContext(ctx, `INSERT INTO assets(ticker, asset_type, currency) VALUES('PETR4','stock','BRL')`)
	if err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	_, err = svc.DB.ExecContext(ctx, `INSERT INTO positions(user_id, asset_id, quantity, avg_price, source) VALUES(1, 1, 10, 100, 'b3')`)
	if err != nil {
		t.Fatalf("insert position: %v", err)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	database, err := db.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database, config.Config{})
}

func TestParseMonteCarloParamsAcceptsPercentages(t *testing.T) {
	params := ParseMonteCarloParams("15", "2500", "12", "30")
	if params.Years != 15 || params.Simulations != 2500 {
		t.Fatalf("unexpected numeric parsing: %+v", params)
	}
	if params.ExpectedReturn != 0.12 {
		t.Fatalf("expected_return should be converted to decimal, got %f", params.ExpectedReturn)
	}
	if params.Volatility != 0.3 {
		t.Fatalf("volatility should be converted to decimal, got %f", params.Volatility)
	}
}

func TestGetMonteCarloSimulationProducesTimeline(t *testing.T) {
	svc := newTestService(t)
	seedPosition(t, svc)

	resp, err := svc.GetMonteCarloSimulation(context.Background(), ParseMonteCarloParams("5", "400", "0.1", "0.2"))
	if err != nil {
		t.Fatalf("simulation error: %v", err)
	}
	if resp.InitialValue <= 0 {
		t.Fatalf("expected positive initial value, got %f", resp.InitialValue)
	}
	if len(resp.Timeline) != 5 {
		t.Fatalf("expected 5 timeline points, got %d", len(resp.Timeline))
	}
	last := resp.Timeline[len(resp.Timeline)-1]
	if !(last.P10 <= last.P50 && last.P50 <= last.P90) {
		t.Fatalf("percentiles out of order: %+v", last)
	}
	if last.ProbPositive < 0 || last.ProbPositive > 1 {
		t.Fatalf("probability out of bounds: %f", last.ProbPositive)
	}
}

func TestGetMonteCarloSimulationEmptyPortfolio(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.GetMonteCarloSimulation(context.Background(), ParseMonteCarloParams("", "", "", ""))
	if err != nil {
		t.Fatalf("simulation error: %v", err)
	}
	if resp.InitialValue != 0 {
		t.Fatalf("expected zero initial value for empty portfolio, got %f", resp.InitialValue)
	}
	if len(resp.Timeline) != 0 {
		t.Fatalf("expected empty timeline, got %d", len(resp.Timeline))
	}
}
