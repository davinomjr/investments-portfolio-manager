package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"investments-portfolio-manager/backend/internal/models"
)

// Pluggy (Open Finance aggregator) integration via "Meu Pluggy": the user
// connects their own brokers at meu.pluggy.ai and the backend reads the
// resulting items through the Pluggy API with the dashboard app's client
// credentials. Runs entirely server-side, no browser needed.

const defaultPluggyBaseURL = "https://api.pluggy.ai"

// PluggyInvestment is the subset of Pluggy's /investments item we use.
type PluggyInvestment struct {
	ID             string   `json:"id"`
	ItemID         string   `json:"itemId"`
	Name           string   `json:"name"`
	Code           string   `json:"code"`
	ISIN           string   `json:"isin"`
	Type           string   `json:"type"`
	Subtype        string   `json:"subtype"`
	Status         string   `json:"status"`
	Quantity       *float64 `json:"quantity"`
	Value          *float64 `json:"value"`
	Amount         *float64 `json:"amount"`
	AmountOriginal *float64 `json:"amountOriginal"`
	Balance        float64  `json:"balance"`
	CurrencyCode   string   `json:"currencyCode"`
	Issuer         string   `json:"issuer"`
	DueDate        string   `json:"dueDate"`
	RateType       string   `json:"rateType"`
	Institution    *struct {
		Name   string `json:"name"`
		Number string `json:"number"`
	} `json:"institution"`
}

// PluggyPreview is returned by the preview endpoint so the mapping can be
// compared against the B3 sync before switching sources.
type PluggyPreview struct {
	Holdings []models.HoldingPayload `json:"holdings"`
	Skipped  []PluggySkipped         `json:"skipped"`
	Raw      []PluggyInvestment      `json:"raw,omitempty"`
}

type PluggySkipped struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Reason  string `json:"reason"`
}

func (s *Service) pluggyBaseURL() string {
	if s.Config.PluggyBaseURL != "" {
		return strings.TrimRight(s.Config.PluggyBaseURL, "/")
	}
	return defaultPluggyBaseURL
}

func (s *Service) pluggyConfigured() error {
	if s.Config.PluggyClientID == "" || s.Config.PluggyClientSecret == "" || len(s.Config.PluggyItemIDs) == 0 {
		return fmt.Errorf("PLUGGY_CLIENT_ID, PLUGGY_CLIENT_SECRET and PLUGGY_ITEM_IDS must be configured")
	}
	return nil
}

func (s *Service) pluggyAPIKey(ctx context.Context, client *http.Client) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"clientId":     s.Config.PluggyClientID,
		"clientSecret": s.Config.PluggyClientSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.pluggyBaseURL()+"/auth", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("pluggy auth: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("pluggy auth: status %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var out struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.APIKey == "" {
		return "", fmt.Errorf("pluggy auth: no apiKey in response")
	}
	return out.APIKey, nil
}

