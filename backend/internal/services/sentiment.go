package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	neturl "net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"investments-portfolio-manager/backend/internal/models"
)

var (
	newsSearchBaseURL       = "https://news.google.com/rss/search"
	transcriptSearchBaseURL = "https://html.duckduckgo.com/html/"
	linkTagPattern          = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	snippetPattern          = regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>(.*?)</a>|<div[^>]*class="result__snippet"[^>]*>(.*?)</div>`)
	tagPattern              = regexp.MustCompile(`<[^>]+>`)
	spacePattern            = regexp.MustCompile(`\s+`)
)

type sentimentSnapshotRecord struct {
	ID              int64
	AssetID         int64
	Status          string
	Score           *float64
	Label           string
	Confidence      *float64
	Trend           string
	SourceCount     int
	LastRefreshedAt string
	ExpiresAt       string
	LastError       string
	Message         string
	UpdatedAt       string
	Sources         []models.SentimentSource
}

type trackedAssetWithID = models.TrackedAsset

type normalizedSourceItem struct {
	SourceType      string
	Provider        string
	Title           string
	URL             string
	PublishedAt     time.Time
	Language        string
	Excerpt         string
	Ticker          string
	CompanyName     string
	RawText         string
	MatchConfidence float64
	Weight          float64
	SignalTags      []string
	Score           float64
}

type scoredSentiment struct {
	Status      string
	Label       string
	Score       *float64
	Confidence  *float64
	Trend       string
	SourceCount int
	WindowStart string
	WindowEnd   string
	Message     string
	IsStale     bool
}

type sentimentAdapter interface {
	Name() string
	Fetch(ctx context.Context, asset trackedAssetWithID, now time.Time) ([]normalizedSourceItem, error)
}

type newsAdapter struct {
	service *Service
}

type transcriptAdapter struct {
	service *Service
}

type googleNewsRSS struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

func (s *Service) GetOrRefreshSentiment(ctx context.Context, asset models.TrackedAsset) *models.TickerSentiment {
	if !s.Config.SentimentEnabled {
		return &models.TickerSentiment{
			Status:  "unavailable",
			Message: "Sentiment refresh is disabled.",
			Sources: []models.SentimentSource{},
		}
	}

	snapshot, err := s.loadSentimentSnapshot(ctx, asset.AssetID)
	if err == nil && snapshot != nil && !snapshot.isExpired() {
		return snapshot.toAPIModel()
	}

	if err == nil && snapshot != nil && snapshot.Status == "refreshing" && snapshot.isRecentlyRefreshing() {
		model := snapshot.toAPIModel()
		if len(model.Sources) > 0 {
			model.IsStale = true
			model.Status = "stale"
		}
		return model
	}

	refreshed, refreshErr := s.refreshSentimentSnapshot(ctx, asset, snapshot)
	if refreshErr == nil {
		return refreshed.toAPIModel()
	}

	if snapshot != nil {
		model := snapshot.toAPIModel()
		model.IsStale = true
		model.Status = "stale"
		model.Message = firstNonEmpty(refreshErr.Error(), model.Message)
		return model
	}

	return &models.TickerSentiment{
		Status:  "unavailable",
		Message: firstNonEmpty(refreshErr.Error(), "Sentiment is unavailable right now."),
		Sources: []models.SentimentSource{},
	}
}

func (s *Service) loadSentimentSnapshot(ctx context.Context, assetID int64) (*sentimentSnapshotRecord, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, asset_id, status, score, COALESCE(label,''), confidence, COALESCE(trend,''), source_count,
		       COALESCE(last_refreshed_at,''), COALESCE(expires_at,''), COALESCE(last_error,''), COALESCE(updated_at,'')
		FROM sentiment_snapshots
		WHERE asset_id=?`, assetID)

	record := &sentimentSnapshotRecord{}
	var score, confidence sql.NullFloat64
	if err := row.Scan(
		&record.ID,
		&record.AssetID,
		&record.Status,
		&score,
		&record.Label,
		&confidence,
		&record.Trend,
		&record.SourceCount,
		&record.LastRefreshedAt,
		&record.ExpiresAt,
		&record.LastError,
		&record.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if score.Valid {
		record.Score = &score.Float64
	}
	if confidence.Valid {
		record.Confidence = &confidence.Float64
	}

	sourceRows, err := s.DB.QueryContext(ctx, `
		SELECT source_type, provider, title, url, COALESCE(published_at,''), COALESCE(excerpt,''), sentiment_score, weight
		FROM sentiment_sources
		WHERE snapshot_id=?
		ORDER BY COALESCE(published_at, created_at) DESC, id DESC`, record.ID)
	if err != nil {
		return nil, err
	}
	defer sourceRows.Close()

	record.Sources = []models.SentimentSource{}
	for sourceRows.Next() {
		var source models.SentimentSource
		var sourceScore sql.NullFloat64
		if err := sourceRows.Scan(
			&source.SourceType,
			&source.Provider,
			&source.Title,
			&source.URL,
			&source.PublishedAt,
			&source.Excerpt,
			&sourceScore,
			&source.Weight,
		); err != nil {
			return nil, err
		}
		if sourceScore.Valid {
			source.Score = &sourceScore.Float64
		}
		record.Sources = append(record.Sources, source)
	}
	return record, sourceRows.Err()
}

