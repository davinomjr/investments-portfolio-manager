"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import type { Position } from "@/lib/api";
import { formatHoldingLabel, getAssetStyle } from "@/lib/asset-style";
import { useVisibility } from "@/components/visibility-context";

function formatCurrency(value: number, currency = "BRL") {
  const locale = currency === "BRL" ? "pt-BR" : "en-US";
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
    maximumFractionDigits: 2,
  }).format(value);
}

type SortKey = "ticker" | "type" | "qty" | "price" | "value";
type SortDir = "asc" | "desc";

const NUMERIC_KEYS: ReadonlySet<SortKey> = new Set(["qty", "price", "value"]);

function positionSortValue(position: Position, key: SortKey): number | string {
  switch (key) {
    case "ticker":
      return formatHoldingLabel(position.ticker, position.company_name, position.asset_type).toLowerCase();
    case "type":
      return getAssetStyle(position.asset_type).label.toLowerCase();
    case "qty":
      return position.quantity;
    case "price":
      return position.avg_price;
    case "value":
      return position.market_value_brl;
  }
}

function SortHeader({
  label,
  sortKey,
  activeKey,
  activeDir,
  onSort,
  className,
}: {
  label: string;
  sortKey: SortKey;
  activeKey: SortKey;
  activeDir: SortDir;
  onSort: (key: SortKey) => void;
  className?: string;
}) {
  const isActive = activeKey === sortKey;
  const arrow = isActive ? (activeDir === "asc" ? "↑" : "↓") : "";
  return (
    <th className={className} aria-sort={isActive ? (activeDir === "asc" ? "ascending" : "descending") : "none"}>
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className={`flex items-center gap-1 uppercase tracking-wider transition-colors hover:text-white/90 ${isActive ? "text-white" : ""}`}
      >
        <span>{label}</span>
        <span className="w-3 text-xs">{arrow}</span>
      </button>
    </th>
  );
}

export function PositionsTable({ positions }: { positions: Position[] }) {
  const { visible } = useVisibility();
  const [sortKey, setSortKey] = useState<SortKey>("value");
  const [sortDir, setSortDir] = useState<SortDir>("desc");

  const handleSort = (key: SortKey) => {
    if (key === sortKey) {
      setSortDir((dir) => (dir === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir(NUMERIC_KEYS.has(key) ? "desc" : "asc");
    }
  };

  const sortedPositions = useMemo(() => {
    const copy = [...positions];
    copy.sort((a, b) => {
      const av = positionSortValue(a, sortKey);
      const bv = positionSortValue(b, sortKey);
      if (av < bv) return sortDir === "asc" ? -1 : 1;
      if (av > bv) return sortDir === "asc" ? 1 : -1;
      return 0;
    });
    return copy;
  }, [positions, sortKey, sortDir]);

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
                <SortHeader label="Ticker" sortKey="ticker" activeKey={sortKey} activeDir={sortDir} onSort={handleSort} className="pb-2 pr-4" />
                <SortHeader label="Type" sortKey="type" activeKey={sortKey} activeDir={sortDir} onSort={handleSort} className="hidden pb-2 pr-4 sm:table-cell" />
                <SortHeader label="Qty" sortKey="qty" activeKey={sortKey} activeDir={sortDir} onSort={handleSort} className="pb-2 pr-4" />
                <SortHeader label="Avg/Close" sortKey="price" activeKey={sortKey} activeDir={sortDir} onSort={handleSort} className="hidden pb-2 pr-4 sm:table-cell" />
                <SortHeader label="Value" sortKey="value" activeKey={sortKey} activeDir={sortDir} onSort={handleSort} className="pb-2 pr-4" />
              </tr>
            </thead>
            <tbody>
              {sortedPositions.map((position) => (
                <tr key={position.ticker} className="rounded-2xl border border-white/10 bg-[#272a36]">
                  <td className="rounded-l-2xl px-3 py-2.5 font-semibold md:px-4 md:py-3">
                    <Link
                      href={`/results#${position.ticker}`}
                      className="transition-colors hover:text-white/80 hover:underline"
                    >
                      {formatHoldingLabel(position.ticker, position.company_name, position.asset_type)}
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