// checkPluggyItem fails the sync when an item is not healthy (expired
// consent, login error) so stale data never silently replaces positions.
func (s *Service) checkPluggyItem(ctx context.Context, client *http.Client, apiKey, itemID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.pluggyBaseURL()+"/items/"+url.PathEscape(itemID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("pluggy item %s: %w", itemID, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pluggy item %s: status %d: %s", itemID, resp.StatusCode, truncate(string(raw), 300))
	}
	var item struct {
		Status        string `json:"status"`
		LastUpdatedAt string `json:"lastUpdatedAt"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return fmt.Errorf("pluggy item %s: decode: %w", itemID, err)
	}
	switch item.Status {
	case "UPDATED", "UPDATING":
		return nil
	}
	return fmt.Errorf("pluggy item %s is %s (last updated %s) — reconnect it in Meu Pluggy", itemID, item.Status, item.LastUpdatedAt)
}

// fetchPluggyInvestments pages through /investments for every configured item.
func (s *Service) fetchPluggyInvestments(ctx context.Context) ([]PluggyInvestment, error) {
	if err := s.pluggyConfigured(); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	apiKey, err := s.pluggyAPIKey(ctx, client)
	if err != nil {
		return nil, err
	}
	all := make([]PluggyInvestment, 0)
	for _, itemID := range s.Config.PluggyItemIDs {
		if err := s.checkPluggyItem(ctx, client, apiKey, itemID); err != nil {
			return nil, err
		}
		for page := 1; ; page++ {
			q := url.Values{}
			q.Set("itemId", itemID)
			q.Set("pageSize", "500")
			q.Set("page", strconv.Itoa(page))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.pluggyBaseURL()+"/investments?"+q.Encode(), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("X-API-KEY", apiKey)
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("pluggy investments (item %s): %w", itemID, err)
			}
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("pluggy investments (item %s): status %d: %s", itemID, resp.StatusCode, truncate(string(raw), 300))
			}
			var out struct {
				TotalPages int                `json:"totalPages"`
				Results    []PluggyInvestment `json:"results"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				return nil, fmt.Errorf("pluggy investments (item %s): decode: %w", itemID, err)
			}
			for i := range out.Results {
				if out.Results[i].ItemID == "" {
					out.Results[i].ItemID = itemID
				}
			}
			all = append(all, out.Results...)
			if page >= out.TotalPages || len(out.Results) == 0 {
				break
			}
		}
	}
	return all, nil
}

// mapPluggyInvestments converts Pluggy investments into the holdings shape
// used by every other import source, merging the same ticker across brokers.
//
// Field semantics observed on real MeuPluggy data:
//   - equities/ETFs/FIIs: `code` is the B3 ticker, `value` the closing price,
//     `amountOriginal` is null (no cost basis), `institution` is null;
//   - treasury: `code` is the ISIN, `name` the bond (LFT, NTNB PRINC...),
//     `rateType` the indexer and `amountOriginal` the real invested amount;
//   - funds: `code` is the CNPJ;
//   - zeroed positions come back with status TOTAL_WITHDRAWAL.
//
// As with the B3 export, equities get the closing price as average price;
// a user-entered price (avg_price_manual) survives the sync in upsertHoldings.
func mapPluggyInvestments(investments []PluggyInvestment) ([]models.HoldingPayload, []PluggySkipped) {
	holdings := make([]models.HoldingPayload, 0, len(investments))
	index := map[string]int{}
	skipped := make([]PluggySkipped, 0)
	for _, inv := range investments {
		skip := func(reason string) {
			skipped = append(skipped, PluggySkipped{Name: inv.Name, Type: inv.Type, Subtype: inv.Subtype, Reason: reason})
		}
		if inv.Status != "" && !strings.EqualFold(inv.Status, "ACTIVE") {
			skip("status " + inv.Status)
			continue
		}
		assetType := pluggyAssetType(inv.Type, inv.Subtype)
		if assetType == "" {
			skip("unsupported type")
			continue
		}
		quantity := 0.0
		if inv.Quantity != nil {
			quantity = *inv.Quantity
		}
		if quantity <= 0 {
			skip("zero quantity")
			continue
		}

		var ticker string
		displayName := inv.Name
		switch assetType {
		case "government_bond":
			ticker = pluggyTreasuryTicker(inv)
			displayName = pluggyTreasuryLabel(inv)
		case "fund", "other":
			ticker = normalizeTickerText(inv.Name)
			if len(ticker) > 40 {
				ticker = ticker[:40]
			}
			displayName = shortFundName(inv.Name)
		default:
			ticker = strings.ToUpper(strings.TrimSpace(inv.Code))
		}
		if ticker == "" {
			skip("no ticker")
			continue
		}

		var avg float64
		switch {
		case inv.AmountOriginal != nil && *inv.AmountOriginal > 0:
			avg = *inv.AmountOriginal / quantity
		case inv.Value != nil && *inv.Value > 0:
			avg = *inv.Value
		case inv.Balance > 0:
			avg = inv.Balance / quantity
		}

		broker := ""
		if inv.Institution != nil {
			broker = inv.Institution.Name
		}

		if i, ok := index[ticker]; ok {
			h := &holdings[i]
			total := h.Quantity + quantity
			h.AveragePrice = (h.AveragePrice*h.Quantity + avg*quantity) / total
			h.Quantity = total
			if broker != "" && !strings.Contains(h.Broker, broker) {
				h.Broker = strings.TrimPrefix(h.Broker+", "+broker, ", ")
			}
			continue
		}
		index[ticker] = len(holdings)
		holdings = append(holdings, models.HoldingPayload{
			Ticker:       ticker,
			Quantity:     quantity,
			AveragePrice: avg,
			Broker:       broker,
			AssetType:    assetType,
			Currency:     defaultString(inv.CurrencyCode, "BRL"),
			CompanyName:  displayName,
		})
	}
	for i := range holdings {
		holdings[i].AveragePrice = math.Round(holdings[i].AveragePrice*1e6) / 1e6
	}
	return holdings, skipped
}