func (s *Service) refreshSentimentSnapshot(ctx context.Context, asset trackedAssetWithID, previous *sentimentSnapshotRecord) (*sentimentSnapshotRecord, error) {
	logID := s.insertRefreshLog(ctx, asset.AssetID)
	if err := s.markSnapshotRefreshing(ctx, asset.AssetID); err != nil {
		return previous, err
	}

	now := time.Now().UTC()
	items, fetchErr := s.fetchSentimentSources(ctx, asset, now)
	if fetchErr != nil && len(items) == 0 {
		s.finishRefreshLog(ctx, logID, "failed", 0, fetchErr.Error())
		if err := s.persistRefreshFailure(ctx, asset.AssetID, previous, fetchErr); err != nil {
			return previous, err
		}
		return s.loadSentimentSnapshot(ctx, asset.AssetID)
	}

	aggregate := scoreSentiment(items, previous, now, s.Config.SentimentTTLHours)
	snapshot, err := s.saveSentimentSnapshot(ctx, asset, aggregate, items, fetchErr)
	if err != nil {
		s.finishRefreshLog(ctx, logID, "failed", len(items), err.Error())
		if previous != nil {
			return previous, err
		}
		return nil, err
	}
	s.finishRefreshLog(ctx, logID, snapshot.Status, len(items), "")
	return snapshot, nil
}

func (s *Service) fetchSentimentSources(ctx context.Context, asset trackedAssetWithID, now time.Time) ([]normalizedSourceItem, error) {
	adapters := []sentimentAdapter{
		newsAdapter{service: s},
		transcriptAdapter{service: s},
	}

	var items []normalizedSourceItem
	var errs []string
	seen := map[string]struct{}{}
	for _, adapter := range adapters {
		found, err := adapter.Fetch(ctx, asset, now)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", adapter.Name(), err))
		}
		for _, item := range found {
			if _, exists := seen[item.URL]; exists {
				continue
			}
			seen[item.URL] = struct{}{}
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})

	maxSources := s.Config.SentimentMaxSourcesPerTicker
	if maxSources <= 0 {
		maxSources = 10
	}
	if len(items) > maxSources {
		items = items[:maxSources]
	}

	if len(items) == 0 && len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	if len(items) == 0 {
		return nil, nil
	}
	if len(errs) > 0 {
		return items, errors.New(strings.Join(errs, "; "))
	}
	return items, nil
}

