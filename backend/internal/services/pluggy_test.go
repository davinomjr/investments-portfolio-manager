package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/models"
)

func fp(v float64) *float64 { return &v }

func TestPluggyAssetType(t *testing.T) {
	cases := []struct{ typ, sub, want string }{
		{"EQUITY", "STOCK", "stock"},
		{"EQUITY", "REAL_ESTATE_FUND", "fii"},
		{"EQUITY", "BDR", "bdr"},
		{"EQUITY", "OPTION", ""},
		{"ETF", "ETF", "etf"},
		{"MUTUAL_FUND", "FIXED_INCOME_FUND", "fund"},
		{"MUTUAL_FUND", "MULTIMARKET_FUND", "fund"},
		{"FIXED_INCOME", "TREASURY", "government_bond"},
		{"FIXED_INCOME", "CDB", "other"},
		{"COE", "", ""},
	}
	for _, c := range cases {
		if got := pluggyAssetType(c.typ, c.sub); got != c.want {
			t.Errorf("pluggyAssetType(%q,%q)=%q want %q", c.typ, c.sub, got, c.want)
		}
	}
}

func TestPluggyTreasuryTicker(t *testing.T) {
	cases := []struct {
		inv  PluggyInvestment
		want string
	}{
		{PluggyInvestment{Name: "LFT", RateType: "SELIC", DueDate: "2031-03-01T03:00:00.000Z"}, "TDSELIC2031"},
		{PluggyInvestment{Name: "NTNB PRINC", RateType: "IPCA", DueDate: "2040-08-15T03:00:00.000Z"}, "TDIPCA2040"},
		{PluggyInvestment{Name: "LTN", DueDate: "2029-01-01T03:00:00.000Z"}, "TDPREFIXADO2029"},
		{PluggyInvestment{Name: "NTN-B", DueDate: "2035-05-15T03:00:00.000Z"}, "TDIPCA2035"},
	}
	for _, c := range cases {
		got := pluggyTreasuryTicker(c.inv)
		if got != c.want {
			t.Errorf("pluggyTreasuryTicker(%+v)=%q want %q", c.inv, got, c.want)
		}
		if _, _, ok := parseTDTicker(got); !ok {
			t.Errorf("%q is not parseable by parseTDTicker", got)
		}
	}
}

// Shapes mirror real MeuPluggy responses: equities carry a closing price in
// `value` but no amountOriginal; treasury carries ISIN + amountOriginal.
func TestMapPluggyInvestments(t *testing.T) {
	inv := []PluggyInvestment{
		{Code: "BBAS3", Name: "BBAS3", Type: "EQUITY", Subtype: "STOCK", Status: "ACTIVE", Quantity: fp(1300), Value: fp(14.69), Balance: 19097},
		{Code: "XPML11", Name: "XPML11", Type: "EQUITY", Subtype: "REAL_ESTATE_FUND", Status: "ACTIVE", Quantity: fp(190), Value: fp(86.97), Balance: 16524.3},
		{Code: "BRSTNCNTB666", Name: "NTNB PRINC", Type: "FIXED_INCOME", Subtype: "TREASURY", Status: "ACTIVE", Quantity: fp(20), Value: fp(1745.42), AmountOriginal: fp(36000), Balance: 34908.4, RateType: "IPCA", DueDate: "2040-08-15T03:00:00.000Z"},
		{Code: "30.178.843/0001-43", Name: "Plural Debentures Incentivadas Hedge 30", Type: "MUTUAL_FUND", Subtype: "MULTIMARKET_FUND", Status: "ACTIVE", Quantity: fp(2519.65), Value: fp(1.98), Balance: 4993.03},
		{Code: "HGRU11", Type: "EQUITY", Subtype: "REAL_ESTATE_FUND", Status: "TOTAL_WITHDRAWAL", Quantity: fp(0)},
		{Code: "PETRA30", Type: "EQUITY", Subtype: "OPTION", Status: "ACTIVE", Quantity: fp(100)},
	}
	holdings, skipped := mapPluggyInvestments(inv)
	if len(holdings) != 4 || len(skipped) != 2 {
		t.Fatalf("got %d holdings, %d skipped: %+v %+v", len(holdings), len(skipped), holdings, skipped)
	}
	byTicker := map[string]models.HoldingPayload{}
	for _, h := range holdings {
		byTicker[h.Ticker] = h
	}
	if h := byTicker["BBAS3"]; h.AssetType != "stock" || h.Quantity != 1300 || h.AveragePrice != 14.69 {
		t.Errorf("BBAS3: %+v", h)
	}
	if h := byTicker["XPML11"]; h.AssetType != "fii" {
		t.Errorf("XPML11: %+v", h)
	}
	if h := byTicker["TDIPCA2040"]; h.AssetType != "government_bond" || h.AveragePrice != 1800 {
		t.Errorf("treasury should use amountOriginal/quantity: %+v", h)
	}
	if h := byTicker["PLURALDEBENTURESINCENTIVADASHEDGE30"]; h.AssetType != "fund" {
		t.Errorf("fund ticker from name: %+v", byTicker)
	}
}

func TestMapPluggyMergesSameTicker(t *testing.T) {
	holdings, _ := mapPluggyInvestments([]PluggyInvestment{
		{Code: "PETR4", Type: "EQUITY", Subtype: "STOCK", Quantity: fp(100), Value: fp(30)},
		{Code: "PETR4", Type: "EQUITY", Subtype: "STOCK", Quantity: fp(100), Value: fp(40)},
	})
	if len(holdings) != 1 || holdings[0].Quantity != 200 || holdings[0].AveragePrice != 35 {
		t.Fatalf("unexpected merge: %+v", holdings)
	}
}

