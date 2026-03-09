package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/db"
	"investments-portfolio-manager/backend/internal/models"
)

func newSentimentTestService(t *testing.T) *Service {
	t.Helper()
	database, err := db.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database, config.Config{
		SentimentEnabled:                true,
		SentimentTTLHours:               24,
		SentimentNewsLookbackDays:       14,
		SentimentTranscriptLookbackDays: 45,
		SentimentMaxSourcesPerTicker:    10,
		SentimentUserAgent:              "test-agent",
	})
}

func seedTrackedAsset(t *testing.T, svc *Service, ticker, assetType, companyName string) models.TrackedAsset {
	t.Helper()
	ctx := context.Background()
	result, err := svc.DB.ExecContext(ctx, `INSERT INTO assets(ticker, asset_type, currency) VALUES(?,?, 'BRL')`, ticker, assetType)
	if err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	assetID, _ := result.LastInsertId()
	if _, err := svc.DB.ExecContext(ctx, `INSERT INTO positions(user_id, asset_id, quantity, avg_price, source) VALUES(1, ?, 10, 100, 'b3')`, assetID); err != nil {
		t.Fatalf("insert position: %v", err)
	}
	if _, err := svc.DB.ExecContext(ctx, `INSERT INTO asset_metadata(asset_id, company_name, tax_id, last_updated) VALUES(?,?,?,?)`, assetID, companyName, "", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert metadata: %v", err)
	}
	return models.TrackedAsset{AssetID: assetID, Ticker: ticker, AssetType: assetType, CompanyName: companyName}
}