func (s *Service) saveSentimentSnapshot(ctx context.Context, asset trackedAssetWithID, aggregate scoredSentiment, items []normalizedSourceItem, fetchErr error) (*sentimentSnapshotRecord, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	lastError := ""
	if fetchErr != nil {
		lastError = fetchErr.Error()
	}

	var snapshotID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM sentiment_snapshots WHERE asset_id=?`, asset.AssetID).Scan(&snapshotID)
	if err == sql.ErrNoRows {
		result, insertErr := tx.ExecContext(ctx, `
			INSERT INTO sentiment_snapshots(
				asset_id, status, score, label, confidence, trend, source_count, window_start, window_end,
				last_refreshed_at, expires_at, last_error, created_at, updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			asset.AssetID, aggregate.Status, aggregate.Score, nullIfEmpty(aggregate.Label), aggregate.Confidence,
			nullIfEmpty(aggregate.Trend), aggregate.SourceCount, nullIfEmpty(aggregate.WindowStart), nullIfEmpty(aggregate.WindowEnd),
			now, time.Now().UTC().Add(time.Duration(maxInt(s.Config.SentimentTTLHours, 1))*time.Hour).Format(time.RFC3339),
			nullIfEmpty(lastError), now, now,
		)
		if insertErr != nil {
			return nil, insertErr
		}
		snapshotID, _ = result.LastInsertId()
	} else if err != nil {
		return nil, err
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE sentiment_snapshots
			SET status=?, score=?, label=?, confidence=?, trend=?, source_count=?, window_start=?, window_end=?,
			    last_refreshed_at=?, expires_at=?, last_error=?, updated_at=?
			WHERE id=?`,
			aggregate.Status, aggregate.Score, nullIfEmpty(aggregate.Label), aggregate.Confidence,
			nullIfEmpty(aggregate.Trend), aggregate.SourceCount, nullIfEmpty(aggregate.WindowStart), nullIfEmpty(aggregate.WindowEnd),
			now, time.Now().UTC().Add(time.Duration(maxInt(s.Config.SentimentTTLHours, 1))*time.Hour).Format(time.RFC3339),
			nullIfEmpty(lastError), now, snapshotID,
		); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sentiment_sources WHERE snapshot_id=?`, snapshotID); err != nil {
			return nil, err
		}
	}

	for _, item := range items {
		tagsJSON, _ := json.Marshal(item.SignalTags)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sentiment_sources(
				snapshot_id, source_type, provider, title, url, published_at, language, excerpt,
				ticker, company_name, sentiment_score, weight, signal_tags, created_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			snapshotID, item.SourceType, item.Provider, item.Title, item.URL, item.PublishedAt.Format(time.RFC3339),
			nullIfEmpty(item.Language), nullIfEmpty(item.Excerpt), asset.Ticker, nullIfEmpty(asset.CompanyName),
			item.Score, item.Weight, string(tagsJSON), now,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.loadSentimentSnapshot(ctx, asset.AssetID)
}

func (s *Service) persistRefreshFailure(ctx context.Context, assetID int64, previous *sentimentSnapshotRecord, refreshErr error) error {
	if previous == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.DB.ExecContext(ctx, `
			INSERT INTO sentiment_snapshots(asset_id, status, source_count, last_error, created_at, updated_at)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(asset_id) DO UPDATE SET status=excluded.status, source_count=excluded.source_count, last_error=excluded.last_error, updated_at=excluded.updated_at`,
			assetID, "unavailable", 0, refreshErr.Error(), now, now,
		)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE sentiment_snapshots
		SET status=?, last_error=?, updated_at=?
		WHERE asset_id=?`,
		"stale", refreshErr.Error(), time.Now().UTC().Format(time.RFC3339), assetID,
	)
	return err
}

func (s *Service) markSnapshotRefreshing(ctx context.Context, assetID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sentiment_snapshots(asset_id, status, source_count, created_at, updated_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(asset_id) DO UPDATE SET status=excluded.status, updated_at=excluded.updated_at`,
		assetID, "refreshing", 0, now, now,
	)
	return err
}

func (s *Service) insertRefreshLog(ctx context.Context, assetID int64) int64 {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx, `
		INSERT INTO sentiment_refresh_log(asset_id, started_at, status, sources_found)
		VALUES(?,?,?,0)`, assetID, now, "running")
	if err != nil {
		return 0
	}
	id, _ := result.LastInsertId()
	return id
}

