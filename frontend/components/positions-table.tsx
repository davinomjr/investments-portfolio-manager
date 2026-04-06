"use client";

import Link from "next/link";
import type { Position } from "@/lib/api";
import { getAssetStyle } from "@/lib/asset-style";
import { useVisibility } from "@/components/visibility-context";

function formatCurrency(value: number, currency = "BRL") {
  const locale = currency === "BRL" ? "pt-BR" : "en-US";
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
    maximumFractionDigits: 2,
  }).format(value);
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("pt-BR", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(new Date(value));
}

export function PositionsTable({ positions }: { positions: Position[] }) {
  const { visible } = useVisibility();

  return (
    <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-4 md:p-6">
      <div className="mb-4 md:mb-6">
        <p className="text-xs uppercase tracking-[0.3em] text-white/55">Positions</p>
        <h2 className="mt-2 text-2xl font-semibold">Imported holdings</h2>
      </div>
      <div className="overflow-x-auto">
          <table className="min-w-full border-separate border-spacing-y-2 text-left text-sm">
            <thead className="text-white/50">
              <tr>
                <th className="pb-2 pr-4">Ticker</th>
                <th className="hidden pb-2 pr-4 sm:table-cell">Type</th>
                <th className="pb-2 pr-4">Qty</th>
                <th className="hidden pb-2 pr-4 sm:table-cell">Avg/Close</th>
                <th className="pb-2 pr-4">Value</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((position) => (
                <tr key={position.ticker} className="rounded-2xl border border-white/10 bg-[#272a36]">
                  <td className="rounded-l-2xl px-3 py-2.5 font-semibold md:px-4 md:py-3">
                    <Link
                      href={`/results#${position.ticker}`}
                      className="transition-colors hover:text-white/80 hover:underline"
                    >
                      {position.ticker}
                    </Link>
                  </td>
                  <td className="hidden px-3 py-2.5 sm:table-cell md:px-4 md:py-3">
                    <span
                      className="inline-flex rounded-full border px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
                      style={{
                        backgroundColor: getAssetStyle(position.asset_type).soft,
                        borderColor: getAssetStyle(position.asset_type).border,
                        color: getAssetStyle(position.asset_type).text,
                      }}
                    >
                      {getAssetStyle(position.asset_type).label}
                    </span>
                  </td>
                  <td className="px-3 py-2.5 md:px-4 md:py-3">{visible ? position.quantity : "**"}</td>
                  <td className="hidden px-3 py-2.5 sm:table-cell md:px-4 md:py-3">{visible ? formatCurrency(position.avg_price, position.currency) : "**"}</td>
                  <td className="rounded-r-2xl px-3 py-2.5 md:px-4 md:py-3">{visible ? formatCurrency(position.quantity * position.avg_price, position.currency) : "**"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
    </section>
  );
}
