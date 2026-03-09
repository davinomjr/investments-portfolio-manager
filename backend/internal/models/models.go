package models

type ImportJobResponse struct {
	ID        int64  `json:"id"`
	Source    string `json:"source"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type PositionResponse struct {
	Ticker      string  `json:"ticker"`
	AssetType   string  `json:"asset_type"`
	Quantity    float64 `json:"quantity"`
	AvgPrice    float64 `json:"avg_price"`
	Broker      string  `json:"broker,omitempty"`
	Source      string  `json:"source"`
	LastUpdated string  `json:"last_updated"`
}

type AllocationItem struct {
	Ticker      string  `json:"ticker"`
	AssetType   string  `json:"asset_type"`
	MarketValue float64 `json:"market_value"`
	Weight      float64 `json:"weight"`
}

type PortfolioResponse struct {
	TotalPositions     int              `json:"total_positions"`
	EstimatedCostBasis float64          `json:"estimated_cost_basis"`
	Allocations        []AllocationItem `json:"allocations"`
}

type QuarterlyResultItem struct {
	Ticker      string           `json:"ticker"`
	CompanyName string           `json:"company_name,omitempty"`
	AssetType   string           `json:"asset_type"`
	ReportDate  string           `json:"report_date,omitempty"`
	Revenue     *float64         `json:"revenue"`
	NetIncome   *float64         `json:"net_income"`
	EBITDA      *float64         `json:"ebitda"`
	NetMargin   *float64         `json:"net_margin"`
	Sentiment   *TickerSentiment `json:"sentiment"`
	Highlights  []string         `json:"highlights"`
	Status      string           `json:"status"`
	Message     string           `json:"message,omitempty"`
}

type QuarterlyResultsResponse struct {
	Provider   string                `json:"provider"`
	Configured bool                  `json:"configured"`
	Message    string                `json:"message,omitempty"`
	Items      []QuarterlyResultItem `json:"items"`
}

type MonteCarloParams struct {
	Years          int     `json:"years"`
	Simulations    int     `json:"simulations"`
	ExpectedReturn float64 `json:"expected_return"`
	Volatility     float64 `json:"volatility"`
}

type MonteCarloYearPoint struct {
	Year         int     `json:"year"`
	P10          float64 `json:"p10"`
	P50          float64 `json:"p50"`
	P90          float64 `json:"p90"`
	Average      float64 `json:"average"`
	BestCase     float64 `json:"best_case"`
	WorstCase    float64 `json:"worst_case"`
	ProbPositive float64 `json:"prob_positive"`
}

type MonteCarloResponse struct {
	InitialValue float64               `json:"initial_value"`
	Params       MonteCarloParams      `json:"params"`
	Timeline     []MonteCarloYearPoint `json:"timeline"`
	Message      string                `json:"message"`
}

type HoldingPayload struct {
	Ticker       string  `json:"ticker"`
	Quantity     float64 `json:"quantity"`
	AveragePrice float64 `json:"average_price"`
	Broker       string  `json:"broker"`
	AssetType    string  `json:"asset_type"`
	Currency     string  `json:"currency"`
	CompanyName  string  `json:"company_name"`
	TaxID        string  `json:"tax_id"`
}

type WorkerImportResponse struct {
	Holdings []HoldingPayload `json:"holdings"`
	Source   string           `json:"source"`
}

type TrackedAsset struct {
	AssetID     int64
	Ticker      string
	AssetType   string
	CompanyName string
	TaxID       string
}

type SentimentSource struct {
	SourceType  string   `json:"source_type"`
	Provider    string   `json:"provider"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	PublishedAt string   `json:"published_at,omitempty"`
	Excerpt     string   `json:"excerpt,omitempty"`
	Score       *float64 `json:"score"`
	Weight      float64  `json:"weight"`
}

type TickerSentiment struct {
	Status          string            `json:"status"`
	Label           string            `json:"label,omitempty"`
	Score           *float64          `json:"score"`
	Confidence      *float64          `json:"confidence"`
	Trend           string            `json:"trend,omitempty"`
	SourceCount     int               `json:"source_count"`
	LastRefreshedAt string            `json:"last_refreshed_at,omitempty"`
	IsStale         bool              `json:"is_stale"`
	Message         string            `json:"message,omitempty"`
	Sources         []SentimentSource `json:"sources"`
}
