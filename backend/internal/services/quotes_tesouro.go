package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// tdProduct is one row from the Tesouro Direto products dataset.
type tdProduct struct {
	Name     string // "Tesouro IPCA+ (2032)"
	PUVenda  float64
	Rate     float64
	AsOfDate time.Time
}

// tdIndexerProductPrefix maps the indexer code embedded in the user's bond
// ticker to the product-name prefix used in the Treasury dataset. The ticker
// format produced by the worker is `TD<INDEXER><YEAR>` (worker/app/parser.py).
var tdIndexerProductPrefix = map[string]string{
	"IPCA":      "Tesouro IPCA+",
	"PREFIXADO": "Tesouro Prefixado",
	"PRE":       "Tesouro Prefixado",
	"SELIC":     "Tesouro Selic",
	"EDUCA":     "Tesouro Educa+",
	"RENDA":     "Tesouro Renda+ Aposentadoria Extra",
	"IGPM":      "Tesouro IGPM+ com Juros Semestrais",
}

// parseTDTicker splits `TDIPCA2032` into ("IPCA", "2032").
func parseTDTicker(ticker string) (indexer, year string, ok bool) {
	if !strings.HasPrefix(ticker, "TD") {
		return "", "", false
	}
	rest := ticker[2:]
	if len(rest) < 5 {
		return "", "", false
	}
	year = rest[len(rest)-4:]
	for _, r := range year {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	return rest[:len(rest)-4], year, true
}

// fetchTesouroDiretoQuote resolves one bond ticker through the cached daily
// snapshot of all Tesouro Direto products.
func (s *Service) fetchTesouroDiretoQuote(ctx context.Context, ticker string) (Quote, error) {
	snapshot, err := s.tesouroDiretoSnapshot(ctx)
	if err != nil {
		return Quote{}, err
	}
	indexer, year, ok := parseTDTicker(ticker)
	if !ok {
		return Quote{}, fmt.Errorf("td: cannot parse ticker %q", ticker)
	}
	prefix, ok := tdIndexerProductPrefix[indexer]
	if !ok {
		return Quote{}, fmt.Errorf("td: unknown indexer %q in ticker %q", indexer, ticker)
	}
	wanted := fmt.Sprintf("%s (%s)", prefix, year)
	p, ok := snapshot[wanted]
	if !ok {
		return Quote{}, fmt.Errorf("td: product %q not found", wanted)
	}
	return Quote{
		Ticker:        ticker,
		LastPrice:     p.PUVenda,
		PreviousClose: p.PUVenda, // AA40 doesn't expose prior PU; daily change would be 0
		Currency:      "BRL",
		FetchedAt:     time.Now(),
	}, nil
}

// tesouroDiretoSnapshot returns the latest TD product index, fetching it from
// the Treasury's open-data portal when the cached copy is older than
// TesouroDiretoTTL. The snapshot is shared across all government_bond lookups.
func (s *Service) tesouroDiretoSnapshot(ctx context.Context) (map[string]tdProduct, error) {
	s.tdMu.Lock()
	defer s.tdMu.Unlock()
	if s.tdSnapshot != nil && time.Since(s.tdSnapshotAt) < s.Config.TesouroDiretoTTL {
		return s.tdSnapshot, nil
	}
	snap, err := s.fetchTesouroDiretoIndex(ctx)
	if err != nil {
		if s.tdSnapshot != nil {
			return s.tdSnapshot, nil
		}
		return nil, err
	}
	s.tdSnapshot = snap
	s.tdSnapshotAt = time.Now()
	return snap, nil
}

// PrecoTaxaTesouroDireto.csv is the official daily-published price + yield
// dataset for every Tesouro Direto product, hosted on the Treasury's open-data
// portal. ~14 MB, semicolon-delimited, Brazilian decimal format.
const tesouroDiretoCSVURL = "https://www.tesourotransparente.gov.br/ckan/dataset/df56aa42-484a-4a59-8184-7676580c81e3/resource/796d2059-14e9-44e3-80c9-2d9e30b405c1/download/PrecoTaxaTesouroDireto.csv"

func (s *Service) fetchTesouroDiretoIndex(ctx context.Context) (map[string]tdProduct, error) {
	// The CSV is large; use a generous timeout independent of QUOTES_HTTP_TIMEOUT.
	reqCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, tesouroDiretoCSVURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PortfolioManager/1.0)")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("td gov.br http %d", resp.StatusCode)
	}
	return parseTesouroDiretoCSV(resp.Body)
}

// parseTesouroDiretoCSV reads the gov.br Treasury CSV and returns the latest
// PU Venda (sell-back price) for each (product, maturity-year) pair.
func parseTesouroDiretoCSV(body io.Reader) (map[string]tdProduct, error) {
	reader := csv.NewReader(body)
	reader.Comma = ';'
	reader.FieldsPerRecord = -1

	type key struct{ tipo, year string }
	type best struct {
		baseDate time.Time
		pu       float64
		rate     float64
	}
	latest := map[key]best{}

	headerSeen := false
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if !headerSeen {
			headerSeen = true
			continue
		}
		if len(row) < 7 {
			continue
		}
		tipo := strings.TrimSpace(row[0])
		venc := strings.TrimSpace(row[1])
		baseStr := strings.TrimSpace(row[2])
		rateStr := strings.TrimSpace(row[4])
		puStr := strings.TrimSpace(row[6])
		if len(venc) < 10 {
			continue
		}
		year := venc[6:10]
		baseDate, err := time.Parse("02/01/2006", baseStr)
		if err != nil {
			continue
		}
		pu, err := strconv.ParseFloat(strings.ReplaceAll(puStr, ",", "."), 64)
		if err != nil || pu <= 0 {
			continue
		}
		rate, _ := strconv.ParseFloat(strings.ReplaceAll(rateStr, ",", "."), 64)

		k := key{tipo, year}
		if existing, ok := latest[k]; !ok || baseDate.After(existing.baseDate) {
			latest[k] = best{baseDate, pu, rate}
		}
	}

	out := make(map[string]tdProduct, len(latest))
	for k, v := range latest {
		name := fmt.Sprintf("%s (%s)", k.tipo, k.year)
		out[name] = tdProduct{Name: name, PUVenda: v.pu, Rate: v.rate, AsOfDate: v.baseDate}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("td: empty index")
	}
	return out, nil
}
