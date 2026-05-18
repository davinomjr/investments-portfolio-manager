"use client";

import type { Portfolio } from "@/lib/api";
import { formatHoldingLabel, getAssetStyle } from "@/lib/asset-style";
import { useVisibility } from "@/components/visibility-context";

function formatCurrency(value: number) {
  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency: "BRL",
    maximumFractionDigits: 2,
  }).format(value);
}

function formatSignedCurrency(value: number) {
  const sign = value >= 0 ? "+" : "−";
  return `${sign}${formatCurrency(Math.abs(value))}`;
}

function formatPct(value: number) {
  const sign = value >= 0 ? "+" : "−";
  return `${sign}${Math.abs(value).toFixed(2)}%`;
}

export function SummaryCards({ portfolio }: { portfolio: Portfolio }) {
  const { visible } = useVisibility();
  const largestAllocation = portfolio.allocations?.[0] ?? null;
  const largestStyle = largestAllocation ? getAssetStyle(largestAllocation.asset_type) : null;
  const pnlPositive = portfolio.pnl_brl >= 0;
  const pnlColor = pnlPositive ? "text-emerald-300" : "text-rose-300";
  const statusLabel =
    portfolio.quotes_status === "live"
      ? "Live quotes"
      : portfolio.quotes_status === "partial"
      ? "Partial quotes"
      : "Quotes unavailable";

  return (
    <section className="grid gap-3 sm:grid-cols-3 md:gap-4">
      <article className="rounded-[2rem] border border-white/20 bg-white/15 p-4 shadow-[0_20px_60px_rgba(0,0,0,0.4)] md:p-6">
        <div className="flex items-baseline justify-between gap-2">
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Total Value</p>
          <span className="text-[10px] uppercase tracking-[0.25em] text-white/40">{statusLabel}</span>
        </div>
        <p className="mt-3 text-3xl font-semibold md:mt-4 md:text-4xl">{visible ? formatCurrency(portfolio.market_value_brl) : "**"}</p>
        {visible ? (
          <p className="mt-2 text-xs text-white/55">
            Cost basis {formatCurrency(portfolio.estimated_cost_basis)}
            {" · "}
            <span className={`font-semibold ${pnlColor}`}>
              {formatSignedCurrency(portfolio.pnl_brl)}
              {portfolio.pnl_pct !== null ? ` (${formatPct(portfolio.pnl_pct)})` : ""}
            </span>
          </p>
        ) : (
          <p className="mt-2 text-xs text-white/55">**</p>
        )}
      </article>
      <article className="rounded-[2rem] border border-white/15 bg-[#222530] p-4 md:p-6">
        <p className="text-xs uppercase tracking-[0.3em] text-white/55">Positions</p>
        <p className="mt-3 text-3xl font-semibold md:mt-4 md:text-4xl">{portfolio.total_positions}</p>
      </article>
      <article className="rounded-[2rem] border border-white/15 bg-[#222530] p-4 md:p-6">
        <p className="text-xs uppercase tracking-[0.3em] text-white/55">Largest Weight</p>
        {largestAllocation ? (
          <>
            <p className="mt-3 text-2xl font-semibold md:mt-4">{visible ? `${formatHoldingLabel(largestAllocation.ticker, largestAllocation.company_name, largestAllocation.asset_type)} ${(largestAllocation.weight * 100).toFixed(1)}%` : "**"}</p>
            <span
              className="mt-3 inline-flex rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
              style={{
                backgroundColor: largestStyle?.soft,
                borderColor: largestStyle?.border,
                color: largestStyle?.text,
              }}
            >
              {largestStyle?.label}
            </span>
          </>
        ) : (
          <p className="mt-4 text-2xl font-semibold">No data</p>
        )}
      </article>
    </section>
  );
}
