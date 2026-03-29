export const dynamic = "force-dynamic";

import { MonteCarloPanel } from "@/components/monte-carlo-panel";
import { fetchMonteCarloSimulation, type MonteCarloResponse } from "@/lib/api";

const EMPTY_SIMULATION: MonteCarloResponse = {
  initial_value: 0,
  params: {
    years: 10,
    simulations: 1000,
    expected_return: 0.1,
    volatility: 0.18,
  },
  timeline: [],
  message: "Monte Carlo simulator not loaded yet.",
};

export default async function SimulatorPage() {
  let simulation: MonteCarloResponse = EMPTY_SIMULATION;
  let loadError: string | null = null;

  try {
    simulation = await fetchMonteCarloSimulation();
  } catch (error) {
    loadError = error instanceof Error ? error.message : "Failed to load simulation.";
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-8 px-6 py-10 md:px-10">
      <section className="grid gap-8 md:grid-cols-[1.2fr_0.8fr] md:items-end">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Simulator</p>
          <h1 className="mt-4 max-w-3xl text-5xl font-semibold leading-tight">
            Explore Monte Carlo outcomes for your current portfolio cost basis.
          </h1>
          <p className="mt-4 max-w-2xl text-lg text-white/65">
            This projection uses annualized random return paths and shows percentile bands so you can reason about
            range of outcomes instead of a single point estimate.
          </p>
        </div>
        <div className="rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)]">
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Simulation Model</p>
          <p className="mt-3 text-lg">
            Geometric Brownian motion with configurable expected return, volatility, years, and simulation count.
          </p>
        </div>
      </section>

      {loadError ? (
        <section className="rounded-[2rem] border border-white/15 bg-white/10 p-5 text-sm text-white">
          {loadError} Start the Go backend and refresh this page.
        </section>
      ) : null}

      <MonteCarloPanel simulation={simulation} />
    </main>
  );
}
