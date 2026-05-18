export const dynamic = "force-dynamic";

import { AllocationChart } from "@/components/allocation-chart";
import { PositionsTable } from "@/components/positions-table";
import { SummaryCards } from "@/components/summary-cards";
import { UploadPanel } from "@/components/upload-panel";
import {
  fetchLatestImportJob,
  fetchPortfolio,
  fetchPositions,
  type ImportJobResponse,
  type Portfolio,
  type Position,
} from "@/lib/api";

const EMPTY_PORTFOLIO: Portfolio = {
  total_positions: 0,
  estimated_cost_basis: 0,
  market_value_brl: 0,
  pnl_brl: 0,
  pnl_pct: null,
  quotes_status: "unavailable",
  allocations: [],
};

export default async function HomePage() {
  let portfolio: Portfolio = EMPTY_PORTFOLIO;
  let positions: Position[] = [];
  let latestB3Job: ImportJobResponse | null = null;
  let latestIbkrJob: ImportJobResponse | null = null;
  let loadError: string | null = null;

  try {
    [portfolio, positions, latestB3Job, latestIbkrJob] = await Promise.all([
      fetchPortfolio(),
      fetchPositions(),
      fetchLatestImportJob(["b3", "manual_b3_export"]),
      fetchLatestImportJob(["ibkr"]),
    ]);
  } catch (error) {
    loadError = error instanceof Error ? error.message : "Failed to load portfolio data.";
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 px-4 py-8 md:gap-8 md:px-10 md:py-10">
      <section className="flex items-baseline justify-between gap-4">
        <h1 className="text-xl font-semibold leading-tight sm:text-2xl">Portfolio</h1>
        <p className="text-xs text-white/55 sm:text-sm">B3 and IBKR holdings</p>
      </section>

      <UploadPanel latestB3Job={latestB3Job} latestIbkrJob={latestIbkrJob} />
      {loadError ? (
        <section className="rounded-[2rem] border border-white/15 bg-white/10 p-5 text-sm text-white">
          {loadError} Start the Go backend and refresh this page.
        </section>
      ) : null}
      <SummaryCards portfolio={portfolio} />

      <section className="grid gap-8 lg:grid-cols-3 lg:items-start">
        <AllocationChart allocations={portfolio.allocations} />
        <div className="lg:col-span-2">
          <PositionsTable positions={positions} />
        </div>
      </section>
    </main>
  );
}