func TestGetOrRefreshSentimentReturnsFreshSnapshotWithoutNetwork(t *testing.T) {
	svc := newSentimentTestService(t)
	asset := seedTrackedAsset(t, svc, "PETR4", "stock", "Petroleo Brasileiro")
	now := time.Now().UTC()
	_, err := svc.DB.ExecContext(context.Background(), `
		INSERT INTO sentiment_snapshots(asset_id, status, score, label, confidence, trend, source_count, last_refreshed_at, expires_at, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		asset.AssetID, "ok", 35.0, "positive", 0.8, "flat", 1, now.Format(time.RFC3339), now.Add(24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	sentiment := svc.GetOrRefreshSentiment(context.Background(), asset)
	if sentiment == nil || sentiment.Status != "ok" {
		t.Fatalf("expected fresh sentiment, got %+v", sentiment)
	}
	if sentiment.Score == nil || *sentiment.Score != 35 {
		t.Fatalf("expected stored score, got %+v", sentiment.Score)
	}
}

func TestGetOrRefreshSentimentRefreshesExpiredSnapshot(t *testing.T) {
	svc := newSentimentTestService(t)
	asset := seedTrackedAsset(t, svc, "VALE3", "stock", "Vale")
	server := mockSentimentServer(t)
	defer server.Close()
	restore := overrideSentimentEndpoints(server.URL+"/news", server.URL+"/transcripts")
	defer restore()
	svc.Client = server.Client()

	now := time.Now().UTC()
	_, err := svc.DB.ExecContext(context.Background(), `
		INSERT INTO sentiment_snapshots(asset_id, status, score, label, confidence, trend, source_count, last_refreshed_at, expires_at, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		asset.AssetID, "ok", 5.0, "neutral", 0.5, "flat", 1, now.Add(-48*time.Hour).Format(time.RFC3339), now.Add(-2*time.Hour).Format(time.RFC3339), now.Add(-48*time.Hour).Format(time.RFC3339), now.Add(-48*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert expired snapshot: %v", err)
	}

	sentiment := svc.GetOrRefreshSentiment(context.Background(), asset)
	if sentiment == nil || sentiment.Status == "unavailable" {
		t.Fatalf("expected refreshed sentiment, got %+v", sentiment)
	}
	if sentiment.SourceCount == 0 {
		t.Fatalf("expected sources after refresh")
	}
}

func TestGetOrRefreshSentimentReturnsStaleOnRefreshFailure(t *testing.T) {
	svc := newSentimentTestService(t)
	asset := seedTrackedAsset(t, svc, "ABEV3", "stock", "Ambev")
	now := time.Now().UTC()
	_, err := svc.DB.ExecContext(context.Background(), `
		INSERT INTO sentiment_snapshots(asset_id, status, score, label, confidence, trend, source_count, last_refreshed_at, expires_at, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		asset.AssetID, "ok", 22.0, "neutral", 0.7, "flat", 2, now.Add(-72*time.Hour).Format(time.RFC3339), now.Add(-3*time.Hour).Format(time.RFC3339), now.Add(-72*time.Hour).Format(time.RFC3339), now.Add(-72*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert expired snapshot: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusBadGateway)
	}))
	defer server.Close()
	restore := overrideSentimentEndpoints(server.URL+"/news", server.URL+"/transcripts")
	defer restore()
	svc.Client = server.Client()

	sentiment := svc.GetOrRefreshSentiment(context.Background(), asset)
	if sentiment == nil || sentiment.Status != "stale" {
		t.Fatalf("expected stale fallback, got %+v", sentiment)
	}
	if !sentiment.IsStale {
		t.Fatalf("expected stale marker")
	}
}

func TestGetOrRefreshSentimentReturnsUnavailableWithoutSnapshot(t *testing.T) {
	svc := newSentimentTestService(t)
	asset := seedTrackedAsset(t, svc, "ITUB4", "stock", "Itau Unibanco")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusBadGateway)
	}))
	defer server.Close()
	restore := overrideSentimentEndpoints(server.URL+"/news", server.URL+"/transcripts")
	defer restore()
	svc.Client = server.Client()

	sentiment := svc.GetOrRefreshSentiment(context.Background(), asset)
	if sentiment == nil || sentiment.Status != "unavailable" {
		t.Fatalf("expected unavailable sentiment, got %+v", sentiment)
	}
}

func TestScoreSentimentPositiveAndNegative(t *testing.T) {
	now := time.Now().UTC()
	positive := scoreSentiment([]normalizedSourceItem{{
		SourceType:      "news",
		RawText:         "Vale beat expectations with strong demand and margin expansion",
		PublishedAt:     now,
		Weight:          1,
		MatchConfidence: 0.9,
	}}, nil, now, 24)
	if positive.Score == nil || *positive.Score <= 0 || positive.Label != "positive" {
		t.Fatalf("expected positive score, got %+v", positive)
	}

	negative := scoreSentiment([]normalizedSourceItem{{
		SourceType:      "news",
		RawText:         "Vale reported impairment and guidance cut after weaker quarter",
		PublishedAt:     now,
		Weight:          1,
		MatchConfidence: 0.9,
	}}, nil, now, 24)
	if negative.Score == nil || *negative.Score >= 0 || negative.Label != "negative" {
		t.Fatalf("expected negative score, got %+v", negative)
	}
}

func TestComputeMatchConfidenceSupportsBDRIssuerName(t *testing.T) {
	confidence := computeMatchConfidence(models.TrackedAsset{
		Ticker:      "AAPL34",
		AssetType:   "bdr",
		CompanyName: "Apple Inc",
	}, "Apple reported strong demand in its latest quarter")
	if confidence < 0.4 {
		t.Fatalf("expected issuer-name confidence for BDR, got %f", confidence)
	}
}

func mockSentimentServer(t *testing.T) *httptest.Server {
	t.Helper()
	pubDate := time.Now().UTC().Format(time.RFC1123Z)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/news":
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><rss><channel><item><title>Vale beat estimates with strong demand</title><link>https://example.com/news/vale</link><pubDate>%s</pubDate><description>Margin expansion supports another strong quarter.</description></item></channel></rss>`, pubDate)
		case "/transcripts":
			fmt.Fprint(w, `<html><body><a class="result__a" href="https://example.com/transcript/vale">Vale earnings call transcript points to growth</a><div class="result__snippet">Management highlighted deleveraging and guidance raised.</div></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func overrideSentimentEndpoints(newsURL, transcriptURL string) func() {
	oldNews := newsSearchBaseURL
	oldTranscript := transcriptSearchBaseURL
	newsSearchBaseURL = newsURL
	transcriptSearchBaseURL = transcriptURL
	return func() {
		newsSearchBaseURL = oldNews
		transcriptSearchBaseURL = oldTranscript
	}
}
