"use client";

import { useMemo, useState } from "react";
import { Area, CartesianGrid, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

import type { PortfolioHistoryPoint } from "@/lib/api";
import { getAssetStyle } from "@/lib/asset-style";
import { useVisibility } from "@/components/visibility-context";

type View = "total" | "type";
type Range = "30" | "90" | "365" | "all";

const RANGES: { id: Range; label: string }[] = [
  { id: "30", label: "1M" },
  { id: "90", label: "3M" },
  { id: "365", label: "1Y" },
  { id: "all", label: "All" },
];

const VIEWS: { id: View; label: string }[] = [
  { id: "total", label: "Value vs invested" },
  { id: "type", label: "By type" },
];

const MARKET_COLOR = "#34d399";
const INVESTED_COLOR = "rgba(255,255,255,0.55)";

const brlFormatter = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  maximumFractionDigits: 0,
});

const compactFormatter = new Intl.NumberFormat("pt-BR", {
  notation: "compact",
  maximumFractionDigits: 1,
});

// Dates arrive as YYYY-MM-DD in Brazil local time; parse as a plain calendar
// date so the label doesn't shift a day in the viewer's timezone.
function formatDate(date: string, withYear = false) {
  const [y, m, d] = date.split("-").map(Number);
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    ...(withYear ? { year: "numeric" } : {}),
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y, m - 1, d)));
}

function cutoffDate(days: number) {
  const d = new Date();
  d.setDate(d.getDate() - (days - 1));
  return d.toISOString().slice(0, 10);
}

