package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Quote is a snapshot of an asset's price returned by the quote provider.
type Quote struct {
	Ticker        string    `json:"ticker"`
	LastPrice     float64   `json:"last_price"`
	PreviousClose float64   `json:"previous_close"`
	Currency      string    `json:"currency"`
	FetchedAt     time.Time `json:"fetched_at"`
	Stale         bool      `json:"stale"`
}

type quoteCacheEntry struct {
	quote    Quote
	cachedAt time.Time
}

// quoteRequest carries the inputs needed to fetch and persist one quote.
type quoteRequest struct {
	AssetID   int64
	Ticker    string
	Currency  string
	AssetType string
}

// isQuotableAssetType reports whether any provider has data for this asset
// type. IBKR's `international_bond` doesn't have a free public price source
// available, so we still skip those.
func isQuotableAssetType(t string) bool {
	switch t {
	case "international_bond":
		return false
	}
	return true
}

// yahooSymbolOverrides maps tickers that don't resolve under the default
// suffix rule to the symbol Yahoo expects. European UCITS ETFs held via
// IBKR are the main case (e.g., VUAA is listed on London as VUAA.L).
var yahooSymbolOverrides = map[string]string{
	"VUAA": "VUAA.L",
}

// FetchQuotes returns last-known prices keyed by ticker for the given requests.
// Lookup order: in-memory cache (within TTL) → Yahoo HTTP → SQLite fallback.
// When the network call fails and a row exists in asset_quotes, the returned
// quote has Stale=true. Missing keys in the result mean no quote was ever
// recorded for that asset.
func (s *Service) FetchQuotes(ctx context.Context, requests []quoteRequest) map[string]Quote {
	if !s.Config.QuotesEnabled || len(requests) == 0 {
		return nil
	}
	out := make(map[string]Quote, len(requests))
	var mu sync.Mutex

	needsFetch := make([]quoteRequest, 0, len(requests))
	for _, r := range requests {
		if q, ok := s.lookupCachedQuote(r.Ticker); ok {
			out[r.Ticker] = q
			continue
		}
		needsFetch = append(needsFetch, r)
	}

	concurrency := s.Config.QuotesConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, r := range needsFetch {
		r := r
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			q, err := s.fetchOneQuote(ctx, r)
			if err == nil {
				s.storeQuote(ctx, r.AssetID, q)
				mu.Lock()
				out[r.Ticker] = q
				mu.Unlock()
				return
			}
			log.Printf("quote fetch failed for %s: %v", r.Ticker, err)
			if cached, ok := s.loadDBQuote(ctx, r.AssetID, r.Ticker); ok {
				cached.Stale = true
				mu.Lock()
				out[r.Ticker] = cached
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return out
}

// fetchOneQuote routes a single quote request to the right provider based on
// the asset type. Government bonds go to Tesouro Direto; everything else to
// Yahoo Finance.
func (s *Service) fetchOneQuote(ctx context.Context, r quoteRequest) (Quote, error) {
	if r.AssetType == "government_bond" {
		return s.fetchTesouroDiretoQuote(ctx, r.Ticker)
	}
	return s.fetchYahooQuote(ctx, r.Ticker, r.Currency)
}

func (s *Service) lookupCachedQuote(ticker string) (Quote, bool) {
	s.quotesMu.RLock()
	defer s.quotesMu.RUnlock()
	entry, ok := s.quotesCache[ticker]
	if !ok {
		return Quote{}, false
	}
	if time.Since(entry.cachedAt) > s.Config.QuotesTTL {
		return Quote{}, false
	}
	return entry.quote, true
}

func (s *Service) loadDBQuote(ctx context.Context, assetID int64, ticker string) (Quote, bool) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT last_price, COALESCE(previous_close, 0), currency, fetched_at
		FROM asset_quotes WHERE asset_id = ?`, assetID)
	var q Quote
	var fetchedAt string
	if err := row.Scan(&q.LastPrice, &q.PreviousClose, &q.Currency, &fetchedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("asset_quotes read failed for %s: %v", ticker, err)
		}
		return Quote{}, false
	}
	q.Ticker = ticker
	if t, err := time.Parse(time.RFC3339, fetchedAt); err == nil {
		q.FetchedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", fetchedAt); err == nil {
		q.FetchedAt = t
	}
	return q, true
}

func (s *Service) storeQuote(ctx context.Context, assetID int64, q Quote) {
	s.quotesMu.Lock()
	s.quotesCache[q.Ticker] = quoteCacheEntry{quote: q, cachedAt: time.Now()}
	s.quotesMu.Unlock()

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO asset_quotes(asset_id, last_price, previous_close, currency, fetched_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(asset_id) DO UPDATE SET
			last_price = excluded.last_price,
			previous_close = excluded.previous_close,
			currency = excluded.currency,
			fetched_at = excluded.fetched_at`,
		assetID, q.LastPrice, q.PreviousClose, q.Currency, q.FetchedAt.UTC().Format(time.RFC3339))
	if err != nil {
		log.Printf("asset_quotes upsert failed for %s: %v", q.Ticker, err)
	}
}

// yahooSymbol maps a portfolio ticker + currency to the symbol Yahoo expects.
// Brazilian B3 tickers need a `.SA` suffix; US tickers are used as-is. An
// overrides map handles tickers that don't fit either rule.
func yahooSymbol(ticker, currency string) string {
	if s, ok := yahooSymbolOverrides[ticker]; ok {
		return s
	}
	if currency == "BRL" || currency == "" {
		return ticker + ".SA"
	}
	return ticker
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency            string  `json:"currency"`
				Symbol              string  `json:"symbol"`
				RegularMarketPrice  float64 `json:"regularMarketPrice"`
				ChartPreviousClose  float64 `json:"chartPreviousClose"`
				PreviousClose       float64 `json:"previousClose"`
				RegularMarketTime   int64   `json:"regularMarketTime"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (s *Service) fetchYahooQuote(ctx context.Context, ticker, currency string) (Quote, error) {
	symbol := yahooSymbol(ticker, currency)
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", symbol)

	reqCtx, cancel := context.WithTimeout(ctx, s.Config.QuotesHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return Quote{}, err
	}
	// Yahoo 403s the default Go UA.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PortfolioManager/1.0)")

	resp, err := s.Client.Do(req)
	if err != nil {
		return Quote{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("yahoo http %d", resp.StatusCode)
	}

	var body yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Quote{}, err
	}
	if body.Chart.Error != nil {
		return Quote{}, fmt.Errorf("yahoo error: %s", body.Chart.Error.Description)
	}
	if len(body.Chart.Result) == 0 {
		return Quote{}, fmt.Errorf("yahoo empty result for %s", symbol)
	}
	meta := body.Chart.Result[0].Meta
	if meta.RegularMarketPrice <= 0 {
		return Quote{}, fmt.Errorf("yahoo non-positive price for %s", symbol)
	}
	prev := meta.ChartPreviousClose
	if prev == 0 {
		prev = meta.PreviousClose
	}
	return Quote{
		Ticker:        ticker,
		LastPrice:     meta.RegularMarketPrice,
		PreviousClose: prev,
		Currency:      meta.Currency,
		FetchedAt:     time.Now(),
	}, nil
}
