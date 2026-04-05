package services

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"math"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/encoding/charmap"
	"investments-portfolio-manager/backend/internal/config"
	"investments-portfolio-manager/backend/internal/models"
)

type Service struct {
	DB     *sql.DB
	Config config.Config
	Client *http.Client
}

func New(db *sql.DB, cfg config.Config) *Service {
	return &Service{
		DB:     db,
		Config: cfg,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func ParseMonteCarloParams(yearsRaw, simulationsRaw, expectedReturnRaw, volatilityRaw string) models.MonteCarloParams {
	params := models.MonteCarloParams{}
	if years, err := strconv.Atoi(strings.TrimSpace(yearsRaw)); err == nil {
		params.Years = years
	}
	if simulations, err := strconv.Atoi(strings.TrimSpace(simulationsRaw)); err == nil {
		params.Simulations = simulations
	}
	if expectedReturn, err := strconv.ParseFloat(strings.TrimSpace(expectedReturnRaw), 64); err == nil {
		params.ExpectedReturn = normalizeRate(expectedReturn)
	}
	if volatility, err := strconv.ParseFloat(strings.TrimSpace(volatilityRaw), 64); err == nil {
		params.Volatility = math.Abs(normalizeRate(volatility))
	}
	return params
}

func normalizeRate(value float64) float64 {
	if math.Abs(value) > 1 {
		return value / 100
	}
	return value
}

func (s *Service) ImportB3(ctx context.Context) (models.ImportJobResponse, error) {
	jobID, job, err := s.createJob(ctx, "b3", "running", "Import started")
	if err != nil {
		return models.ImportJobResponse{}, err
	}
	// Run the worker in the background so the HTTP request returns immediately.
	// The caller polls GET /portfolio/import-jobs/latest for the final status.
	go func() {
		bgCtx := context.Background()
		holdings, err := s.runWorker(bgCtx, []string{"import", "--json"})
		if err != nil {
			s.updateJob(bgCtx, jobID, "requires_login", err.Error())
			return
		}
		if err := s.upsertHoldings(bgCtx, holdings); err != nil {
			s.updateJob(bgCtx, jobID, "failed", err.Error())
			return
		}
		s.updateJob(bgCtx, jobID, "completed", fmt.Sprintf("Imported %d positions from B3", len(holdings)))
	}()
	return job, nil
}

func (s *Service) ImportIBKR(ctx context.Context) (models.ImportJobResponse, error) {
	if s.Config.IBKRFlexToken == "" || s.Config.IBKRFlexQueryID == "" {
		return models.ImportJobResponse{}, fmt.Errorf("IBKR_FLEX_TOKEN and IBKR_FLEX_QUERY_ID must be configured")
	}
	jobID, job, err := s.createJob(ctx, "ibkr", "running", "IBKR import started")
	if err != nil {
		return models.ImportJobResponse{}, err
	}
	go func() {
		bgCtx := context.Background()
		holdings, err := s.runWorkerWithEnv(bgCtx, []string{"import-ibkr", "--json"}, map[string]string{
			"IBKR_FLEX_TOKEN":    s.Config.IBKRFlexToken,
			"IBKR_FLEX_QUERY_ID": s.Config.IBKRFlexQueryID,
		})
		if err != nil {
			s.updateJob(bgCtx, jobID, "failed", err.Error())
			return
		}
		if err := s.upsertHoldings(bgCtx, holdings); err != nil {
			s.updateJob(bgCtx, jobID, "failed", err.Error())
			return
		}
		s.updateJob(bgCtx, jobID, "completed", fmt.Sprintf("Imported %d positions from IBKR", len(holdings)))
	}()
	return job, nil
}

func (s *Service) ImportFile(ctx context.Context, file multipart.File, filename string) (models.ImportJobResponse, error) {
	if err := os.MkdirAll(s.Config.UploadDir, 0o755); err != nil {
		return models.ImportJobResponse{}, err
	}
	tmp, err := os.CreateTemp(s.Config.UploadDir, "upload-*"+filepath.Ext(filename))
	if err != nil {
		return models.ImportJobResponse{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, file); err != nil {
		return models.ImportJobResponse{}, err
	}

	jobID, _, err := s.createJob(ctx, "manual_b3_export", "running", "Importing "+filename)
	if err != nil {
		return models.ImportJobResponse{}, err
	}
	holdings, err := s.runWorker(ctx, []string{"import-file", tmp.Name(), "--json"})
	if err != nil {
		updated, _ := s.updateJob(ctx, jobID, "failed", err.Error())
		return updated, nil
	}
	if err := s.upsertHoldings(ctx, holdings); err != nil {
		updated, _ := s.updateJob(ctx, jobID, "failed", err.Error())
		return updated, nil
	}
	return s.updateJob(ctx, jobID, "completed", fmt.Sprintf("Imported %d positions from %s", len(holdings), filename))
}

func (s *Service) GetPositions(ctx context.Context) ([]models.PositionResponse, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT a.ticker, a.asset_type, p.quantity, p.avg_price, COALESCE(p.broker,''), p.source, p.last_updated, p.hidden
		FROM positions p
		JOIN assets a ON a.id = p.asset_id
		ORDER BY datetime(p.last_updated) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.PositionResponse, 0)
	for rows.Next() {
		var item models.PositionResponse
		if err := rows.Scan(&item.Ticker, &item.AssetType, &item.Quantity, &item.AvgPrice, &item.Broker, &item.Source, &item.LastUpdated, &item.Hidden); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SetPositionsVisibility(ctx context.Context, visible bool) error {
	hidden := 0
	if !visible {
		hidden = 1
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE positions SET hidden = ?`, hidden)
	return err
}

func (s *Service) GetPortfolio(ctx context.Context) (models.PortfolioResponse, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT a.ticker, a.asset_type, p.quantity, p.avg_price
		FROM positions p
		JOIN assets a ON a.id = p.asset_id`)
	if err != nil {
		return models.PortfolioResponse{}, err
	}
	defer rows.Close()

	type alloc struct {
		assetType string
		value     float64
	}
	allocByTicker := map[string]alloc{}
	total := 0.0
	count := 0
	for rows.Next() {
		count++
		var ticker, assetType string
		var qty, price float64
		if err := rows.Scan(&ticker, &assetType, &qty, &price); err != nil {
			return models.PortfolioResponse{}, err
		}
		value := qty * price
		total += value
		allocByTicker[ticker] = alloc{assetType: assetType, value: value}
	}
	allocations := make([]models.AllocationItem, 0, len(allocByTicker))
	for ticker, item := range allocByTicker {
		weight := 0.0
		if total > 0 {
			weight = item.value / total
		}
		allocations = append(allocations, models.AllocationItem{
			Ticker: ticker, AssetType: item.assetType, MarketValue: item.value, Weight: weight,
		})
	}
	sort.Slice(allocations, func(i, j int) bool { return allocations[i].MarketValue > allocations[j].MarketValue })
	return models.PortfolioResponse{
		TotalPositions: count, EstimatedCostBasis: total, Allocations: allocations,
	}, nil
}

func (s *Service) GetMonteCarloSimulation(ctx context.Context, params models.MonteCarloParams) (models.MonteCarloResponse, error) {
	portfolio, err := s.GetPortfolio(ctx)
	if err != nil {
		return models.MonteCarloResponse{}, err
	}
	initial := portfolio.EstimatedCostBasis
	if params.Years <= 0 {
		params.Years = 10
	}
	if params.Simulations <= 0 {
		params.Simulations = 1000
	}
	if params.ExpectedReturn == 0 {
		params.ExpectedReturn = 0.10
	}
	if params.Volatility == 0 {
		params.Volatility = 0.18
	}
	if params.Years > 40 {
		params.Years = 40
	}
	if params.Simulations > 20000 {
		params.Simulations = 20000
	}
	if params.Volatility < 0 {
		params.Volatility = math.Abs(params.Volatility)
	}
	if params.Volatility > 3 {
		params.Volatility = 3
	}
	if params.ExpectedReturn < -0.99 {
		params.ExpectedReturn = -0.99
	}
	if params.ExpectedReturn > 3 {
		params.ExpectedReturn = 3
	}
	if initial <= 0 {
		return models.MonteCarloResponse{
			InitialValue: 0,
			Params:       params,
			Timeline:     []models.MonteCarloYearPoint{},
			Message:      "Import positions first so the simulator has an initial portfolio value.",
		}, nil
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	paths := make([][]float64, params.Years)
	for year := 0; year < params.Years; year++ {
		paths[year] = make([]float64, params.Simulations)
	}

	drift := params.ExpectedReturn - 0.5*params.Volatility*params.Volatility
	for i := 0; i < params.Simulations; i++ {
		value := initial
		for year := 0; year < params.Years; year++ {
			z := rng.NormFloat64()
			shock := math.Exp(drift + params.Volatility*z)
			value *= shock
			paths[year][i] = value
		}
	}

	timeline := make([]models.MonteCarloYearPoint, 0, params.Years)
	for year := 0; year < params.Years; year++ {
		values := append([]float64(nil), paths[year]...)
		sort.Float64s(values)
		sum := 0.0
		positive := 0
		for _, v := range values {
			sum += v
			if v >= initial {
				positive++
			}
		}
		idx10 := int(math.Floor(float64(len(values)-1) * 0.10))
		idx50 := int(math.Floor(float64(len(values)-1) * 0.50))
		idx90 := int(math.Floor(float64(len(values)-1) * 0.90))
		timeline = append(timeline, models.MonteCarloYearPoint{
			Year:         year + 1,
			P10:          values[idx10],
			P50:          values[idx50],
			P90:          values[idx90],
			Average:      sum / float64(len(values)),
			BestCase:     values[len(values)-1],
			WorstCase:    values[0],
			ProbPositive: float64(positive) / float64(len(values)),
		})
	}

	return models.MonteCarloResponse{
		InitialValue: initial,
		Params:       params,
		Timeline:     timeline,
		Message:      "Annualized geometric Brownian motion simulation using current cost basis as the starting value.",
	}, nil
}

func (s *Service) GetLatestQuarterlyResults(ctx context.Context) (models.QuarterlyResultsResponse, error) {
	tracked, err := s.loadTrackedAssets(ctx)
	if err != nil {
		return models.QuarterlyResultsResponse{}, err
	}
	if len(tracked) == 0 {
		return models.QuarterlyResultsResponse{
			Provider: "cvm_itr", Configured: true, Message: "No stock positions were found in the imported portfolio.", Items: []models.QuarterlyResultItem{},
		}, nil
	}
	needsMetadata := true
	for _, item := range tracked {
		if item.CompanyName != "" || item.TaxID != "" {
			needsMetadata = false
			break
		}
	}
	if needsMetadata {
		items := make([]models.QuarterlyResultItem, 0, len(tracked))
		for _, asset := range tracked {
			items = append(items, models.QuarterlyResultItem{
				Ticker: asset.Ticker, AssetType: asset.AssetType, Highlights: []string{}, Status: "metadata_missing", Message: "Issuer metadata is missing for this position.",
			})
		}
		return models.QuarterlyResultsResponse{
			Provider: "cvm_itr", Configured: true, Message: "Re-upload the B3 workbook once so issuer metadata is stored before CVM matching runs.", Items: items,
		}, nil
	}
	rows, year, err := s.loadLatestITRRows(ctx)
	if err != nil || len(rows) == 0 {
		items := make([]models.QuarterlyResultItem, 0, len(tracked))
		for _, asset := range tracked {
			items = append(items, models.QuarterlyResultItem{
				Ticker: asset.Ticker, CompanyName: asset.CompanyName, AssetType: asset.AssetType, Highlights: []string{}, Status: "unavailable", Message: "CVM ITR dataset unavailable.",
			})
		}
		return models.QuarterlyResultsResponse{Provider: "cvm_itr", Configured: true, Message: "CVM quarterly files could not be loaded right now.", Items: items}, nil
	}
	taxIndex := indexByTaxID(rows)
	nameIndex := indexByName(rows)
	tickers := make([]string, 0, len(tracked))
	for _, asset := range tracked {
		tickers = append(tickers, asset.Ticker)
	}
	dyMap := s.fetchFundamentusDividendYields(ctx, tickers)
	items := make([]models.QuarterlyResultItem, 0, len(tracked))
	for _, asset := range tracked {
		items = append(items, s.buildQuarterlyResult(ctx, asset, taxIndex, nameIndex, dyMap))
	}
	return models.QuarterlyResultsResponse{
		Provider:   "cvm_itr",
		Configured: true,
		Message:    fmt.Sprintf("Source: CVM ITR %d. Latest reported quarter is inferred from filing periods.", year),
		Items:      items,
	}, nil
}

func (s *Service) GetTickerSentiment(ctx context.Context, ticker string) (*models.TickerSentiment, error) {
	var asset models.TrackedAsset
	err := s.DB.QueryRowContext(ctx, `
		SELECT a.id, a.ticker, a.asset_type, COALESCE(m.company_name,''), COALESCE(m.tax_id,'')
		FROM assets a
		LEFT JOIN asset_metadata m ON m.asset_id = a.id
		WHERE a.ticker=?`, strings.ToUpper(ticker)).
		Scan(&asset.AssetID, &asset.Ticker, &asset.AssetType, &asset.CompanyName, &asset.TaxID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sentiment := s.GetOrRefreshSentiment(ctx, asset)
	return sentiment, nil
}

func (s *Service) GetLatestImportJob(ctx context.Context) (models.ImportJobResponse, error) {
	var resp models.ImportJobResponse
	err := s.DB.QueryRowContext(ctx, `SELECT id, source, status, COALESCE(detail,''), created_at, updated_at FROM import_jobs ORDER BY id DESC LIMIT 1`).
		Scan(&resp.ID, &resp.Source, &resp.Status, &resp.Detail, &resp.CreatedAt, &resp.UpdatedAt)
	if err != nil {
		return models.ImportJobResponse{}, err
	}
	return resp, nil
}

func (s *Service) createJob(ctx context.Context, source, status, detail string) (int64, models.ImportJobResponse, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx, `INSERT INTO import_jobs(source,status,detail,created_at,updated_at) VALUES(?,?,?,?,?)`, source, status, detail, now, now)
	if err != nil {
		return 0, models.ImportJobResponse{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, models.ImportJobResponse{}, err
	}
	return id, models.ImportJobResponse{ID: id, Source: source, Status: status, Detail: detail, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) updateJob(ctx context.Context, id int64, status, detail string) (models.ImportJobResponse, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.ExecContext(ctx, `UPDATE import_jobs SET status=?, detail=?, updated_at=? WHERE id=?`, status, detail, now, id); err != nil {
		return models.ImportJobResponse{}, err
	}
	var resp models.ImportJobResponse
	err := s.DB.QueryRowContext(ctx, `SELECT id, source, status, COALESCE(detail,''), created_at, updated_at FROM import_jobs WHERE id=?`, id).
		Scan(&resp.ID, &resp.Source, &resp.Status, &resp.Detail, &resp.CreatedAt, &resp.UpdatedAt)
	return resp, err
}

func (s *Service) runWorker(ctx context.Context, args []string) ([]models.HoldingPayload, error) {
	// Use a dedicated timeout so the browser-based worker isn't killed if the
	// HTTP request context is cancelled (e.g. client disconnect or proxy timeout).
	workerCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Still respect explicit cancellation from the caller (e.g. server shutdown).
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-workerCtx.Done():
		}
	}()
	var cmd *exec.Cmd
	if strings.TrimSpace(s.Config.WorkerCommand) != "" {
		parts := strings.Fields(s.Config.WorkerCommand)
		cmd = exec.CommandContext(workerCtx, parts[0], parts[1:]...)
	} else {
		all := append([]string{"-m", s.Config.WorkerModule}, args...)
		cmd = exec.CommandContext(workerCtx, s.Config.WorkerPython, all...)
	}
	cmd.Dir = s.Config.WorkerDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	var payload models.WorkerImportResponse
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, err
	}
	for i := range payload.Holdings {
		payload.Holdings[i].Ticker = strings.ToUpper(payload.Holdings[i].Ticker)
		if payload.Holdings[i].Currency == "" {
			payload.Holdings[i].Currency = "BRL"
		}
	}
	return payload.Holdings, nil
}

func (s *Service) runWorkerWithEnv(ctx context.Context, args []string, extraEnv map[string]string) ([]models.HoldingPayload, error) {
	workerCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-workerCtx.Done():
		}
	}()
	all := append([]string{"-m", s.Config.WorkerModule}, args...)
	cmd := exec.CommandContext(workerCtx, s.Config.WorkerPython, all...)
	cmd.Dir = s.Config.WorkerDir
	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	var payload models.WorkerImportResponse
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, err
	}
	for i := range payload.Holdings {
		payload.Holdings[i].Ticker = strings.ToUpper(payload.Holdings[i].Ticker)
		if payload.Holdings[i].Currency == "" {
			payload.Holdings[i].Currency = "USD"
		}
	}
	return payload.Holdings, nil
}

func (s *Service) upsertHoldings(ctx context.Context, holdings []models.HoldingPayload) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, holding := range holdings {
		var assetID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM assets WHERE ticker=?`, holding.Ticker).Scan(&assetID)
		if errors.Is(err, sql.ErrNoRows) {
			res, err := tx.ExecContext(ctx, `INSERT INTO assets(ticker,asset_type,currency) VALUES(?,?,?)`, holding.Ticker, holding.AssetType, defaultString(holding.Currency, "BRL"))
			if err != nil {
				return err
			}
			assetID, _ = res.LastInsertId()
		} else if err != nil {
			return err
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE assets SET asset_type=?, currency=? WHERE id=?`, holding.AssetType, defaultString(holding.Currency, "BRL"), assetID); err != nil {
				return err
			}
		}

		var metadataID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM asset_metadata WHERE asset_id=?`, assetID).Scan(&metadataID)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO asset_metadata(asset_id, company_name, tax_id, last_updated) VALUES(?,?,?,?)`, assetID, nullIfEmpty(holding.CompanyName), nullIfEmpty(holding.TaxID), now); err != nil {
				return err
			}
		} else if err == nil {
			if _, err := tx.ExecContext(ctx, `UPDATE asset_metadata SET company_name=COALESCE(?, company_name), tax_id=COALESCE(?, tax_id), last_updated=? WHERE asset_id=?`, nullIfEmpty(holding.CompanyName), nullIfEmpty(holding.TaxID), now, assetID); err != nil {
				return err
			}
		} else {
			return err
		}

		var posID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM positions WHERE user_id=? AND asset_id=?`, s.Config.DefaultUserID, assetID).Scan(&posID)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO positions(user_id,asset_id,quantity,avg_price,broker,source,last_updated) VALUES(?,?,?,?,?,?,?)`, s.Config.DefaultUserID, assetID, holding.Quantity, holding.AveragePrice, nullIfEmpty(holding.Broker), "b3", now); err != nil {
				return err
			}
		} else if err == nil {
			if _, err := tx.ExecContext(ctx, `UPDATE positions SET quantity=?, avg_price=?, broker=?, source='b3', last_updated=? WHERE id=?`, holding.Quantity, holding.AveragePrice, nullIfEmpty(holding.Broker), now, posID); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return tx.Commit()
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

type cvmRow map[string]string

func (s *Service) loadTrackedAssets(ctx context.Context) ([]models.TrackedAsset, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT a.id, a.ticker, a.asset_type, COALESCE(m.company_name,''), COALESCE(m.tax_id,'')
		FROM assets a
		JOIN positions p ON p.asset_id = a.id
		LEFT JOIN asset_metadata m ON m.asset_id = a.id
		WHERE p.source='b3' AND a.asset_type IN ('stock','bdr')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.TrackedAsset
	for rows.Next() {
		var item models.TrackedAsset
		if err := rows.Scan(&item.AssetID, &item.Ticker, &item.AssetType, &item.CompanyName, &item.TaxID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) loadLatestITRRows(ctx context.Context) ([]cvmRow, int, error) {
	year := time.Now().Year()
	for _, candidate := range []int{year, year - 1, year - 2} {
		path, err := s.ensureITRZip(ctx, candidate)
		if err != nil {
			continue
		}
		rows, err := readDRERows(path)
		if err == nil && len(rows) > 0 {
			return rows, candidate, nil
		}
	}
	return nil, 0, errors.New("no ITR rows available")
}

func (s *Service) ensureITRZip(ctx context.Context, year int) (string, error) {
	if err := os.MkdirAll(s.Config.DataCacheDir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(s.Config.DataCacheDir, fmt.Sprintf("itr_cia_aberta_%d.zip", year))
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/itr_cia_aberta_%d.zip", s.Config.CVMITRBaseURL, year), nil)
	if err != nil {
		return "", err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("cvm returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return target, os.WriteFile(target, body, 0o644)
}

func readDRERows(zipPath string) ([]cvmRow, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var files []*zip.File
	for _, file := range reader.File {
		if strings.Contains(file.Name, "DRE_con") {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		for _, file := range reader.File {
			if strings.Contains(file.Name, "DRE_ind") {
				files = append(files, file)
			}
		}
	}
	var rows []cvmRow
	for _, file := range files {
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		reader := csv.NewReader(strings.NewReader(decodeLatin1(content)))
		reader.Comma = ';'
		reader.LazyQuotes = true
		headers, err := reader.Read()
		if err != nil {
			return nil, err
		}
		for {
			record, err := reader.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			row := cvmRow{}
			for i, header := range headers {
				if i < len(record) {
					row[header] = strings.TrimSpace(record[i])
				}
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func indexByTaxID(rows []cvmRow) map[string][]cvmRow {
	out := map[string][]cvmRow{}
	for _, row := range rows {
		taxID := normalizeTaxID(row["CNPJ_CIA"])
		if taxID == "" {
			continue
		}
		out[taxID] = append(out[taxID], row)
	}
	return out
}

func indexByName(rows []cvmRow) map[string][]cvmRow {
	out := map[string][]cvmRow{}
	for _, row := range rows {
		key := normalizeCompanyName(row["DENOM_CIA"])
		if key == "" {
			continue
		}
		out[key] = append(out[key], row)
	}
	return out
}

// fetchFundamentusDividendYields fetches the trailing dividend yield for each
// ticker from Fundamentus concurrently. It returns an empty map (never nil)
// so callers can degrade gracefully when data is unavailable.
func (s *Service) fetchFundamentusDividendYields(ctx context.Context, tickers []string) map[string]*float64 {
	out := make(map[string]*float64, len(tickers))
	if len(tickers) == 0 {
		return out
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, ticker := range tickers {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			dy := s.scrapeFundamentusDY(ctx, t)
			if dy != nil {
				mu.Lock()
				out[strings.ToUpper(t)] = dy
				mu.Unlock()
			}
		}(ticker)
	}
	wg.Wait()
	return out
}

// scrapeFundamentusDY fetches a single ticker page from Fundamentus and
// extracts the "Div. Yield" value. Returns nil on any error.
func (s *Service) scrapeFundamentusDY(ctx context.Context, ticker string) *float64 {
	url := "https://www.fundamentus.com.br/detalhes.php?papel=" + strings.ToUpper(ticker)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("fundamentus: build request %s: %v", ticker, err)
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	resp, err := s.Client.Do(req)
	if err != nil {
		log.Printf("fundamentus: request %s: %v", ticker, err)
		return nil
	}
	defer resp.Body.Close()
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := decoder.Reader(resp.Body)
	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("fundamentus: read %s: %v", ticker, err)
		return nil
	}
	html := string(body)
	anchor := `<span class="txt">Div. Yield</span>`
	idx := strings.Index(html, anchor)
	if idx < 0 {
		log.Printf("fundamentus: Div. Yield not found for %s", ticker)
		return nil
	}
	sub := html[idx+len(anchor):]
	spanIdx := strings.Index(sub, `<span class="txt">`)
	if spanIdx < 0 {
		return nil
	}
	sub = sub[spanIdx+len(`<span class="txt">`):]
	endIdx := strings.Index(sub, "</span>")
	if endIdx < 0 {
		return nil
	}
	raw := strings.TrimSpace(sub[:endIdx]) // e.g. "7,3%"
	raw = strings.ReplaceAll(raw, "%", "")
	raw = strings.ReplaceAll(raw, ",", ".")
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		log.Printf("fundamentus: parse DY %q for %s: %v", raw, ticker, err)
		return nil
	}
	return &v
}

type fiiScrapedData struct {
	DividendYield   *float64
	PVP             *float64
	FFOYield        *float64
	DividendPerUnit *float64
	CapRate         *float64
	VacancyRate     *float64
	AvgDailyVolume  *float64
}

func (s *Service) scrapeFundamentusFII(ctx context.Context, ticker string) *fiiScrapedData {
	url := "https://www.fundamentus.com.br/detalhes.php?papel=" + strings.ToUpper(ticker)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("fundamentus fii: build request %s: %v", ticker, err)
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	resp, err := s.Client.Do(req)
	if err != nil {
		log.Printf("fundamentus fii: request %s: %v", ticker, err)
		return nil
	}
	defer resp.Body.Close()
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := decoder.Reader(resp.Body)
	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("fundamentus fii: read %s: %v", ticker, err)
		return nil
	}
	html := string(body)
	out := &fiiScrapedData{}
	out.DividendYield = scrapeFundamentusField(html, "Div. Yield", true)
	out.PVP = scrapeFundamentusField(html, "P/VP", false)
	out.FFOYield = scrapeFundamentusField(html, "FFO Yield", true)
	out.DividendPerUnit = scrapeFundamentusField(html, "Dividendo/cota", false)
	out.CapRate = scrapeFundamentusField(html, "Cap Rate", true)
	out.VacancyRate = scrapeFundamentusField(html, "Vacância Média", true)
	out.AvgDailyVolume = scrapeFundamentusVolume(html, "Vol $ méd (2m)")
	if out.DividendYield == nil && out.PVP == nil && out.FFOYield == nil && out.DividendPerUnit == nil {
		return nil
	}
	return out
}

func scrapeFundamentusField(html, label string, isPercent bool) *float64 {
	// Anchor on the label inside its txt span to avoid matching tooltip title attributes
	// which may also contain the label text (e.g. "FFO Yield" appears in its own tooltip).
	// Use LastIndex because some fields (e.g. "Div. Yield", "P/VP") appear in both the
	// general stock section and the FII-specific section of detalhes.php. The last
	// occurrence is always the FII section, which carries the correct trailing 12M values.
	anchor := `<span class="txt">` + label + `</span>`
	idx := strings.LastIndex(html, anchor)
	if idx < 0 {
		return nil
	}
	sub := html[idx+len(anchor):]
	spanIdx := strings.Index(sub, `<span class="txt">`)
	if spanIdx < 0 {
		return nil
	}
	sub = sub[spanIdx+len(`<span class="txt">`):]
	endIdx := strings.Index(sub, "</span>")
	if endIdx < 0 {
		return nil
	}
	raw := strings.TrimSpace(sub[:endIdx])
	if isPercent {
		raw = strings.ReplaceAll(raw, "%", "")
	}
	raw = strings.ReplaceAll(raw, ",", ".")
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return nil
	}
	return &v
}

// scrapeFundamentusVolume scrapes a volume/currency field where dots are
// thousands separators (e.g. "6.100.890") rather than decimal points.
// Uses LastIndex for the same reason as scrapeFundamentusField: some labels
// appear in both the stock and FII sections; the last occurrence is the FII one.
func scrapeFundamentusVolume(html, label string) *float64 {
	anchor := `<span class="txt">` + label + `</span>`
	idx := strings.LastIndex(html, anchor)
	if idx < 0 {
		return nil
	}
	sub := html[idx+len(anchor):]
	spanIdx := strings.Index(sub, `<span class="txt">`)
	if spanIdx < 0 {
		return nil
	}
	sub = sub[spanIdx+len(`<span class="txt">`):]
	endIdx := strings.Index(sub, "</span>")
	if endIdx < 0 {
		return nil
	}
	raw := strings.TrimSpace(sub[:endIdx])
	// Strip thousands-separator dots, replace decimal comma with dot
	raw = strings.ReplaceAll(raw, ".", "")
	raw = strings.ReplaceAll(raw, ",", ".")
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return nil
	}
	return &v
}

// scrapeStatusInvestFII fetches primary FII metrics from statusinvest.com.br.
// Fundamentus remains the fallback when Status Invest is unavailable.
func (s *Service) scrapeStatusInvestFII(ctx context.Context, ticker string) *fiiScrapedData {
	url := "https://statusinvest.com.br/fundos-imobiliarios/" + strings.ToLower(ticker)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("statusinvest fii: build request %s: %v", ticker, err)
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")
	resp, err := s.Client.Do(req)
	if err != nil {
		log.Printf("statusinvest fii: request %s: %v", ticker, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("statusinvest fii: unexpected status %s for %s", resp.Status, ticker)
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("statusinvest fii: read %s: %v", ticker, err)
		return nil
	}
	html := string(body)
	text := normalizeStatusInvestText(html)
	out := &fiiScrapedData{}
	out.DividendYield = scrapeStatusInvestDividendYield(text)
	out.PVP = scrapeStatusInvestPVP(text)
	out.DividendPerUnit = scrapeStatusInvestLastDividend(text)
	out.AvgDailyVolume = scrapeStatusInvestLiquidity(text)
	out.VacancyRate = scrapeStatusInvestVacancy(text)
	// FFO Yield and Cap Rate are not published on Status Invest.
	if out.DividendYield == nil && out.PVP == nil {
		return nil
	}
	return out
}

func normalizeStatusInvestText(rawHTML string) string {
	withoutScripts := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(rawHTML, " ")
	withoutStyles := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(withoutScripts, " ")
	textOnly := regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(withoutStyles, " ")
	textOnly = html.UnescapeString(textOnly)
	textOnly = strings.ToLower(textOnly)
	textOnly = strings.NewReplacer(
		"á", "a",
		"à", "a",
		"ã", "a",
		"â", "a",
		"é", "e",
		"ê", "e",
		"í", "i",
		"ó", "o",
		"ô", "o",
		"õ", "o",
		"ú", "u",
		"ç", "c",
	).Replace(textOnly)
	return strings.Join(strings.Fields(textOnly), " ")
}

func scrapeStatusInvestDividendYield(text string) *float64 {
	section := statusInvestSection(text, "dividend yield", "valorizacao (12m)")
	return findLastPercent(section)
}

func scrapeStatusInvestPVP(text string) *float64 {
	section := statusInvestSection(text, "p/vp", "valor em caixa")
	return findFirstNumber(section)
}

func scrapeStatusInvestLastDividend(text string) *float64 {
	section := statusInvestSection(text, "ultimo rendimento", "proximo rendimento")
	return findFirstCurrency(section)
}

func scrapeStatusInvestLiquidity(text string) *float64 {
	section := statusInvestSection(text, "liquidez media diaria", "participacao no ifix")
	return findFirstCurrency(section)
}

func scrapeStatusInvestVacancy(text string) *float64 {
	section := statusInvestSection(text, "vacancia", "numero de imoveis")
	if section == "" {
		section = statusInvestSection(text, "vacancia", "gestao")
	}
	return findLastPercent(section)
}

func statusInvestSection(text, start, end string) string {
	startIdx := strings.Index(text, start)
	if startIdx < 0 {
		return ""
	}
	sub := text[startIdx:]
	endIdx := strings.Index(sub, end)
	if endIdx < 0 {
		return sub
	}
	return sub[:endIdx]
}

func findLastPercent(text string) *float64 {
	matches := regexp.MustCompile(`([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]+)?|[0-9]+(?:,[0-9]+)?)\s*%`).FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	return parseBrazilianNumber(matches[len(matches)-1][1])
}

func findFirstNumber(text string) *float64 {
	match := regexp.MustCompile(`(-|[0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]+)?|[0-9]+(?:,[0-9]+)?)`).FindStringSubmatch(text)
	if len(match) < 2 || match[1] == "-" {
		return nil
	}
	return parseBrazilianNumber(match[1])
}

func findFirstCurrency(text string) *float64 {
	match := regexp.MustCompile(`r\$\s*(-|[0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]+)?|[0-9]+(?:,[0-9]+)?)`).FindStringSubmatch(text)
	if len(match) < 2 || match[1] == "-" {
		return nil
	}
	return parseBrazilianNumber(match[1])
}

func parseBrazilianNumber(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return nil
	}
	if strings.Contains(raw, ",") {
		raw = strings.ReplaceAll(raw, ".", "")
		raw = strings.ReplaceAll(raw, ",", ".")
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}

func (s *Service) fetchFIIMetrics(ctx context.Context, tickers []string) map[string]*fiiScrapedData {
	out := make(map[string]*fiiScrapedData, len(tickers))
	if len(tickers) == 0 {
		return out
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, ticker := range tickers {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			data := s.scrapeStatusInvestFII(ctx, t)
			if data == nil {
				// Status Invest may be geo-blocked in production; fall back to Fundamentus.
				log.Printf("statusinvest fii: no data for %s, falling back to Fundamentus", t)
				data = s.scrapeFundamentusFII(ctx, t)
			}
			if data != nil {
				mu.Lock()
				out[strings.ToUpper(t)] = data
				mu.Unlock()
			}
		}(ticker)
	}
	wg.Wait()
	return out
}

func (s *Service) GetLatestFIIResults(ctx context.Context) (models.FIIResultsResponse, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT a.id, a.ticker, a.asset_type, COALESCE(m.company_name,''), COALESCE(m.tax_id,'')
		FROM positions p JOIN assets a ON p.asset_id = a.id
		LEFT JOIN asset_metadata m ON a.id = m.asset_id
		WHERE p.source='b3' AND a.asset_type IN ('fii','etf_or_fii','fund')`)
	if err != nil {
		return models.FIIResultsResponse{}, err
	}
	defer rows.Close()
	var tracked []models.TrackedAsset
	for rows.Next() {
		var item models.TrackedAsset
		if err := rows.Scan(&item.AssetID, &item.Ticker, &item.AssetType, &item.CompanyName, &item.TaxID); err != nil {
			return models.FIIResultsResponse{}, err
		}
		tracked = append(tracked, item)
	}
	if err := rows.Err(); err != nil {
		return models.FIIResultsResponse{}, err
	}
	if len(tracked) == 0 {
		return models.FIIResultsResponse{Items: []models.FIIResultItem{}}, nil
	}
	tickers := make([]string, 0, len(tracked))
	for _, asset := range tracked {
		tickers = append(tickers, asset.Ticker)
	}
	metricsMap := s.fetchFIIMetrics(ctx, tickers)
	items := make([]models.FIIResultItem, 0, len(tracked))
	for _, asset := range tracked {
		scraped := metricsMap[strings.ToUpper(asset.Ticker)]
		item := models.FIIResultItem{
			Ticker:      asset.Ticker,
			CompanyName: asset.CompanyName,
			AssetType:   asset.AssetType,
			Status:      "ok",
		}
		if scraped != nil {
			item.DividendYield = scraped.DividendYield
			item.PVP = scraped.PVP
			item.FFOYield = scraped.FFOYield
			item.DividendPerUnit = scraped.DividendPerUnit
			item.CapRate = scraped.CapRate
			item.VacancyRate = scraped.VacancyRate
			item.AvgDailyVolume = scraped.AvgDailyVolume
		} else {
			item.Status = "unavailable"
			item.Message = "Fundamentus data could not be loaded for this FII."
		}
		items = append(items, item)
	}
	return models.FIIResultsResponse{Items: items}, nil
}

func (s *Service) buildQuarterlyResult(ctx context.Context, asset models.TrackedAsset, taxIndex map[string][]cvmRow, nameIndex map[string][]cvmRow, dyMap map[string]*float64) models.QuarterlyResultItem {
	sentiment := s.GetOrRefreshSentiment(ctx, asset)
	var companyRows []cvmRow
	if asset.TaxID != "" {
		companyRows = taxIndex[asset.TaxID]
	}
	if len(companyRows) == 0 && asset.CompanyName != "" {
		companyRows = matchCompanyRows(asset.CompanyName, nameIndex)
	}
	if len(companyRows) == 0 {
		return models.QuarterlyResultItem{Ticker: asset.Ticker, CompanyName: asset.CompanyName, AssetType: asset.AssetType, Sentiment: sentiment, Highlights: []string{}, Status: "unavailable", Message: "No matching company was found in CVM ITR data for this holding."}
	}
	quarterRows := selectLatestQuarterRows(companyRows)
	if len(quarterRows) == 0 {
		return models.QuarterlyResultItem{Ticker: asset.Ticker, CompanyName: asset.CompanyName, AssetType: asset.AssetType, Sentiment: sentiment, Highlights: []string{}, Status: "unavailable", Message: "No quarter-length DRE rows were found for the latest filing period."}
	}
	revenue := extractRevenueMetric(quarterRows)
	netIncome := extractMetric(quarterRows, map[string]bool{"3.11": true, "3.13": true, "3.11.01": true}, []string{"LUCRO", "PREJU", "PERIODO"})
	reportDate := firstNonEmpty(quarterRows[0]["DT_FIM_EXERC"], quarterRows[0]["DT_REFER"])
	var margin *float64
	if revenue != nil && netIncome != nil && *revenue != 0 {
		v := (*netIncome / *revenue) * 100
		margin = &v
	}
	if revenue == nil && netIncome == nil {
		return models.QuarterlyResultItem{
			Ticker: asset.Ticker, CompanyName: firstNonEmpty(asset.CompanyName, quarterRows[0]["DENOM_CIA"]), AssetType: asset.AssetType, ReportDate: reportDate,
			Sentiment: sentiment, Highlights: []string{}, Status: "unavailable", Message: "Matched CVM company, but revenue and net income were not found in the latest DRE quarter rows.",
		}
	}
	return models.QuarterlyResultItem{
		Ticker: asset.Ticker, CompanyName: firstNonEmpty(asset.CompanyName, quarterRows[0]["DENOM_CIA"]), AssetType: asset.AssetType, ReportDate: reportDate,
		Revenue: revenue, NetIncome: netIncome, NetMargin: margin, DividendYield12M: dyMap[strings.ToUpper(asset.Ticker)], Sentiment: sentiment, Highlights: buildHighlights(revenue, netIncome, margin), Status: "ok",
	}
}

func matchCompanyRows(company string, idx map[string][]cvmRow) []cvmRow {
	key := normalizeCompanyName(company)
	if rows, ok := idx[key]; ok {
		return rows
	}
	target := tokenSet(key)
	best := ""
	bestScore := 0.0
	for candidate := range idx {
		score := jaccard(target, tokenSet(candidate))
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	if best != "" && bestScore >= 0.45 {
		return idx[best]
	}
	return nil
}

func selectLatestQuarterRows(rows []cvmRow) []cvmRow {
	var latest time.Time
	for _, row := range rows {
		if dt := parseDate(firstNonEmpty(row["DT_FIM_EXERC"], row["DT_REFER"])); dt.After(latest) {
			latest = dt
		}
	}
	if latest.IsZero() {
		return nil
	}
	var current []cvmRow
	for _, row := range rows {
		if parseDate(firstNonEmpty(row["DT_FIM_EXERC"], row["DT_REFER"])).Equal(latest) && normalizeUpper(row["ORDEM_EXERC"]) == "ULTIMO" {
			current = append(current, row)
		}
	}
	if len(current) == 0 {
		for _, row := range rows {
			if parseDate(firstNonEmpty(row["DT_FIM_EXERC"], row["DT_REFER"])).Equal(latest) {
				current = append(current, row)
			}
		}
	}
	grouped := map[string][]cvmRow{}
	for _, row := range current {
		key := row["DT_INI_EXERC"] + "|" + firstNonEmpty(row["DT_FIM_EXERC"], row["DT_REFER"])
		grouped[key] = append(grouped[key], row)
	}
	type group struct {
		days int
		rows []cvmRow
	}
	var quarterGroups []group
	for key, rows := range grouped {
		parts := strings.Split(key, "|")
		if len(parts) != 2 {
			continue
		}
		start, end := parseDate(parts[0]), parseDate(parts[1])
		if start.IsZero() || end.IsZero() {
			continue
		}
		days := int(end.Sub(start).Hours() / 24)
		if days >= 70 && days <= 120 {
			quarterGroups = append(quarterGroups, group{days: days, rows: rows})
		}
	}
	if len(quarterGroups) > 0 {
		sort.Slice(quarterGroups, func(i, j int) bool { return quarterGroups[i].days < quarterGroups[j].days })
		return latestVersionRows(quarterGroups[0].rows)
	}
	return latestVersionRows(current)
}

func latestVersionRows(rows []cvmRow) []cvmRow {
	best := 0
	for _, row := range rows {
		if v, _ := strconv.Atoi(strings.TrimSpace(row["VERSAO"])); v > best {
			best = v
		}
	}
	var out []cvmRow
	for _, row := range rows {
		if v, _ := strconv.Atoi(strings.TrimSpace(row["VERSAO"])); v == best {
			out = append(out, row)
		}
	}
	return out
}

func extractMetric(rows []cvmRow, codes map[string]bool, tokens []string) *float64 {
	for _, row := range rows {
		if codes[strings.TrimSpace(row["CD_CONTA"])] {
			if value := rowValue(row); value != nil {
				return value
			}
		}
	}
	for _, row := range rows {
		desc := normalizeCompanyName(row["DS_CONTA"])
		match := true
		for _, token := range tokens {
			if !strings.Contains(desc, normalizeCompanyName(token)) {
				match = false
				break
			}
		}
		if match {
			if value := rowValue(row); value != nil {
				return value
			}
		}
	}
	return nil
}

func extractRevenueMetric(rows []cvmRow) *float64 {
	if value := extractMetric(rows, map[string]bool{"3.01": true}, []string{"RECEITA"}); value != nil && *value != 0 {
		return value
	}
	type candidate struct {
		value float64
		desc  string
	}
	var candidates []candidate
	for _, row := range rows {
		desc := normalizeCompanyName(row["DS_CONTA"])
		if !strings.Contains(desc, "RECEITA") || strings.Contains(desc, "FINANCEIRA") {
			continue
		}
		value := rowValue(row)
		if value == nil || *value <= 0 {
			continue
		}
		candidates = append(candidates, candidate{value: *value, desc: desc})
	}
	if len(candidates) == 0 {
		return nil
	}
	priorities := [][]string{
		{"PRESTACAO", "SERVICOS"},
		{"OUTRAS", "RECEITAS", "OPERACIONAIS"},
		{"RECEITAS", "OPERACIONAIS"},
	}
	for _, tokens := range priorities {
		best := -1.0
		for _, cand := range candidates {
			match := true
			for _, token := range tokens {
				if !strings.Contains(cand.desc, token) {
					match = false
					break
				}
			}
			if match && cand.value > best {
				best = cand.value
			}
		}
		if best >= 0 {
			return &best
		}
	}
	best := candidates[0].value
	for _, cand := range candidates[1:] {
		if cand.value > best {
			best = cand.value
		}
	}
	return &best
}

func rowValue(row cvmRow) *float64 {
	raw := strings.TrimSpace(row["VL_CONTA"])
	if raw == "" {
		return nil
	}
	value, err := parseCVMNumber(raw)
	if err != nil {
		return nil
	}
	scale := 1.0
	switch normalizeUpper(row["ESCALA_MOEDA"]) {
	case "MIL", "MILHAR", "MILHARES", "R$ MIL":
		scale = 1000
	case "MILHAO", "MILHOES", "R$ MILHOES":
		scale = 1_000_000
	}
	result := value * scale
	return &result
}

func parseCVMNumber(raw string) (float64, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, errors.New("empty")
	}
	if strings.Contains(text, ",") && strings.Contains(text, ".") {
		text = strings.ReplaceAll(text, ".", "")
		text = strings.ReplaceAll(text, ",", ".")
	} else if strings.Contains(text, ",") {
		text = strings.ReplaceAll(text, ".", "")
		text = strings.ReplaceAll(text, ",", ".")
	}
	return strconv.ParseFloat(text, 64)
}

func normalizeTaxID(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeCompanyName(value string) string {
	text := strings.ToUpper(strings.TrimSpace(value))
	replacements := map[string]string{
		"S.A.":  "SA",
		"S/A":   "SA",
		" BCO ": " BANCO ",
		" CIA ": " COMPANHIA ",
	}
	for old, newValue := range replacements {
		text = strings.ReplaceAll(text, old, newValue)
	}
	text = removeAccents(text)
	reg := regexp.MustCompile(`[^A-Z0-9 ]`)
	text = reg.ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(text), " ")
}

func normalizeUpper(value string) string {
	return strings.ReplaceAll(removeAccents(strings.ToUpper(strings.TrimSpace(value))), "Ú", "U")
}

func removeAccents(value string) string {
	replacer := strings.NewReplacer(
		"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
		"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
		"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
		"Ç", "C",
	)
	return replacer.Replace(value)
}

func decodeLatin1(content []byte) string {
	runes := make([]rune, len(content))
	for i, b := range content {
		runes[i] = rune(b)
	}
	return string(runes)
}

func parseDate(value string) time.Time {
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if dt, err := time.Parse(layout, value); err == nil {
			return dt
		}
	}
	return time.Time{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildHighlights(revenue, netIncome, margin *float64) []string {
	var out []string
	if revenue != nil {
		out = append(out, "Revenue "+formatBRL(*revenue))
	}
	if netIncome != nil {
		out = append(out, "Net income "+formatBRL(*netIncome))
	}
	if margin != nil {
		out = append(out, fmt.Sprintf("Net margin %.1f%%", *margin))
	}
	return out
}

func formatBRL(value float64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = math.Abs(value)
	}
	switch {
	case value >= 1_000_000_000:
		return fmt.Sprintf("%sR$ %.2fB", sign, value/1_000_000_000)
	case value >= 1_000_000:
		return fmt.Sprintf("%sR$ %.2fM", sign, value/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%sR$ %.1fK", sign, value/1_000)
	default:
		return fmt.Sprintf("%sR$ %.2f", sign, value)
	}
}

func tokenSet(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, token := range strings.Fields(value) {
		out[token] = struct{}{}
	}
	return out
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	union := map[string]struct{}{}
	for key := range a {
		union[key] = struct{}{}
		if _, ok := b[key]; ok {
			inter++
		}
	}
	for key := range b {
		union[key] = struct{}{}
	}
	return float64(inter) / float64(len(union))
}