function ToggleGroup<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: { id: T; label: string }[];
  value: T;
  onChange: (value: T) => void;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="flex rounded-full border border-white/10 bg-black/20 p-1">
      {options.map((option) => {
        const active = option.id === value;
        return (
          <button
            key={option.id}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(option.id)}
            className={`rounded-full px-3 py-1.5 text-xs font-medium transition ${
              active ? "bg-white text-[#1a1d25]" : "text-white/65 hover:text-white"
            }`}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

export function HistoryChart({ points }: { points: PortfolioHistoryPoint[] }) {
  const { visible } = useVisibility();
  const [view, setView] = useState<View>("total");
  const [range, setRange] = useState<Range>("90");

  const filtered = useMemo(() => {
    if (range === "all") return points;
    const cutoff = cutoffDate(Number(range));
    return points.filter((p) => p.date >= cutoff);
  }, [points, range]);

  // Order asset types by latest value so the largest band sits at the bottom
  // of the stack and the legend reads biggest first.
  const assetTypes = useMemo(() => {
    const totals = new Map<string, number>();
    for (const p of filtered) {
      for (const [type, value] of Object.entries(p.by_asset_type)) {
        totals.set(type, value);
      }
    }
    return [...totals.entries()].sort((a, b) => b[1] - a[1]).map(([type]) => type);
  }, [filtered]);

  const data = useMemo(
    () =>
      filtered.map((p) => ({
        date: p.date,
        market: p.market_value_brl,
        invested: p.cost_basis_brl,
        ...Object.fromEntries(assetTypes.map((t) => [t, p.by_asset_type[t] ?? 0])),
      })),
    [filtered, assetTypes],
  );

  const first = filtered[0];
  const last = filtered[filtered.length - 1];
  const change = first && last ? last.market_value_brl - first.market_value_brl : 0;
  const gain = last ? last.market_value_brl - last.cost_basis_brl : 0;
  const gainColor = gain >= 0 ? "text-emerald-300" : "text-rose-300";
  const changeColor = change >= 0 ? "text-emerald-300" : "text-rose-300";
  // With a single snapshot there is no line to draw, so show the day's numbers
  // and a dot until the next sync adds a second point.
  const single = filtered.length === 1;
  const singleDot = (color: string) => (single ? { r: 4, fill: color, stroke: color } : false);
  const signed = (v: number) => `${v >= 0 ? "+" : "−"}${brlFormatter.format(Math.abs(v))}`;

  return (
    <section className="overflow-hidden rounded-[2rem] border border-white/15 bg-[#222530] p-4 md:p-6">
      <div className="mb-4 flex flex-col gap-3 md:mb-6 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">History</p>
          <h2 className="mt-2 text-2xl font-semibold">Portfolio growth</h2>
          {single ? (
            <p className="mt-2 text-xs text-white/55">
              {visible ? (
                <>
                  Market value <span className="font-semibold text-white">{brlFormatter.format(last.market_value_brl)}</span>
                  {" · "}
                  Invested <span className="font-semibold text-white">{brlFormatter.format(last.cost_basis_brl)}</span>
                  {" · "}
                  Gain <span className={`font-semibold ${gainColor}`}>{signed(gain)}</span>
                </>
              ) : (
                "**"
              )}
            </p>
          ) : last ? (
            <p className="mt-2 text-xs text-white/55">
              {visible ? (
                <>
                  <span className={`font-semibold ${changeColor}`}>{signed(change)}</span> since {formatDate(first.date)}
                  {" · "}
                  Gain over invested <span className={`font-semibold ${gainColor}`}>{signed(gain)}</span>
                </>
              ) : (
                "**"
              )}
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <ToggleGroup label="Chart view" options={VIEWS} value={view} onChange={setView} />
          <ToggleGroup label="Date range" options={RANGES} value={range} onChange={setRange} />
        </div>
      </div>

      {filtered.length === 0 ? (
        <div className="flex h-56 items-center justify-center rounded-2xl border border-dashed border-white/10 px-6 text-center text-sm text-white/55 sm:h-64">
          {points.length === 0
            ? "No history yet. A snapshot is saved after every sync. The chart fills in from your next one."
            : "No snapshots in this range. Try a longer one."}
        </div>
      ) : (
        <>
          <div className="h-56 sm:h-64 md:h-72">
            <ResponsiveContainer width="100%" height="100%">
              <ComposedChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
                <defs>
                  <linearGradient id="history-market-fill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={MARKET_COLOR} stopOpacity={0.3} />
                    <stop offset="100%" stopColor={MARKET_COLOR} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="rgba(255,255,255,0.06)" vertical={false} />
                <XAxis
                  dataKey="date"
                  tickFormatter={(d: string) => formatDate(d)}
                  tick={{ fill: "rgba(255,255,255,0.55)", fontSize: 11 }}
                  axisLine={false}
                  tickLine={false}
                  minTickGap={32}
                />
                <YAxis
                  tickFormatter={(v: number) => (visible ? compactFormatter.format(v) : "**")}
                  tick={{ fill: "rgba(255,255,255,0.55)", fontSize: 11 }}
                  axisLine={false}
                  tickLine={false}
                  width={48}
                  domain={view === "total" ? ["auto", "auto"] : [0, "auto"]}
                />
                <Tooltip
                  contentStyle={{ borderRadius: "16px", border: "1px solid rgba(255,255,255,0.15)", backgroundColor: "#272a36", color: "#fff" }}
                  itemStyle={{ color: "#fff" }}
                  labelStyle={{ color: "rgba(255,255,255,0.65)", marginBottom: 4 }}
                  labelFormatter={(d: string) => formatDate(d, true)}
                  formatter={(value: number, name: string) => [
                    visible ? brlFormatter.format(value) : "**",
                    name === "market" ? "Market value" : name === "invested" ? "Invested" : getAssetStyle(name).label,
                  ]}
                />
                {/* Recharts only finds series among direct children or arrays, not fragments. */}
                {view === "total"
                  ? [
                      <Area
                        key="market"
                        type="monotone"
                        dataKey="market"
                        stroke={MARKET_COLOR}
                        strokeWidth={2}
                        fill="url(#history-market-fill)"
                        dot={singleDot(MARKET_COLOR)}
                        activeDot={{ r: 4 }}
                      />,
                      <Line
                        key="invested"
                        type="monotone"
                        dataKey="invested"
                        stroke={INVESTED_COLOR}
                        strokeWidth={1.5}
                        strokeDasharray="5 4"
                        dot={singleDot(INVESTED_COLOR)}
                        activeDot={{ r: 3 }}
                      />,
                    ]
                  : assetTypes.map((type) => (
                      <Area
                        key={type}
                        type="monotone"
                        dataKey={type}
                        stackId="types"
                        stroke={getAssetStyle(type).color}
                        fill={getAssetStyle(type).color}
                        fillOpacity={0.35}
                        strokeWidth={1.5}
                        dot={singleDot(getAssetStyle(type).color)}
                      />
                    ))}
              </ComposedChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5 text-xs text-white/65">
            {single ? (
              <span className="w-full text-white/45 sm:order-last sm:ml-auto sm:w-auto">
                First snapshot on {formatDate(last.date, true)}. The line starts after the next daily sync.
              </span>
            ) : null}
            {view === "total" ? (
              <>
                <LegendItem color={MARKET_COLOR} label="Market value" />
                <LegendItem color={INVESTED_COLOR} label="Invested" dashed />
              </>
            ) : (
              assetTypes.map((type) => (
                <LegendItem key={type} color={getAssetStyle(type).color} label={getAssetStyle(type).label} />
              ))
            )}
          </div>
        </>
      )}
    </section>
  );
}

function LegendItem({ color, label, dashed = false }: { color: string; label: string; dashed?: boolean }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        className="inline-block h-0 w-4 border-t-2"
        style={{ borderColor: color, borderStyle: dashed ? "dashed" : "solid" }}
      />
      {label}
    </span>
  );
}