func TestManualAvgPriceSurvivesSync(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()
	sync := func(price float64) {
		t.Helper()
		if err := svc.upsertHoldings(ctx, []models.HoldingPayload{{Ticker: "BBAS3", Quantity: 1300, AveragePrice: price, AssetType: "stock"}}, "pluggy"); err != nil {
			t.Fatal(err)
		}
	}
	avg := func() (float64, bool) {
		t.Helper()
		var v float64
		var manual bool
		if err := svc.DB.QueryRowContext(ctx, `SELECT avg_price, avg_price_manual IS NOT NULL FROM positions`).Scan(&v, &manual); err != nil {
			t.Fatal(err)
		}
		return v, manual
	}

	sync(14.69)
	if err := svc.SetManualAvgPrice(ctx, "bbas3", fp(21.5)); err != nil {
		t.Fatal(err)
	}
	sync(15.10)
	if v, manual := avg(); v != 21.5 || !manual {
		t.Fatalf("manual price lost after sync: %v manual=%v", v, manual)
	}

	if err := svc.SetManualAvgPrice(ctx, "BBAS3", nil); err != nil {
		t.Fatal(err)
	}
	sync(15.20)
	if v, manual := avg(); v != 15.20 || manual {
		t.Fatalf("cleared override should restore source price: %v manual=%v", v, manual)
	}

	if err := svc.SetManualAvgPrice(ctx, "XXXX3", fp(1)); !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("want ErrPositionNotFound, got %v", err)
	}
	if err := svc.SetManualAvgPrice(ctx, "BBAS3", fp(-1)); err == nil {
		t.Fatal("negative price should be rejected")
	}
}

func newPluggyServer(t *testing.T, itemStatus string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["clientId"] != "id" || body["clientSecret"] != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"apiKey": "k"})
			return
		}
		if r.Header.Get("X-API-KEY") != "k" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/items/item-1":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "item-1", "status": itemStatus, "lastUpdatedAt": "2026-11-26T11:44:00Z"})
		case "/investments":
			page := r.URL.Query().Get("page")
			code := map[string]string{"1": "ITSA4", "2": "BBAS3"}[page]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"totalPages": 2,
				"results":    []map[string]any{{"code": code, "type": "EQUITY", "subtype": "STOCK", "status": "ACTIVE", "quantity": 1, "value": 10, "balance": 10}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchPluggyInvestmentsAuthAndPagination(t *testing.T) {
	srv := newPluggyServer(t, "UPDATED")
	svc := &Service{Config: config.Config{PluggyClientID: "id", PluggyClientSecret: "secret", PluggyItemIDs: []string{"item-1"}, PluggyBaseURL: srv.URL}}
	got, err := svc.fetchPluggyInvestments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Code != "ITSA4" || got[1].Code != "BBAS3" || got[0].ItemID != "item-1" {
		t.Fatalf("unexpected investments: %+v", got)
	}

	svc.Config.PluggyClientSecret = "wrong"
	if _, err := svc.fetchPluggyInvestments(context.Background()); err == nil {
		t.Fatal("expected auth error")
	}
}

func TestFetchPluggyRejectsUnhealthyItem(t *testing.T) {
	srv := newPluggyServer(t, "LOGIN_ERROR")
	svc := &Service{Config: config.Config{PluggyClientID: "id", PluggyClientSecret: "secret", PluggyItemIDs: []string{"item-1"}, PluggyBaseURL: srv.URL}}
	if _, err := svc.fetchPluggyInvestments(context.Background()); err == nil {
		t.Fatal("expected error for LOGIN_ERROR item")
	}
}

func TestImportPluggyReplacesB3(t *testing.T) {
	srv := newPluggyServer(t, "UPDATED")
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1, PluggyClientID: "id", PluggyClientSecret: "secret", PluggyItemIDs: []string{"item-1"}, PluggyBaseURL: srv.URL, PluggyReplacesB3: true})
	ctx := context.Background()
	if err := svc.upsertHoldings(ctx, []models.HoldingPayload{
		{Ticker: "BBAS3", Quantity: 5, AveragePrice: 9, AssetType: "stock"},
		{Ticker: "OLD3", Quantity: 1, AveragePrice: 1, AssetType: "stock"},
	}, "b3"); err != nil {
		t.Fatal(err)
	}
	if err := svc.upsertHoldings(ctx, []models.HoldingPayload{{Ticker: "AAPL", Quantity: 1, AveragePrice: 100, AssetType: "international_stock", Currency: "USD"}}, "ibkr"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportPluggy(ctx); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		var status string
		_ = svc.DB.QueryRowContext(ctx, `SELECT status FROM import_jobs ORDER BY id DESC LIMIT 1`).Scan(&status)
		if status == "completed" {
			break
		}
		if status == "failed" || time.Now().After(deadline) {
			t.Fatalf("job status %q", status)
		}
		time.Sleep(20 * time.Millisecond)
	}

	rows, err := svc.DB.QueryContext(ctx, `SELECT a.ticker, p.source FROM positions p JOIN assets a ON a.id=p.asset_id ORDER BY a.ticker`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var ticker, source string
		_ = rows.Scan(&ticker, &source)
		got[ticker] = source
	}
	want := map[string]string{"AAPL": "ibkr", "BBAS3": "pluggy", "ITSA4": "pluggy"}
	if len(got) != len(want) {
		t.Fatalf("positions: got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("positions: got %v want %v", got, want)
		}
	}
}
