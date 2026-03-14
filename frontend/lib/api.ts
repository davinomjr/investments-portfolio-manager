export type Position = {
  ticker: string;
  asset_type: string;
  quantity: number;
  avg_price: number;
  broker: string | null;
  source: string;
  last_updated: string;
};

export type Allocation = {
  ticker: string;
  asset_type: string;
  market_value: number;
  weight: number;
};

export type Portfolio = {
  total_positions: number;
  estimated_cost_basis: number;
  allocations: Allocation[];
};

export type QuarterlyResultItem = {
  ticker: string;
  company_name: string | null;
  asset_type: string;
  report_date: string | null;
  revenue: number | null;
  net_income: number | null;
  net_margin: number | null;
  dividend_yield_12m: number | null;
  sentiment: TickerSentiment | null;
  highlights: string[];
  status: string;
  message: string | null;
};

export type SentimentSource = {
  source_type: string;
  provider: string;
  title: string;
  url: string;
  published_at: string | null;
  excerpt: string | null;
  score: number | null;
  weight: number;
};

export type TickerSentiment = {
  status: string;
  label: string | null;
  score: number | null;
  confidence: number | null;
  trend: string | null;
  source_count: number;
  last_refreshed_at: string | null;
  is_stale: boolean;
  message: string | null;
  sources: SentimentSource[];
};

export type QuarterlyResultsResponse = {
  provider: string;
  configured: boolean;
  message: string | null;
  items: QuarterlyResultItem[];
};

export type MonteCarloParams = {
  years: number;
  simulations: number;
  expected_return: number;
  volatility: number;
};

export type MonteCarloYearPoint = {
  year: number;
  p10: number;
  p50: number;
  p90: number;
  average: number;
  best_case: number;
  worst_case: number;
  prob_positive: number;
};

export type MonteCarloResponse = {
  initial_value: number;
  params: MonteCarloParams;
  timeline: MonteCarloYearPoint[];
  message: string;
};

export type ImportJobResponse = {
  id: number;
  source: string;
  status: string;
  detail?: string;
  created_at: string;
  updated_at: string;
};

const API_BASE = process.env.INTERNAL_API_BASE_URL ?? "http://127.0.0.1:8000";

export async function fetchPortfolio(): Promise<Portfolio> {
  const response = await fetch(`${API_BASE}/portfolio`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Failed to load portfolio.");
  }
  return response.json();
}

export async function fetchPositions(): Promise<Position[]> {
  const response = await fetch(`${API_BASE}/positions`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Failed to load positions.");
  }
  return response.json();
}

export async function fetchQuarterlyResults(): Promise<QuarterlyResultsResponse> {
  const response = await fetch(`${API_BASE}/stocks/latest-results`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Failed to load latest quarter results.");
  }
  return response.json();
}

export async function fetchLatestImportJob(): Promise<ImportJobResponse | null> {
  const response = await fetch(`${API_BASE}/portfolio/import-jobs/latest`, { cache: "no-store" });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error("Failed to load latest import job.");
  return response.json();
}

export async function fetchMonteCarloSimulation(
  query: Partial<{ years: number; simulations: number; expected_return: number; volatility: number }> = {},
): Promise<MonteCarloResponse> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined) {
      params.set(key, String(value));
    }
  }
  const path = params.size > 0 ? `/portfolio/monte-carlo?${params.toString()}` : "/portfolio/monte-carlo";
  const response = await fetch(`${API_BASE}${path}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Failed to load Monte Carlo simulation.");
  }
  return response.json();
}
