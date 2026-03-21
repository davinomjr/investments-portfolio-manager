export type Position = {
  ticker: string;
  asset_type: string;
  quantity: number;
  avg_price: number;
  broker: string | null;
  source: string;
  last_updated: string;
  hidden: boolean;
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

export type FIIResultItem = {
  ticker: string;
  company_name: string | null;
  asset_type: string;
  dividend_yield: number | null;
  pvp: number | null;
  ffo_yield: number | null;
  dividend_per_unit: number | null;
  cap_rate: number | null;
  vacancy_rate: number | null;
  avg_daily_volume: number | null;
  status: string;
  message: string | null;
};

export type FIIResultsResponse = { items: FIIResultItem[] };

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

async function serverFetch(path: string): Promise<Response> {
  const { cookies } = await import("next/headers");
  const jar = await cookies();
  const token = jar.get("auth_token")?.value;
  return fetch(`${API_BASE}${path}`, {
    cache: "no-store",
    headers: token ? { Cookie: `auth_token=${token}` } : {},
  });
}

export async function fetchPortfolio(): Promise<Portfolio> {
  const response = await serverFetch("/portfolio");
  if (!response.ok) {
    throw new Error("Failed to load portfolio.");
  }
  return response.json();
}

export async function fetchPositions(): Promise<Position[]> {
  const response = await serverFetch("/positions");
  if (!response.ok) {
    throw new Error("Failed to load positions.");
  }
  return response.json();
}

export async function fetchQuarterlyResults(): Promise<QuarterlyResultsResponse> {
  const response = await serverFetch("/stocks/latest-results");
  if (!response.ok) {
    throw new Error("Failed to load latest quarter results.");
  }
  return response.json();
}

export async function fetchFIIResults(): Promise<FIIResultsResponse> {
  const response = await serverFetch("/fiis/latest-results");
  if (!response.ok) {
    throw new Error("Failed to load FII results.");
  }
  return response.json();
}

export async function fetchLatestImportJob(): Promise<ImportJobResponse | null> {
  const response = await serverFetch("/portfolio/import-jobs/latest");
  if (response.status === 404) return null;
  if (!response.ok) throw new Error("Failed to load latest import job.");
  return response.json();
}

export async function setPositionsVisibility(visible: boolean): Promise<void> {
  const response = await fetch(`/api/positions/visibility`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ visible }),
  });
  if (!response.ok && response.status !== 204) {
    throw new Error("Failed to update positions visibility.");
  }
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
