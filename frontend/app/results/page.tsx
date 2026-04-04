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
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 px-4 py-8 md:gap-8 md:px-10 md:py-10">
      <section className="grid gap-6 md:grid-cols-[1.2fr_0.8fr] md:items-end md:gap-8">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-white/55">Results</p>
          <h1 className="mt-3 max-w-3xl text-3xl font-semibold leading-tight sm:text-4xl md:mt-4 md:text-5xl">
            Read the latest quarterly filing summary without loading the full portfolio screen.
          </h1>
          <p className="mt-3 max-w-2xl text-base text-white/65 md:mt-4 md:text-lg">
            This page combines official CVM quarterly data with sourced public-market sentiment for each imported stock position.
          </p>
        </div>
        <div className="hidden rounded-[2rem] border border-white/20 bg-white/10 p-6 shadow-[0_24px_80px_rgba(0,0,0,0.4)] md:block">
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
