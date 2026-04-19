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
  allocations: [],
};

export default async function HomePage() {
  let portfolio: Portfolio = EMPTY_PORTFOLIO;
  let positions: Position[] = [];
  let latestJob: ImportJobResponse | null = null;
  let loadError: string | null = null;

  try {
    [portfolio, positions, latestJob] = await Promise.all([
      fetchPortfolio(),
      fetchPositions(),
      fetchLatestImportJob(),
    ]);
  } catch (error) {
    loadError = error instanceof Error ? error.message : "Failed to load portfolio data.";
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 px-4 py-8 md:gap-8 md:px-10 md:py-10">
      <section className="grid gap-6 md:grid-cols-[1.2fr_0.8fr] md:items-end md:gap-8">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Portfolio</p>
          <h1 className="mt-3 max-w-3xl text-3xl font-semibold leading-tight sm:text-4xl md:mt-4 md:text-5xl">
            Track B3 and IBKR holdings in one place.
          </h1>
          <p className="mt-3 max-w-2xl text-base text-white/65 md:mt-4 md:text-lg">
            Import positions, review allocation weight, and stress-test your portfolio with Monte Carlo.
          </p>
        </div>
        <div className="hidden rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)] md:block">
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Current Scope</p>
          <p className="mt-3 text-lg">
            Manual import, normalized positions, allocation visualization, and Monte Carlo simulation are live.
          </p>
        </div>
      </section>

      <UploadPanel latestJob={latestJob} />
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