func pluggyAssetType(typ, subtype string) string {
	switch strings.ToUpper(typ) {
	case "EQUITY":
		switch strings.ToUpper(subtype) {
		case "STOCK":
			return "stock"
		case "REAL_ESTATE_FUND":
			return "fii"
		case "BDR":
			return "bdr"
		case "ETF":
			return "etf"
		case "DERIVATIVES", "OPTION":
			return ""
		}
		return "stock"
	case "ETF":
		return "etf"
	case "MUTUAL_FUND":
		return "fund"
	case "FIXED_INCOME":
		if strings.EqualFold(subtype, "TREASURY") {
			return "government_bond"
		}
		return "other"
	}
	return ""
}

// pluggyTreasuryTicker builds the `TD<INDEXER><YEAR>` ticker the B3 worker
// produces (see parseTDTicker), so Tesouro rows keep their asset and quotes.
func pluggyTreasuryTicker(inv PluggyInvestment) string {
	year := ""
	if len(inv.DueDate) >= 4 {
		year = inv.DueDate[:4]
	}
	if year == "" {
		return strings.ToUpper(inv.Code)
	}
	indexer := ""
	switch strings.ToUpper(strings.ReplaceAll(inv.RateType, "-", "_")) {
	case "SELIC":
		indexer = "SELIC"
	case "IPCA":
		indexer = "IPCA"
	case "IGPM", "IGP_M":
		indexer = "IGPM"
	case "PRE_FIXADO", "PREFIXADO", "PRE":
		indexer = "PREFIXADO"
	}
	if indexer == "" {
		name := normalizeTickerText(inv.Name)
		switch {
		case strings.HasPrefix(name, "LFT"):
			indexer = "SELIC"
		case strings.HasPrefix(name, "LTN"), strings.HasPrefix(name, "NTNF"):
			indexer = "PREFIXADO"
		case strings.HasPrefix(name, "NTNB"):
			indexer = "IPCA"
		case strings.HasPrefix(name, "NTNC"):
			indexer = "IGPM"
		default:
			indexer = name
		}
	}
	return "TD" + indexer + year
}

// pluggyTreasuryLabel gives a short, readable name for a Tesouro bond, e.g.
// "TD NTNB 2040" (NTN-B Principal), "TD NTNB JS 2035" (NTN-B with semiannual
// coupons) or "TD SELIC 2031" (LFT).
func pluggyTreasuryLabel(inv PluggyInvestment) string {
	year := ""
	if len(inv.DueDate) >= 4 {
		year = inv.DueDate[:4]
	}
	name := normalizeTickerText(inv.Name)
	bond := ""
	switch {
	case strings.HasPrefix(name, "LFT"):
		bond = "SELIC"
	case strings.HasPrefix(name, "LTN"):
		bond = "PRE"
	case strings.HasPrefix(name, "NTNF"):
		bond = "PRE JS"
	case strings.HasPrefix(name, "NTNBPRINC"):
		bond = "NTNB"
	case strings.HasPrefix(name, "NTNB1"), strings.HasPrefix(name, "NTNB"):
		bond = "NTNB JS"
	case strings.HasPrefix(name, "NTNC"):
		bond = "IGPM"
	default:
		indexer, _, ok := parseTDTicker(pluggyTreasuryTicker(inv))
		if !ok {
			return strings.TrimSpace(inv.Name)
		}
		bond = indexer
	}
	return strings.TrimSpace("TD " + bond + " " + year)
}

