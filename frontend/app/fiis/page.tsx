export const dynamic = "force-dynamic";

import { FIIResults } from "@/components/fii-results";
import { fetchFIIResults, type FIIResultsResponse } from "@/lib/api";

const EMPTY_RESULTS: FIIResultsResponse = { items: [] };

export default async function FIIsPage() {
  let results: FIIResultsResponse = EMPTY_RESULTS;
  let loadError: string | null = null;

  try {
    results = await fetchFIIResults();
  } catch (error) {
    loadError = error instanceof Error ? error.message : "Failed to load FII results.";
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 px-4 py-8 md:gap-8 md:px-10 md:py-10">
      <section className="grid gap-6 md:grid-cols-[1.2fr_0.8fr] md:items-end md:gap-8">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Real Estate Funds</p>
          <h1 className="mt-3 max-w-3xl text-3xl font-semibold leading-tight sm:text-4xl md:mt-4 md:text-5xl">
            FII metrics for your held real estate fund positions.
          </h1>
          <p className="mt-3 max-w-2xl text-base text-white/65 md:mt-4 md:text-lg">
            Dividend yield, P/VP, liquidity, and dividend per unit sourced from Funds Explorer, with Fundamentus as fallback.
          </p>
        </div>
        <div className="hidden rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)] md:block">
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">P/VP Guide</p>
          <p className="mt-3 text-lg">
            Green below 1.0 — trading at a discount. Yellow 1.0–1.2 — fair value. Red above 1.2 — premium territory.
          </p>
        </div>
      </section>

      {loadError ? (
        <section className="rounded-[2rem] border border-white/15 bg-white/10 p-5 text-sm text-white">
          {loadError} Start the Go backend and refresh this page.
        </section>
      ) : null}

      <FIIResults results={results} />
    </main>
  );
}
