import type { MonteCarloResponse } from "@/lib/api";

function formatCurrency(value: number) {
  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency: "BRL",
    maximumFractionDigits: 2,
  }).format(value);
}

function formatPercent(value: number) {
  return `${(value * 100).toFixed(1)}%`;
}

export function MonteCarloPanel({ simulation }: { simulation: MonteCarloResponse }) {
  const lastPoint = simulation.timeline[simulation.timeline.length - 1];

  return (
    <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-6">
      <div className="flex flex-wrap items-start justify-between gap-6">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Monte Carlo</p>
          <h2 className="mt-2 text-2xl font-semibold">Portfolio projection curve</h2>
          <p className="mt-2 max-w-2xl text-sm text-white/65">{simulation.message}</p>
        </div>
        <div className="rounded-2xl border border-white/20 bg-white/10 px-5 py-4">
          <p className="text-xs uppercase tracking-[0.25em] text-white/60">Initial Value</p>
          <p className="mt-2 text-2xl font-semibold">{formatCurrency(simulation.initial_value)}</p>
        </div>
      </div>

      <div className="mt-6 grid gap-4 md:grid-cols-3">
        <article className="rounded-2xl border border-white/10 bg-[#272a36] p-4">
          <p className="text-xs uppercase tracking-[0.25em] text-white/55">Years</p>
          <p className="mt-2 text-2xl font-semibold">{simulation.params.years}</p>
        </article>
        <article className="rounded-2xl border border-white/10 bg-[#272a36] p-4">
          <p className="text-xs uppercase tracking-[0.25em] text-white/55">Simulations</p>
          <p className="mt-2 text-2xl font-semibold">{simulation.params.simulations}</p>
        </article>
        <article className="rounded-2xl border border-white/10 bg-[#272a36] p-4">
          <p className="text-xs uppercase tracking-[0.25em] text-white/55">Expected Return / Volatility</p>
          <p className="mt-2 text-2xl font-semibold">
            {formatPercent(simulation.params.expected_return)} / {formatPercent(simulation.params.volatility)}
          </p>
        </article>
      </div>

      {lastPoint ? (
        <div className="mt-6 grid gap-4 md:grid-cols-4">
          <article className="rounded-2xl border border-white/10 bg-[#272a36] p-4">
            <p className="text-xs uppercase tracking-[0.25em] text-white/55">Median (P50)</p>
            <p className="mt-2 text-lg font-semibold">{formatCurrency(lastPoint.p50)}</p>
          </article>
          <article className="rounded-2xl border border-white/10 bg-[#272a36] p-4">
            <p className="text-xs uppercase tracking-[0.25em] text-white/55">Conservative (P10)</p>
            <p className="mt-2 text-lg font-semibold">{formatCurrency(lastPoint.p10)}</p>
          </article>
          <article className="rounded-2xl border border-white/10 bg-[#272a36] p-4">
            <p className="text-xs uppercase tracking-[0.25em] text-white/55">Aggressive (P90)</p>
            <p className="mt-2 text-lg font-semibold">{formatCurrency(lastPoint.p90)}</p>
          </article>
          <article className="rounded-2xl border border-white/10 bg-[#272a36] p-4">
            <p className="text-xs uppercase tracking-[0.25em] text-white/55">Prob. positive</p>
            <p className="mt-2 text-lg font-semibold">{formatPercent(lastPoint.prob_positive)}</p>
          </article>
        </div>
      ) : (
        <p className="mt-6 rounded-2xl border border-white/10 bg-[#272a36] p-4 text-sm text-white/70">No portfolio positions found yet.</p>
      )}

      {simulation.timeline.length > 0 ? (
        <div className="mt-6 overflow-x-auto">
          <table className="min-w-full border-collapse text-left text-sm">
            <thead>
              <tr className="border-b border-white/15 text-white/60">
                <th className="px-3 py-2">Year</th>
                <th className="px-3 py-2">P10</th>
                <th className="px-3 py-2">P50</th>
                <th className="px-3 py-2">P90</th>
                <th className="px-3 py-2">Average</th>
              </tr>
            </thead>
            <tbody>
              {simulation.timeline.map((point) => (
                <tr key={point.year} className="border-b border-white/10">
                  <td className="px-3 py-2 font-medium">{point.year}</td>
                  <td className="px-3 py-2">{formatCurrency(point.p10)}</td>
                  <td className="px-3 py-2">{formatCurrency(point.p50)}</td>
                  <td className="px-3 py-2">{formatCurrency(point.p90)}</td>
                  <td className="px-3 py-2">{formatCurrency(point.average)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
