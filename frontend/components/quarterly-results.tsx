import type { QuarterlyResultsResponse } from "@/lib/api";
import { getAssetStyle } from "@/lib/asset-style";

function formatCurrency(value: number | null) {
  if (value === null) return null;
  const absolute = Math.abs(value);
  const sign = value < 0 ? "-" : "";
  const units = [
    { value: 1_000_000_000_000_000, suffix: "Q" },
    { value: 1_000_000_000_000, suffix: "T" },
    { value: 1_000_000_000, suffix: "B" },
    { value: 1_000_000, suffix: "M" },
    { value: 1_000, suffix: "K" },
  ];

  for (const unit of units) {
    if (absolute >= unit.value) {
      return `${sign}R$ ${(absolute / unit.value).toFixed(2)}${unit.suffix}`;
    }
  }

  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency: "BRL",
    maximumFractionDigits: 0,
  }).format(value);
}

export function QuarterlyResults({ results }: { results: QuarterlyResultsResponse }) {
  return (
    <section className="rounded-[2rem] border border-black bg-white p-6">
      <div className="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-black/55">Latest Quarter</p>
          <h2 className="mt-2 text-2xl font-semibold">Results summary for held stocks</h2>
        </div>
        <p className="max-w-xl text-sm leading-6 text-black/60">
          Official CVM filing snapshot for imported equity positions, condensed into a faster read than raw statements.
        </p>
      </div>

      {results.message ? (
        <div className="mt-6 rounded-[1.5rem] border border-black bg-[#f5f5f5] px-4 py-3 text-sm text-black/70">
          {results.message}
        </div>
      ) : null}

      <div className="mt-6 grid gap-4 xl:grid-cols-2">
        {results.items.map((item) => {
          const style = getAssetStyle(item.asset_type);
          const verdict = getQuarterVerdict(item);
          return (
            <article key={item.ticker} className="rounded-[1.75rem] border border-black bg-white p-5 shadow-[0_14px_40px_rgba(0,0,0,0.05)]">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="flex items-center gap-3">
                    <h3 className="text-2xl font-semibold">{item.ticker}</h3>
                    <span
                      className="inline-flex rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
                      style={{
                        backgroundColor: style.soft,
                        borderColor: style.border,
                        color: style.text,
                      }}
                    >
                      {style.label}
                    </span>
                  </div>
                  <p className="mt-2 text-sm text-black/60">{item.company_name ?? "Company name unavailable"}</p>
                </div>
                <div className="text-right text-sm text-black/55">
                  <p className="uppercase tracking-[0.18em]">Status</p>
                  <p className="font-semibold text-black">{item.status.replaceAll("_", " ")}</p>
                  {item.report_date ? <p className="mt-1">{item.report_date}</p> : null}
                </div>
              </div>

              {item.status === "ok" ? (
                <>
                  <div
                    className="mt-5 rounded-[1.25rem] border px-4 py-4"
                    style={{
                      borderColor: verdict.border,
                      backgroundColor: verdict.soft,
                    }}
                  >
                    <p className="text-xs font-semibold uppercase tracking-[0.18em]" style={{ color: verdict.text }}>
                      {verdict.label}
                    </p>
                    <p className="mt-2 text-sm leading-6 text-black/80">{verdict.summary}</p>
                  </div>

                  <div className="mt-4 grid gap-3 sm:grid-cols-2">
                    <Metric label="Revenue" value={formatCurrency(item.revenue)} accent={style.color} />
                    <Metric label="Net Income" value={formatCurrency(item.net_income)} accent={style.color} />
                    <Metric
                      label="Net Margin"
                      value={item.net_margin !== null ? `${item.net_margin.toFixed(1)}%` : null}
                      accent={style.color}
                    />
                    <NoteCard text="EBITDA is omitted here because it is not a standardized line in CVM quarterly DRE data." />
                  </div>
                  <ul className="mt-4 flex flex-wrap gap-2">
                    {item.highlights.map((highlight) => (
                      <li
                        key={highlight}
                        className="rounded-full border border-black px-3 py-1 text-[11px] uppercase tracking-[0.16em] text-black/70"
                      >
                        {highlight}
                      </li>
                    ))}
                  </ul>
                </>
              ) : (
                <div className="mt-5 rounded-[1.25rem] border border-black bg-[#f7f7f7] px-4 py-4">
                  <p className="text-sm leading-6 text-black/65">{item.message ?? "Quarterly data unavailable for this ticker."}</p>
                </div>
              )}
            </article>
          );
        })}
      </div>
    </section>
  );
}

function Metric({ label, value, accent }: { label: string; value: string | null; accent: string }) {
  return (
    <div className="rounded-[1.25rem] border border-black bg-[#fafafa] p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-black/50">{label}</p>
      <p
        className="mt-2 whitespace-nowrap text-[clamp(1rem,1.8vw,2.2rem)] font-semibold leading-tight"
        style={{ color: accent }}
        title={value ?? "N/A"}
      >
        {value ?? "N/A"}
      </p>
    </div>
  );
}

function NoteCard({ text }: { text: string }) {
  return (
    <div className="rounded-[1.25rem] border border-black bg-[#f3f3f3] p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-black/50">Note</p>
      <p className="mt-2 text-sm leading-6 text-black/70">{text}</p>
    </div>
  );
}

function getQuarterVerdict(item: QuarterlyResultsResponse["items"][number]) {
  if (item.net_income === null && item.revenue === null) {
    return {
      label: "Insufficient Data",
      summary: "The latest filing matched the company, but the key quarter metrics could not be extracted cleanly.",
      soft: "#f5f5f5",
      border: "#d4d4d4",
      text: "#404040",
    };
  }

  if ((item.net_income ?? 0) < 0) {
    return {
      label: "Weak Quarter",
      summary: "The company reported a loss in the latest quarter, which is a clear negative signal.",
      soft: "#fee2e2",
      border: "#fca5a5",
      text: "#b91c1c",
    };
  }

  if ((item.net_income ?? 0) > 0 && (item.net_margin ?? 0) >= 10) {
    return {
      label: "Good Quarter",
      summary: "The company stayed profitable and delivered a solid net margin in the latest quarter.",
      soft: "#dcfce7",
      border: "#86efac",
      text: "#15803d",
    };
  }

  if ((item.net_income ?? 0) > 0) {
    return {
      label: "Mixed Quarter",
      summary: "The company remained profitable, but margins look modest and deserve a closer read.",
      soft: "#fef3c7",
      border: "#fcd34d",
      text: "#b45309",
    };
  }

  return {
    label: "Neutral Quarter",
    summary: "The quarter does not show an obvious profit signal yet, so it needs more context from the full release.",
    soft: "#e5e7eb",
    border: "#cbd5e1",
    text: "#334155",
  };
}