func (s *Service) finishRefreshLog(ctx context.Context, logID int64, status string, sourcesFound int, refreshError string) {
	if logID == 0 {
		return
	}
	_, _ = s.DB.ExecContext(ctx, `
		UPDATE sentiment_refresh_log
		SET finished_at=?, status=?, sources_found=?, error=?
		WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), status, sourcesFound, nullIfEmpty(refreshError), logID,
	)
}

func (a newsAdapter) Name() string { return "news" }

func (a newsAdapter) Fetch(ctx context.Context, asset trackedAssetWithID, now time.Time) ([]normalizedSourceItem, error) {
	query := firstNonEmpty(asset.CompanyName, asset.Ticker) + " " + asset.Ticker
	params := neturl.Values{}
	params.Set("q", query)
	params.Set("hl", "pt-BR")
	params.Set("gl", "BR")
	params.Set("ceid", "BR:pt-419")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, newsSearchBaseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", a.service.Config.SentimentUserAgent)

	resp, err := a.service.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed googleNewsRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}

	lookback := now.AddDate(0, 0, -maxInt(a.service.Config.SentimentNewsLookbackDays, 14))
	items := []normalizedSourceItem{}
	for _, item := range feed.Channel.Items {
		publishedAt := parseSentimentTime(item.PubDate)
		if !publishedAt.IsZero() && publishedAt.Before(lookback) {
			continue
		}
		excerpt := sanitizeSnippet(item.Description)
		rawText := strings.TrimSpace(item.Title + " " + excerpt)
		matchConfidence := computeMatchConfidence(asset, rawText)
		if matchConfidence < 0.45 {
			continue
		}
		items = append(items, normalizedSourceItem{
			SourceType:      "news",
			Provider:        "google_news",
			Title:           sanitizeSnippet(item.Title),
			URL:             resolveNewsLink(item.Link),
			PublishedAt:     publishedAt,
			Language:        "pt-BR",
			Excerpt:         excerpt,
			Ticker:          asset.Ticker,
			CompanyName:     asset.CompanyName,
			RawText:         rawText,
			MatchConfidence: matchConfidence,
			Weight:          1.0,
		})
	}
	return items, nil
}

func (a transcriptAdapter) Name() string { return "transcript" }

func (a transcriptAdapter) Fetch(ctx context.Context, asset trackedAssetWithID, now time.Time) ([]normalizedSourceItem, error) {
	query := strings.TrimSpace(firstNonEmpty(asset.CompanyName, asset.Ticker) + " earnings call transcript")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, transcriptSearchBaseURL+"?q="+neturl.QueryEscape(query), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", a.service.Config.SentimentUserAgent)

	resp, err := a.service.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	matches := linkTagPattern.FindAllStringSubmatch(string(body), -1)
	snippets := snippetPattern.FindAllStringSubmatch(string(body), -1)
	items := []normalizedSourceItem{}
	lookback := now.AddDate(0, 0, -maxInt(a.service.Config.SentimentTranscriptLookbackDays, 45))
	_ = lookback
	for index, match := range matches {
		if len(match) < 3 {
			continue
		}
		title := sanitizeSnippet(match[2])
		excerpt := ""
		if index < len(snippets) {
			excerpt = sanitizeSnippet(firstNonEmpty(snippets[index][1], snippets[index][2]))
		}
		rawText := strings.TrimSpace(title + " " + excerpt)
		matchConfidence := computeMatchConfidence(asset, rawText)
		if matchConfidence < 0.4 {
			continue
		}
		items = append(items, normalizedSourceItem{
			SourceType:      "transcript",
			Provider:        "duckduckgo_public_search",
			Title:           title,
			URL:             decodeDuckDuckGoLink(match[1]),
			PublishedAt:     now,
			Language:        "en",
			Excerpt:         excerpt,
			Ticker:          asset.Ticker,
			CompanyName:     asset.CompanyName,
			RawText:         rawText,
			MatchConfidence: matchConfidence,
			Weight:          1.4,
		})
		if len(items) >= 4 {
			break
		}
	}
	return items, nil
}

func scoreSentiment(items []normalizedSourceItem, previous *sentimentSnapshotRecord, now time.Time, ttlHours int) scoredSentiment {
	if len(items) == 0 {
		return scoredSentiment{
			Status:      "unavailable",
			Trend:       previousTrend(previous),
			SourceCount: 0,
			Message:     "No recent public news or transcript sources were found.",
		}
	}

	windowStart := now
	windowEnd := time.Time{}
	weightedSum := 0.0
	weightTotal := 0.0
	sourceKinds := map[string]struct{}{}
	confidenceAccumulator := 0.0
	for index := range items {
		if items[index].PublishedAt.IsZero() {
			items[index].PublishedAt = now
		}
		if items[index].PublishedAt.Before(windowStart) {
			windowStart = items[index].PublishedAt
		}
		if items[index].PublishedAt.After(windowEnd) {
			windowEnd = items[index].PublishedAt
		}
		score, tags := scoreItemText(items[index].RawText)
		items[index].Score = clamp(score, -100, 100)
		items[index].SignalTags = tags
		recencyWeight := recencyDecay(now, items[index].PublishedAt)
		itemWeight := items[index].Weight * recencyWeight * math.Max(items[index].MatchConfidence, 0.35)
		weightedSum += items[index].Score * itemWeight
		weightTotal += itemWeight
		confidenceAccumulator += items[index].MatchConfidence
		sourceKinds[items[index].SourceType] = struct{}{}
	}

	scoreValue := 0.0
	if weightTotal > 0 {
		scoreValue = weightedSum / weightTotal
	}
	scoreValue = clamp(scoreValue, -100, 100)
	confidenceValue := clamp((confidenceAccumulator/float64(len(items)))*0.65+float64(len(sourceKinds))*0.15+math.Min(float64(len(items))/6, 0.2), 0, 1)
	label := deriveSentimentLabel(scoreValue, len(items))
	trend := deriveTrend(previous, scoreValue)
	status := "ok"
	message := ""
	if len(items) > 0 && windowEnd.Before(now.Add(-time.Duration(maxInt(ttlHours, 1))*time.Hour)) {
		status = "stale"
		message = "Sentiment sources are older than the refresh window."
	}

	return scoredSentiment{
		Status:      status,
		Label:       label,
		Score:       floatPointer(scoreValue),
		Confidence:  floatPointer(confidenceValue),
		Trend:       trend,
		SourceCount: len(items),
		WindowStart: windowStart.Format(time.RFC3339),
		WindowEnd:   windowEnd.Format(time.RFC3339),
		Message:     message,
	}
}

func scoreItemText(rawText string) (float64, []string) {
	text := normalizeCompanyName(rawText)
	positive := map[string]float64{
		"BEAT": 18, "GROWTH": 16, "GUIDANCE RAISED": 20, "STRONG DEMAND": 16, "MARGIN EXPANSION": 18,
		"DELEVERAGING": 15, "RECORDE": 18, "CRESCIMENTO": 16, "FORTE DEMANDA": 16, "EXPANSAO DE MARGEM": 18,
		"REDUCAO DE DIVIDA": 16, "LUCRO": 12, "UPSIDE": 10,
	}
	negative := map[string]float64{
		"MISS": -18, "WEAKER": -12, "GUIDANCE CUT": -20, "PRESSURE": -10, "IMPAIRMENT": -18,
		"LEVERAGE INCREASE": -16, "INVESTIGATION": -18, "QUEDA": -14, "PRESSAO": -10, "PREJUIZO": -14,
		"CORTE DE GUIDANCE": -20, "ALAVANCAGEM": -12, "FRAQUEZA": -10,
	}

	score := 0.0
	tags := []string{}
	for key, weight := range positive {
		if strings.Contains(text, key) {
			score += weight
			tags = append(tags, key)
		}
	}
	for key, weight := range negative {
		if strings.Contains(text, key) {
			score += weight
			tags = append(tags, key)
		}
	}
	if score == 0 {
		if strings.Contains(text, "RESULT") || strings.Contains(text, "RESULTADO") {
			tags = append(tags, "results_update")
		}
	}
	return score, tags
}

func deriveSentimentLabel(score float64, sourceCount int) string {
	if sourceCount < 2 && math.Abs(score) < 20 {
		return "mixed"
	}
	switch {
	case score >= 25:
		return "positive"
	case score <= -25:
		return "negative"
	case math.Abs(score) <= 24:
		return "neutral"
	default:
		return "mixed"
	}
}

func deriveTrend(previous *sentimentSnapshotRecord, current float64) string {
	if previous == nil || previous.Score == nil {
		return "insufficient_data"
	}
	delta := current - *previous.Score
	switch {
	case delta >= 15:
		return "improving"
	case delta <= -15:
		return "worsening"
	default:
		return "flat"
	}
}

func previousTrend(previous *sentimentSnapshotRecord) string {
	if previous == nil || previous.Trend == "" {
		return "insufficient_data"
	}
	return previous.Trend
}

func recencyDecay(now, publishedAt time.Time) float64 {
	if publishedAt.IsZero() {
		return 0.8
	}
	days := now.Sub(publishedAt).Hours() / 24
	switch {
	case days <= 1:
		return 1.2
	case days <= 7:
		return 1.0
	case days <= 14:
		return 0.85
	default:
		return 0.65
	}
}

func computeMatchConfidence(asset trackedAssetWithID, rawText string) float64 {
	text := normalizeCompanyName(rawText)
	companyKey := normalizeCompanyName(asset.CompanyName)
	score := 0.0
	if asset.Ticker != "" && strings.Contains(text, normalizeCompanyName(asset.Ticker)) {
		score += 0.45
	}
	if companyKey != "" {
		if strings.Contains(text, companyKey) {
			score += 0.55
		}
		score += 0.45 * jaccard(tokenSet(companyKey), tokenSet(text))
		score += 0.25 * companyTokenOverlap(companyKey, text)
	}
	if asset.AssetType == "bdr" && asset.CompanyName != "" {
		score += 0.1
	}
	return clamp(score, 0, 1)
}

func parseSentimentTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 02 Jan 2006 15:04:05 MST"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func sanitizeSnippet(value string) string {
	value = tagPattern.ReplaceAllString(value, " ")
	value = strings.NewReplacer("&amp;", "&", "&#39;", "'", "&quot;", "\"", "&nbsp;", " ").Replace(value)
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}

func decodeDuckDuckGoLink(value string) string {
	decoded, err := neturl.QueryUnescape(value)
	if err == nil {
		value = decoded
	}
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	if strings.HasPrefix(value, "/l/?uddg=") {
		if parsed, err := neturl.Parse("https://duckduckgo.com" + value); err == nil {
			target := parsed.Query().Get("uddg")
			if target != "" {
				if unescaped, err := neturl.QueryUnescape(target); err == nil {
					return unescaped
				}
				return target
			}
		}
	}
	return value
}

func resolveNewsLink(value string) string {
	if strings.HasPrefix(value, "./") {
		return "https://news.google.com/" + strings.TrimPrefix(value, "./")
	}
	return value
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func floatPointer(value float64) *float64 {
	v := value
	return &v
}

func maxInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func companyTokenOverlap(companyKey, text string) float64 {
	companyTokens := strings.Fields(companyKey)
	textTokens := tokenSet(text)
	if len(companyTokens) == 0 {
		return 0
	}
	matches := 0
	relevant := 0
	for _, token := range companyTokens {
		if len(token) <= 3 {
			continue
		}
		relevant++
		if _, ok := textTokens[token]; ok {
			matches++
		}
	}
	if relevant == 0 {
		return 0
	}
	return float64(matches) / float64(relevant)
}

func (record *sentimentSnapshotRecord) isExpired() bool {
	if record == nil || record.ExpiresAt == "" {
		return true
	}
	return parseSentimentTime(record.ExpiresAt).Before(time.Now().UTC())
}

func (record *sentimentSnapshotRecord) isRecentlyRefreshing() bool {
	if record == nil || record.Status != "refreshing" || record.UpdatedAt == "" {
		return false
	}
	return parseSentimentTime(record.UpdatedAt).After(time.Now().UTC().Add(-2 * time.Minute))
}

func (record *sentimentSnapshotRecord) toAPIModel() *models.TickerSentiment {
	if record == nil {
		return nil
	}
	isStale := record.Status == "stale"
	message := record.Message
	if message == "" {
		switch record.Status {
		case "unavailable":
			message = "No recent public news or transcript sources were found."
		case "refreshing":
			message = "Refreshing public sentiment sources."
		case "stale":
			message = firstNonEmpty(record.LastError, "Showing stale sentiment while refresh is retried.")
		}
	}
	return &models.TickerSentiment{
		Status:          record.Status,
		Label:           record.Label,
		Score:           record.Score,
		Confidence:      record.Confidence,
		Trend:           record.Trend,
		SourceCount:     record.SourceCount,
		LastRefreshedAt: record.LastRefreshedAt,
		IsStale:         isStale,
		Message:         message,
		Sources:         record.Sources,
	}
}
