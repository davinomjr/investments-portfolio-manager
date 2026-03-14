"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import type { Position } from "@/lib/api";
import { setPositionsVisibility } from "@/lib/api";
import { getAssetStyle } from "@/lib/asset-style";

function formatCurrency(value: number) {
  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency: "BRL",
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
  // Derive initial state from DB: visible if at least one position is not hidden
  const [visible, setVisible] = useState(() => positions.length === 0 || positions.some((p) => !p.hidden));
  const [pending, startTransition] = useTransition();

  function handleToggle() {
    const next = !visible;
    startTransition(async () => {
      await setPositionsVisibility(next);
      setVisible(next);
    });
  }

  return (
    <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Positions</p>
          <h2 className="mt-2 text-2xl font-semibold">Imported holdings</h2>
        </div>
        <button
          onClick={handleToggle}
          disabled={pending}
          title={visible ? "Hide positions" : "Show positions"}
          aria-pressed={visible}
          className="flex h-7 w-12 items-center rounded-full border border-white/20 bg-white/10 px-1 transition-colors hover:border-white/40 disabled:opacity-50"
        >
          <span
            className={`h-5 w-5 rounded-full transition-transform duration-200${visible ? " translate-x-5 bg-pine" : " translate-x-0 bg-white/30"}`}
          />
        </button>
      </div>
      {visible && (
        <div className="overflow-x-auto">
          <table className="min-w-full border-separate border-spacing-y-2 text-left text-sm">
            <thead className="text-white/50">
              <tr>
                <th className="pb-2 pr-4">Ticker</th>
                <th className="pb-2 pr-4">Type</th>
                <th className="pb-2 pr-4">Quantity</th>
                <th className="pb-2 pr-4">Avg/Close</th>
                <th className="pb-2 pr-4">Broker</th>
                <th className="pb-2">Updated</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((position) => (
                <tr key={position.ticker} className="rounded-2xl border border-white/10 bg-[#272a36]">
                  <td className="rounded-l-2xl px-4 py-3 font-semibold">
                    <Link
                      href={`/results#${position.ticker}`}
                      className="hover:underline hover:text-white/80 transition-colors"
                    >
                      {position.ticker}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
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
                  <td className="px-4 py-3">{position.quantity}</td>
                  <td className="px-4 py-3">{formatCurrency(position.avg_price)}</td>
                  <td className="px-4 py-3">{position.broker ?? "-"}</td>
                  <td className="rounded-r-2xl px-4 py-3">{formatDate(position.last_updated)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
