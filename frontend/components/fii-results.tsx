import type { FIIResultsResponse } from "@/lib/api";
import { getAssetStyle } from "@/lib/asset-style";

function formatVolume(value: number | null) {
  if (value === null) return null;
  if (value >= 1_000_000) return `R$ ${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `R$ ${(value / 1_000).toFixed(0)}K`;
  return `R$ ${value.toFixed(0)}`;
}

function getPVPStyle(pvp: number | null): { soft: string; border: string; text: string; dark: boolean } {
  if (pvp === null) return { soft: "#272a36", border: "rgba(255,255,255,0.1)", text: "#fff", dark: true };
  if (pvp < 1.0) return { soft: "#dcfce7", border: "#86efac", text: "#15803d", dark: false };
  if (pvp <= 1.2) return { soft: "#fef3c7", border: "#fcd34d", text: "#b45309", dark: false };
  return { soft: "#fee2e2", border: "#fca5a5", text: "#b91c1c", dark: false };
}

function getVacancyStyle(vacancy: number | null): { soft: string; border: string; text: string; dark: boolean } {
  if (vacancy === null) return { soft: "#272a36", border: "rgba(255,255,255,0.1)", text: "#fff", dark: true };
  if (vacancy < 10) return { soft: "#dcfce7", border: "#86efac", text: "#15803d", dark: false };
  if (vacancy <= 20) return { soft: "#fef3c7", border: "#fcd34d", text: "#b45309", dark: false };
  return { soft: "#fee2e2", border: "#fca5a5", text: "#b91c1c", dark: false };
}

export function FIIResults({ results }: { results: FIIResultsResponse }) {
  if (results.items.length === 0) {
    return (
      <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-6">
        <p className="text-sm text-white/60">No FII positions found. Import your B3 portfolio first.</p>
      </section>
    );
  }

  return (
    <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-6">
      <div className="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Real Estate Funds</p>
          <h2 className="mt-2 text-2xl font-semibold">FII metrics for held positions</h2>
        </div>
        <p className="max-w-xl text-sm leading-6 text-white/60">
          Key income and valuation metrics from Status Invest, with Fundamentus used when Status Invest is unavailable.
        </p>
      </div>

      <div className="mt-6 grid gap-4 xl:grid-cols-2">
        {results.items.map((item) => {
          const style = getAssetStyle(item.asset_type);
          const pvpStyle = getPVPStyle(item.pvp);
          const vacancyStyle = getVacancyStyle(item.vacancy_rate);
          return (
            <article key={item.ticker} className="rounded-[1.75rem] border border-white/10 bg-[#272a36] p-5 shadow-[0_14px_40px_rgba(0,0,0,0.3)]">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="flex items-center gap-3">
                    <h3 className="text-2xl font-semibold">{item.ticker}</h3>
                    <span
                      className="inline-flex rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
                      style={{ backgroundColor: style.soft, borderColor: style.border, color: style.text }}
                    >
                      {style.label}
                    </span>
                  </div>
                  <p className="mt-2 text-sm text-white/60">{item.company_name ?? "Company name unavailable"}</p>
                </div>
                <div className="text-right text-sm text-white/55">
                  <p className="uppercase tracking-[0.18em]">Status</p>
                  <p className="font-semibold text-white">{item.status.replaceAll("_", " ")}</p>
                </div>
              </div>

              {item.status === "ok" ? (
                <>
                  {/* Income metrics */}
                  <div className="mt-4 grid gap-3 sm:grid-cols-2">
                    <Metric label="Div. Yield (12M)" value={item.dividend_yield !== null ? `${item.dividend_yield.toFixed(2)}%` : null} />
                    <MetricWithTooltip
                      label="FFO Yield"
                      value={item.ffo_yield !== null ? `${item.ffo_yield.toFixed(2)}%` : null}
                      tooltip="Net income adjusted for non-cash items (property revaluations, asset sales). FFO Yield > Div. Yield means the fund retains a cash buffer beyond what it distributes."
                    />
                    <Metric label="Dividend/Unit" value={item.dividend_per_unit !== null ? `R$ ${item.dividend_per_unit.toFixed(2)}` : null} />
                    <Metric label="Cap Rate" value={item.cap_rate !== null ? `${item.cap_rate.toFixed(1)}%` : null} />
                  </div>

                  {/* Valuation + health row */}
                  <div className="mt-3 grid gap-3 sm:grid-cols-3">
                    <ColorMetric label="P/VP" value={item.pvp !== null ? item.pvp.toFixed(2) : null} s={pvpStyle} />
                    <ColorMetric label="Avg. Vacancy" value={item.vacancy_rate !== null ? `${item.vacancy_rate.toFixed(1)}%` : null} s={vacancyStyle} />
                    <Metric label="Avg. Vol. (2m)" value={formatVolume(item.avg_daily_volume)} />
                  </div>
                </>
              ) : (
                <div className="mt-5 rounded-[1.25rem] border border-white/10 bg-[#272a36] px-4 py-4">
                  <p className="text-sm leading-6 text-white/65">{item.message ?? "Data unavailable for this FII."}</p>
                </div>
              )}
            </article>
          );
        })}
      </div>
    </section>
  );
}

function MetricWithTooltip({ label, value, tooltip }: { label: string; value: string | null; tooltip: string }) {
  return (
    <div className="group relative rounded-[1.25rem] border border-white/10 bg-[#272a36] p-4">
      <div className="flex items-center gap-1.5">
        <p className="text-xs uppercase tracking-[0.18em] text-white/50">{label}</p>
        <span className="flex h-3.5 w-3.5 cursor-help items-center justify-center rounded-full border border-white/20 text-[9px] font-bold text-white/35 transition group-hover:border-white/40 group-hover:text-white/60">
          ?
        </span>
      </div>
      <p className="mt-2 whitespace-nowrap text-[clamp(1rem,1.8vw,2.2rem)] font-semibold leading-tight text-white" title={value ?? "N/A"}>
        {value ?? "N/A"}
      </p>
      <div className="pointer-events-none absolute bottom-full left-0 z-10 mb-2 w-64 rounded-2xl border border-white/15 bg-[#1a1d25] p-3 text-xs leading-5 text-white/70 opacity-0 shadow-[0_8px_32px_rgba(0,0,0,0.5)] transition-opacity group-hover:opacity-100">
        {tooltip}
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string | null }) {
  return (
    <div className="rounded-[1.25rem] border border-white/10 bg-[#272a36] p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-white/50">{label}</p>
      <p className="mt-2 whitespace-nowrap text-[clamp(1rem,1.8vw,2.2rem)] font-semibold leading-tight text-white" title={value ?? "N/A"}>
        {value ?? "N/A"}
      </p>
    </div>
  );
}

function ColorMetric({
  label,
  value,
  s,
}: {
  label: string;
  value: string | null;
  s: { soft: string; border: string; text: string; dark: boolean };
}) {
  return (
    <div className="rounded-[1.25rem] border p-4" style={{ backgroundColor: s.soft, borderColor: s.border }}>
      <p className="text-xs uppercase tracking-[0.18em]" style={{ color: s.dark ? "rgba(255,255,255,0.5)" : s.text }}>
        {label}
      </p>
      <p
        className="mt-2 whitespace-nowrap text-[clamp(1rem,1.8vw,2.2rem)] font-semibold leading-tight"
        style={{ color: s.dark ? "#fff" : "#111827" }}
        title={value ?? "N/A"}
      >
        {value ?? "N/A"}
      </p>
    </div>
  );
}