// fundNameStopWords mark where a fund's legal name stops being descriptive
// ("Sparta Debêntures Incentivadas Inflação FIC de Fundos ...").
var fundNameStopWords = map[string]struct{}{
	"FIC": {}, "FIF": {}, "FI": {}, "FIM": {}, "FIA": {}, "FIRF": {}, "RF": {}, "CP": {}, "RL": {},
	"DE": {}, "FUNDO": {}, "FUNDOS": {}, "-": {}, "–": {},
}

// shortFundName keeps up to the first four descriptive words of a fund name.
func shortFundName(name string) string {
	words := strings.Fields(name)
	out := make([]string, 0, 4)
	for _, w := range words {
		if _, stop := fundNameStopWords[strings.ToUpper(w)]; stop && len(out) > 0 {
			break
		}
		if len(out) > 0 && strings.IndexFunc(w, func(r rune) bool { return r >= '0' && r <= '9' }) == 0 {
			break // stop before share-class numbers like "Hedge 30"
		}
		out = append(out, w)
		if len(out) == 4 {
			break
		}
	}
	if len(out) == 0 {
		return strings.TrimSpace(name)
	}
	return strings.Join(out, " ")
}

// normalizeTickerText mirrors worker/app/parser.py normalize_ticker.
func normalizeTickerText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(value) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// PreviewPluggy fetches and maps Pluggy investments without touching the DB.
func (s *Service) PreviewPluggy(ctx context.Context, includeRaw bool) (PluggyPreview, error) {
	investments, err := s.fetchPluggyInvestments(ctx)
	if err != nil {
		return PluggyPreview{}, err
	}
	holdings, skipped := mapPluggyInvestments(investments)
	preview := PluggyPreview{Holdings: holdings, Skipped: skipped}
	if includeRaw {
		preview.Raw = investments
	}
	return preview, nil
}

// ImportPluggy syncs positions from Pluggy in the background (202 + job).
func (s *Service) ImportPluggy(ctx context.Context) (models.ImportJobResponse, error) {
	if err := s.pluggyConfigured(); err != nil {
		return models.ImportJobResponse{}, err
	}
	jobID, job, err := s.createJob(ctx, "pluggy", "running", "Pluggy import started")
	if err != nil {
		return models.ImportJobResponse{}, err
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		_ = s.runPluggyJob(bgCtx, jobID)
	}()
	return job, nil
}

// runPluggyJob runs a Pluggy sync synchronously, recording the outcome on jobID.
func (s *Service) runPluggyJob(ctx context.Context, jobID int64) error {
	fail := func(err error) error {
		s.updateJob(ctx, jobID, "failed", err.Error())
		return err
	}
	investments, err := s.fetchPluggyInvestments(ctx)
	if err != nil {
		return fail(err)
	}
	holdings, skipped := mapPluggyInvestments(investments)
	if len(holdings) == 0 {
		return fail(fmt.Errorf("Pluggy returned no importable positions"))
	}
	if err := s.upsertHoldings(ctx, holdings, "pluggy"); err != nil {
		return fail(err)
	}
	detail := fmt.Sprintf("Imported %d positions from Pluggy (%d skipped)", len(holdings), len(skipped))
	if s.Config.PluggyReplacesB3 {
		// Positions B3 still owns after the upsert are ones Pluggy no
		// longer reports (sold, or a broker not connected to Meu Pluggy).
		removed, err := s.removePositionsBySource(ctx, "b3")
		if err != nil {
			return fail(err)
		}
		if removed > 0 {
			detail += fmt.Sprintf(", removed %d leftover B3 positions", removed)
		}
	}
	s.updateJob(ctx, jobID, "completed", detail)
	s.snapshotAfterImport()
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
