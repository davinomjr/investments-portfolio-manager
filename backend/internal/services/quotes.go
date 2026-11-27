package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
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
//
// Brazilian funds (`fund`) have no exchange ticker — their identifier is the
// fund name/CNPJ — so Yahoo always 404s them; their value comes from the
// import's own price instead.
func isQuotableAssetType(t string) bool {
	switch t {
	case "international_bond", "fund":
		return false
	}
	return true
}

// quoteFailureBackoff is how long a ticker whose fetch failed is served from
// the last stored quote (or left missing) before the provider is tried again.
const quoteFailureBackoff = 30 * time.Minute

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
		if s.recentlyFailed(r.Ticker) {
			if cached, ok := s.loadDBQuote(ctx, r.AssetID, r.Ticker); ok {
				cached.Stale = true
				out[r.Ticker] = cached
			}
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
			// Tesouro's snapshot is still loading right after startup; that's
			// not the ticker's fault, so don't back off from it.
			if !errors.Is(err, errTDSnapshotWarming) {
				s.markQuoteFailed(r.Ticker)
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
	return s.fetchYahooQuote(ctx, r.Ticker, r.Currency, r.AssetType)
}

func (s *Service) recentlyFailed(ticker string) bool {
	s.quotesMu.RLock()
	defer s.quotesMu.RUnlock()
	at, ok := s.quoteFailures[ticker]
	return ok && time.Since(at) < quoteFailureBackoff
}

func (s *Service) markQuoteFailed(ticker string) {
	s.quotesMu.Lock()
	s.quoteFailures[ticker] = time.Now()
	s.quotesMu.Unlock()
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
	delete(s.quoteFailures, q.Ticker)
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

// yahooSymbolCandidates lists the symbols to try for a ticker, in order.
// International ETFs held via IBKR are often UCITS funds listed only in
// London (VUAA → VUAA.L), so those also try the `.L` listing.
func yahooSymbolCandidates(ticker, currency, assetType string) []string {
	primary := yahooSymbol(ticker, currency)
	if _, overridden := yahooSymbolOverrides[ticker]; overridden || primary != ticker {
		return []string{primary}
	}
	if strings.HasPrefix(assetType, "international_") {
		return []string{ticker, ticker + ".L"}
	}
	return []string{primary}
}

// errYahooNotFound marks a symbol Yahoo doesn't know, so the next candidate
// can be tried.
var errYahooNotFound = errors.New("yahoo symbol not found")

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency           string  `json:"currency"`
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				PreviousClose      float64 `json:"previousClose"`
				RegularMarketTime  int64   `json:"regularMarketTime"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (s *Service) fetchYahooQuote(ctx context.Context, ticker, currency, assetType string) (Quote, error) {
	candidates := yahooSymbolCandidates(ticker, currency, assetType)
	s.quotesMu.RLock()
	resolved, known := s.yahooResolved[ticker]
	s.quotesMu.RUnlock()
	if known {
		candidates = []string{resolved}
	}

	var lastErr error
	for i, symbol := range candidates {
		q, err := s.fetchYahooSymbol(ctx, ticker, symbol)
		if err == nil && i > 0 && !strings.EqualFold(q.Currency, currency) {
			// An alternate listing quoted in another currency (e.g. GBp in
			// London for a USD position) would misprice the holding.
			err = fmt.Errorf("%w: %s quoted in %s, position in %s", errYahooNotFound, symbol, q.Currency, currency)
		}
		if err == nil {
			if !known && i > 0 {
				s.quotesMu.Lock()
				s.yahooResolved[ticker] = symbol
				s.quotesMu.Unlock()
			}
			return q, nil
		}
		lastErr = err
		if !errors.Is(err, errYahooNotFound) {
			break // network/server error: don't try other listings
		}
	}
	return Quote{}, lastErr
}

func (s *Service) fetchYahooSymbol(ctx context.Context, ticker, symbol string) (Quote, error) {
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
	if resp.StatusCode == http.StatusNotFound {
		return Quote{}, fmt.Errorf("%w: %s (yahoo http 404)", errYahooNotFound, symbol)
	}
	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("yahoo http %d for %s", resp.StatusCode, symbol)
	}

	var body yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Quote{}, err
	}
	if body.Chart.Error != nil {
		return Quote{}, fmt.Errorf("%w: %s (%s)", errYahooNotFound, symbol, body.Chart.Error.Description)
	}
	if len(body.Chart.Result) == 0 {
		return Quote{}, fmt.Errorf("%w: empty result for %s", errYahooNotFound, symbol)
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
