package services

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"investments-portfolio-manager/backend/internal/config"
)

// fakeTransport serves canned responses keyed by URL substring and counts hits.
type fakeTransport struct {
	mu     sync.Mutex
	routes map[string]func() (int, string)
	hits   map[string]int
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	url := req.URL.String()
	for key, route := range f.routes {
		if strings.Contains(url, key) {
			f.hits[key]++
			status, body := route()
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: req}, nil
		}
	}
	f.hits["unmatched:"+url]++
	return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: req}, nil
}

func (f *fakeTransport) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[key]
}

func yahooBody(currency string, price float64) string {
	return `{"chart":{"result":[{"meta":{"currency":"` + currency + `","regularMarketPrice":` + strconv.FormatFloat(price, 'f', -1, 64) + `,"chartPreviousClose":1}}]}}`
}

func newQuoteTestService(t *testing.T, ft *fakeTransport) *Service {
	t.Helper()
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1, QuotesEnabled: true, QuotesTTL: time.Minute, QuotesHTTPTimeout: time.Second, QuotesConcurrency: 2})
	svc.Client = &http.Client{Transport: ft}
	return svc
}

func TestFundsAreNotQuoted(t *testing.T) {
	if isQuotableAssetType("fund") || isQuotableAssetType("international_bond") {
		t.Fatal("fund and international_bond must not be quoted")
	}
	if !isQuotableAssetType("stock") || !isQuotableAssetType("government_bond") {
		t.Fatal("stock and government_bond must be quoted")
	}
}

func TestYahooFallsBackToLondonListing(t *testing.T) {
	ft := &fakeTransport{hits: map[string]int{}, routes: map[string]func() (int, string){
		"/chart/XUSE.L?": func() (int, string) { return 200, yahooBody("USD", 41.5) },
		"/chart/XUSE?":   func() (int, string) { return 404, "" },
	}}
	svc := newQuoteTestService(t, ft)
	ctx := context.Background()

	q, err := svc.fetchYahooQuote(ctx, "XUSE", "USD", "international_etf")
	if err != nil || q.LastPrice != 41.5 || q.Currency != "USD" {
		t.Fatalf("got %+v, %v", q, err)
	}
	// Second lookup goes straight to the resolved London symbol.
	if _, err := svc.fetchYahooQuote(ctx, "XUSE", "USD", "international_etf"); err != nil {
		t.Fatal(err)
	}
	if ft.count("/chart/XUSE?") != 1 || ft.count("/chart/XUSE.L?") != 2 {
		t.Fatalf("hits: %v", ft.hits)
	}
}

func TestYahooRejectsFallbackInOtherCurrency(t *testing.T) {
	ft := &fakeTransport{hits: map[string]int{}, routes: map[string]func() (int, string){
		"/chart/IDTP.L?": func() (int, string) { return 200, yahooBody("GBp", 450) },
		"/chart/IDTP?":   func() (int, string) { return 404, "" },
	}}
	svc := newQuoteTestService(t, ft)
	if _, err := svc.fetchYahooQuote(context.Background(), "IDTP", "USD", "international_etf"); err == nil {
		t.Fatal("a GBp London quote must not price a USD position")
	}
}

func TestFailedQuoteIsBackedOff(t *testing.T) {
	ft := &fakeTransport{hits: map[string]int{}, routes: map[string]func() (int, string){
		"/chart/CSBGU0": func() (int, string) { return 404, "" },
	}}
	svc := newQuoteTestService(t, ft)
	ctx := context.Background()
	req := []quoteRequest{{AssetID: 1, Ticker: "CSBGU0", Currency: "USD", AssetType: "international_etf"}}

	svc.FetchQuotes(ctx, req)
	first := ft.count("/chart/CSBGU0")
	svc.FetchQuotes(ctx, req)
	svc.FetchQuotes(ctx, req)
	if first == 0 || ft.count("/chart/CSBGU0") != first {
		t.Fatalf("failed ticker should not be refetched within the backoff: first=%d total=%d", first, ft.count("/chart/CSBGU0"))
	}

	// After the backoff expires it is tried again.
	svc.quotesMu.Lock()
	svc.quoteFailures["CSBGU0"] = time.Now().Add(-quoteFailureBackoff - time.Second)
	svc.quotesMu.Unlock()
	svc.FetchQuotes(ctx, req)
	if ft.count("/chart/CSBGU0") == first {
		t.Fatal("ticker should be retried after the backoff")
	}
}

func TestUSDToBRLFailureIsBackedOff(t *testing.T) {
	ft := &fakeTransport{hits: map[string]int{}, routes: map[string]func() (int, string){
		"awesomeapi":  func() (int, string) { return 429, "" },
		"frankfurter": func() (int, string) { return 503, "" },
	}}
	svc := newQuoteTestService(t, ft)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rate := svc.getUSDToBRL(ctx); rate != fallbackUSDToBRL {
				t.Errorf("rate=%v want fallback %v", rate, fallbackUSDToBRL)
			}
		}()
	}
	wg.Wait()
	if ft.count("awesomeapi") != 1 || ft.count("frankfurter") != 1 {
		t.Fatalf("concurrent callers should share one fetch and then back off: %v", ft.hits)
	}
}

func TestUSDToBRLSuccessIsShared(t *testing.T) {
	ft := &fakeTransport{hits: map[string]int{}, routes: map[string]func() (int, string){
		"awesomeapi": func() (int, string) { return 200, `{"USDBRL":{"bid":"5.40"}}` },
	}}
	svc := newQuoteTestService(t, ft)
	for i := 0; i < 3; i++ {
		if rate := svc.getUSDToBRL(context.Background()); rate != 5.40 {
			t.Fatalf("rate=%v", rate)
		}
	}
	if ft.count("awesomeapi") != 1 {
		t.Fatalf("rate should be cached: %v", ft.hits)
	}
}

func TestUnquotableAssetsDontMakeQuotesPartial(t *testing.T) {
	ft := &fakeTransport{hits: map[string]int{}, routes: map[string]func() (int, string){
		"/chart/PETR4.SA": func() (int, string) { return 200, yahooBody("BRL", 40) },
		"awesomeapi":      func() (int, string) { return 200, `{"USDBRL":{"bid":"5.40"}}` },
	}}
	svc := newQuoteTestService(t, ft)
	seedFullPosition(t, svc, "PETR4", "stock", 10, 30.0, "", "pluggy", false, 0)
	seedFullPosition(t, svc, "SPARTADEB", "fund", 100, 1.85, "", "pluggy", false, 0)

	resp, err := svc.GetPortfolio(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.QuotesStatus != "live" {
		t.Fatalf("QuotesStatus=%q want live (fund has no quote source)", resp.QuotesStatus)
	}
	if ft.count("SPARTADEB") != 0 {
		t.Fatalf("fund must not be sent to Yahoo: %v", ft.hits)
	}
}
