package services

import (
	"context"
	"testing"
	"time"

	"investments-portfolio-manager/backend/internal/models"
)

func snapshotPosition(ticker, assetType string, cost, market float64) models.PositionResponse {
	return models.PositionResponse{
		Ticker:         ticker,
		AssetType:      assetType,
		Currency:       "BRL",
		Quantity:       1,
		AvgPrice:       cost,
		CostBasisBRL:   cost,
		MarketValueBRL: market,
		QuoteStatus:    "live",
	}
}

func TestGetPortfolioHistoryEmpty(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.GetPortfolioHistory(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Points == nil || len(resp.Points) != 0 {
		t.Fatalf("expected empty non-nil points, got %#v", resp.Points)
	}
}

func TestGetPortfolioHistoryAggregatesByDateAndType(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now()

	if err := svc.recordSnapshot(ctx, "2026-09-02", now, []models.PositionResponse{
		snapshotPosition("PETR4", "stock", 100, 120),
		snapshotPosition("XPML11", "fii", 50, 45),
		snapshotPosition("VALE3", "stock", 200, 210),
	}); err != nil {
		t.Fatalf("record day 2: %v", err)
	}
	if err := svc.recordSnapshot(ctx, "2026-09-01", now, []models.PositionResponse{
		snapshotPosition("PETR4", "stock", 100, 110),
	}); err != nil {
		t.Fatalf("record day 1: %v", err)
	}

	resp, err := svc.GetPortfolioHistory(ctx, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(resp.Points))
	}
	first, second := resp.Points[0], resp.Points[1]
	if first.Date != "2026-09-01" || second.Date != "2026-09-02" {
		t.Fatalf("expected oldest first, got %s then %s", first.Date, second.Date)
	}
	if first.MarketValueBRL != 110 || first.CostBasisBRL != 100 {
		t.Errorf("day 1 totals: got market %v cost %v", first.MarketValueBRL, first.CostBasisBRL)
	}
	if second.MarketValueBRL != 375 || second.CostBasisBRL != 350 {
		t.Errorf("day 2 totals: got market %v cost %v", second.MarketValueBRL, second.CostBasisBRL)
	}
	if second.ByAssetType["stock"] != 330 || second.ByAssetType["fii"] != 45 {
		t.Errorf("day 2 by type: got %v", second.ByAssetType)
	}
}

func TestRecordSnapshotSameDayReplacesPreviousRows(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now()

	// B3 push lands first, then the IBKR sync re-snapshots the same day with
	// a position that was sold in between.
	if err := svc.recordSnapshot(ctx, "2026-09-26", now, []models.PositionResponse{
		snapshotPosition("PETR4", "stock", 100, 120),
		snapshotPosition("SOLD3", "stock", 80, 90),
	}); err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	if err := svc.recordSnapshot(ctx, "2026-09-26", now, []models.PositionResponse{
		snapshotPosition("PETR4", "stock", 100, 125),
		snapshotPosition("VUAA", "international_etf", 500, 520),
	}); err != nil {
		t.Fatalf("second snapshot: %v", err)
	}

	resp, err := svc.GetPortfolioHistory(ctx, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(resp.Points))
	}
	p := resp.Points[0]
	if p.MarketValueBRL != 645 || p.CostBasisBRL != 600 {
		t.Errorf("got market %v cost %v, want 645 / 600", p.MarketValueBRL, p.CostBasisBRL)
	}
	if _, ok := p.ByAssetType["stock"]; !ok || p.ByAssetType["stock"] != 125 {
		t.Errorf("stale SOLD3 row survived: by type %v", p.ByAssetType)
	}
}

func TestGetPortfolioHistoryDaysFilter(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now()
	today := now.In(snapshotLocation)

	for _, offset := range []int{0, 5, 40} {
		date := today.AddDate(0, 0, -offset).Format("2006-01-02")
		if err := svc.recordSnapshot(ctx, date, now, []models.PositionResponse{snapshotPosition("PETR4", "stock", 100, 100)}); err != nil {
			t.Fatalf("record %s: %v", date, err)
		}
	}

	resp, err := svc.GetPortfolioHistory(ctx, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Points) != 2 {
		t.Fatalf("expected 2 points within 30 days, got %d", len(resp.Points))
	}
}
