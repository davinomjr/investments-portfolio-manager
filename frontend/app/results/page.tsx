export const dynamic = "force-dynamic";

import { QuarterlyResults } from "@/components/quarterly-results";
import { fetchQuarterlyResults, type QuarterlyResultsResponse } from "@/lib/api";

const EMPTY_RESULTS: QuarterlyResultsResponse = {
  provider: "cvm_itr",
  configured: false,
  message: "Quarterly results are not loaded yet.",
  items: [],
};

export default async function ResultsPage() {
  let results: QuarterlyResultsResponse = EMPTY_RESULTS;
  let loadError: string | null = null;

  try {
    results = await fetchQuarterlyResults();
  } catch (error) {
    loadError = error instanceof Error ? error.message : "Failed to load quarterly results.";
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-8 px-6 py-10 md:px-10">
      <section className="grid gap-8 md:grid-cols-[1.2fr_0.8fr] md:items-end">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Results</p>
          <h1 className="mt-4 max-w-3xl text-5xl font-semibold leading-tight">
            Read the latest quarterly filing summary without loading the full portfolio screen.
          </h1>
          <p className="mt-4 max-w-2xl text-lg text-white/65">
            This page combines official CVM quarterly data with sourced public-market sentiment for each imported stock position.
          </p>
        </div>
        <div className="rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)]">
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Read Mode</p>
          <p className="mt-3 text-lg">
            Portfolio import stays on its own screen. This one is optimized for reviewing quarter updates only.
          </p>
        </div>
      </section>

      {loadError ? (
        <section className="rounded-[2rem] border border-white/15 bg-white/10 p-5 text-sm text-white">
          {loadError} Start the Go backend and refresh this page.
        </section>
      ) : null}

      <QuarterlyResults results={results} />
    </main>
  );
}
