package services

import (
	"context"
	"log"
	"sort"
	"time"

	"investments-portfolio-manager/backend/internal/models"
)

// snapshotLocation fixes the calendar day a snapshot belongs to. Brazil has
// had no DST since 2019, so a fixed UTC-3 offset avoids depending on tzdata
// being present in the container.
var snapshotLocation = time.FixedZone("BRT", -3*60*60)

// snapshotTimeout bounds the post-import snapshot, which fans out to quote
// providers just like a /portfolio page load.
const snapshotTimeout = 2 * time.Minute

// snapshotAfterImport records today's portfolio snapshot in the background so
// imports return immediately. Failures are logged, never surfaced: a missed
// snapshot only leaves a gap in the history chart.
func (s *Service) snapshotAfterImport() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
		defer cancel()
		if err := s.SnapshotPortfolio(ctx); err != nil {
			log.Printf("portfolio snapshot failed: %v", err)
		}
	}()
}

// SnapshotPortfolio stores every current position, valued with live quotes,
// under today's date. Re-running on the same day replaces that day's rows,
// so the last sync of the day wins (B3 push and IBKR finish minutes apart).
func (s *Service) SnapshotPortfolio(ctx context.Context) error {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()

	positions, err := s.loadEnrichedPositions(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	return s.recordSnapshot(ctx, now.In(snapshotLocation).Format("2006-01-02"), now, positions)
}

func (s *Service) recordSnapshot(ctx context.Context, date string, capturedAt time.Time, positions []models.PositionResponse) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM position_snapshots WHERE snapshot_date = ?`, date); err != nil {
		return err
	}
	captured := capturedAt.UTC().Format(time.RFC3339)
	for _, p := range positions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO position_snapshots(snapshot_date, ticker, asset_type, currency, quantity, avg_price, last_price, cost_basis_brl, market_value_brl, quote_status, captured_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			date, p.Ticker, p.AssetType, p.Currency, p.Quantity, p.AvgPrice, p.LastPrice, p.CostBasisBRL, p.MarketValueBRL, p.QuoteStatus, captured); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetPortfolioHistory returns one point per snapshot day, oldest first, with
// market value, cost basis (amount invested), and value per asset type.
// days > 0 limits the result to the most recent N calendar days.
func (s *Service) GetPortfolioHistory(ctx context.Context, days int) (models.PortfolioHistoryResponse, error) {
	query := `
		SELECT snapshot_date, asset_type, SUM(market_value_brl), SUM(cost_basis_brl)
		FROM position_snapshots`
	args := []any{}
	if days > 0 {
		since := time.Now().In(snapshotLocation).AddDate(0, 0, -(days - 1)).Format("2006-01-02")
		query += ` WHERE snapshot_date >= ?`
		args = append(args, since)
	}
	query += ` GROUP BY snapshot_date, asset_type`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return models.PortfolioHistoryResponse{}, err
	}
	defer rows.Close()

	byDate := map[string]*models.PortfolioHistoryPoint{}
	for rows.Next() {
		var date, assetType string
		var market, cost float64
		if err := rows.Scan(&date, &assetType, &market, &cost); err != nil {
			return models.PortfolioHistoryResponse{}, err
		}
		point, ok := byDate[date]
		if !ok {
			point = &models.PortfolioHistoryPoint{Date: date, ByAssetType: map[string]float64{}}
			byDate[date] = point
		}
		point.MarketValueBRL += market
		point.CostBasisBRL += cost
		point.ByAssetType[assetType] += market
	}
	if err := rows.Err(); err != nil {
		return models.PortfolioHistoryResponse{}, err
	}

	points := make([]models.PortfolioHistoryPoint, 0, len(byDate))
	for _, p := range byDate {
		points = append(points, *p)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Date < points[j].Date })
	return models.PortfolioHistoryResponse{Points: points}, nil
}
