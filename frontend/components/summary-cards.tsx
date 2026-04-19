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

export function SummaryCards({ portfolio }: { portfolio: Portfolio }) {
  const { visible } = useVisibility();
  const largestAllocation = portfolio.allocations?.[0] ?? null;
  const largestStyle = largestAllocation ? getAssetStyle(largestAllocation.asset_type) : null;

  return (
    <section className="grid gap-3 sm:grid-cols-3 md:gap-4">
      <article className="rounded-[2rem] border border-white/20 bg-white/15 p-4 shadow-[0_20px_60px_rgba(0,0,0,0.4)] md:p-6">
        <p className="text-xs uppercase tracking-[0.3em] text-white/55">Total Invested</p>
        <p className="mt-3 text-3xl font-semibold md:mt-4 md:text-4xl">{visible ? formatCurrency(portfolio.estimated_cost_basis) : "**"}</p>
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
