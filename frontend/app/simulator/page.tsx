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
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 px-4 py-8 md:gap-8 md:px-10 md:py-10">
      <section className="grid gap-6 md:grid-cols-[1.2fr_0.8fr] md:items-end md:gap-8">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Simulator</p>
          <h1 className="mt-3 max-w-3xl text-3xl font-semibold leading-tight sm:text-4xl md:mt-4 md:text-5xl">
            Explore Monte Carlo outcomes based on your total invested.
          </h1>
          <p className="mt-3 max-w-2xl text-base text-white/65 md:mt-4 md:text-lg">
            This projection uses annualized random return paths and shows percentile bands so you can reason about
            range of outcomes instead of a single point estimate.
          </p>
        </div>
        <div className="hidden rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)] md:block">
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
