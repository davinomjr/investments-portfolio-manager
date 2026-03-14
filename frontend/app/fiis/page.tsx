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
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-8 px-6 py-10 md:px-10">
      <section className="grid gap-8 md:grid-cols-[1.2fr_0.8fr] md:items-end">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Real Estate Funds</p>
          <h1 className="mt-4 max-w-3xl text-5xl font-semibold leading-tight">
            FII metrics for your held real estate fund positions.
          </h1>
          <p className="mt-4 max-w-2xl text-lg text-white/65">
            Dividend yield, P/VP, FFO yield, and dividend per unit sourced from Fundamentus for each FII in your portfolio.
          </p>
        </div>
        <div className="rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)]">
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
