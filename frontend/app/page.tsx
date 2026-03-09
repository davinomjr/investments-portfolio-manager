import { AllocationChart } from "@/components/allocation-chart";
import { PositionsTable } from "@/components/positions-table";
import { SummaryCards } from "@/components/summary-cards";
import { UploadPanel } from "@/components/upload-panel";
import {
  fetchPortfolio,
  fetchPositions,
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
  let loadError: string | null = null;

  try {
    [portfolio, positions] = await Promise.all([fetchPortfolio(), fetchPositions()]);
  } catch (error) {
    loadError = error instanceof Error ? error.message : "Failed to load portfolio data.";
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-8 px-6 py-10 md:px-10">
      <section className="grid gap-8 md:grid-cols-[1.2fr_0.8fr] md:items-end">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Portfolio</p>
          <h1 className="mt-4 max-w-3xl text-5xl font-semibold leading-tight">
            Visualize imported B3 positions before you start building scenarios.
          </h1>
          <p className="mt-4 max-w-2xl text-lg text-white/65">
            Upload a manual B3 export, inspect normalized holdings, and review allocation weight before running your
            Monte Carlo scenarios.
          </p>
        </div>
        <div className="rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)]">
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Current Scope</p>
          <p className="mt-3 text-lg">
            Manual import, normalized positions, allocation visualization, and Monte Carlo simulation are live.
          </p>
        </div>
      </section>

      <UploadPanel />
      {loadError ? (
        <section className="rounded-[2rem] border border-white/15 bg-white/10 p-5 text-sm text-white">
          {loadError} Start the Go backend and refresh this page.
        </section>
      ) : null}
      <SummaryCards portfolio={portfolio} />

      <section className="grid gap-8 lg:grid-cols-[0.9fr_1.1fr]">
        <AllocationChart allocations={portfolio.allocations} />
        <PositionsTable positions={positions} />
      </section>
    </main>
  );
}
