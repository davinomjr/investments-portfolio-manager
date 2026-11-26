"use client";

import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip, type PieLabelRenderProps } from "recharts";

import type { Allocation } from "@/lib/api";
import { ALLOCATION_BUCKET_COLORS, allocationBucket, type AllocationBucket } from "@/lib/asset-style";
import { useVisibility } from "@/components/visibility-context";

const brlFormatter = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  maximumFractionDigits: 0,
});

type Slice = { name: AllocationBucket; value: number; marketValue: number };

const RADIAN = Math.PI / 180;

// Name + percentage next to each slice, with a short leader line.
function renderSliceLabel(props: PieLabelRenderProps) {
  const cx = Number(props.cx);
  const cy = Number(props.cy);
  const midAngle = Number(props.midAngle);
  const outerRadius = Number(props.outerRadius);
  const percent = Number(props.percent ?? 0);
  if (percent < 0.01) return null;
  const radius = outerRadius + 16;
  const x = cx + radius * Math.cos(-midAngle * RADIAN);
  const y = cy + radius * Math.sin(-midAngle * RADIAN);
  const anchor = x > cx ? "start" : "end";
  return (
    <text x={x} y={y} textAnchor={anchor} dominantBaseline="central" className="fill-white text-[11px] sm:text-xs">
      <tspan x={x} dy="-0.6em" fontWeight={600}>
        {props.name}
      </tspan>
      <tspan x={x} dy="1.2em" className="fill-white/65">
        {(percent * 100).toFixed(1)}%
      </tspan>
    </text>
  );
}

export function AllocationChart({ allocations }: { allocations: Allocation[] }) {
  const { visible } = useVisibility();

  const totals = new Map<AllocationBucket, number>();
  for (const item of allocations ?? []) {
    const bucket = allocationBucket(item.asset_type, item.source);
    totals.set(bucket, (totals.get(bucket) ?? 0) + item.market_value);
  }
  const total = [...totals.values()].reduce((sum, v) => sum + v, 0);
  const data: Slice[] = [...totals.entries()]
    .map(([name, marketValue]) => ({
      name,
      marketValue,
      value: total > 0 ? Number(((marketValue / total) * 100).toFixed(2)) : 0,
    }))
    .sort((a, b) => b.marketValue - a.marketValue);

  return (
    <section className="overflow-hidden rounded-[2rem] border border-white/15 bg-[#222530] p-4 md:p-6">
      <div className="mb-2 md:mb-4">
        <p className="text-xs uppercase tracking-[0.3em] text-white/55">Allocation</p>
        <h2 className="mt-2 text-2xl font-semibold">By category</h2>
      </div>
      <div className="h-64 sm:h-72">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart margin={{ top: 16, right: 24, bottom: 16, left: 24 }}>
            <Pie
              data={data}
              dataKey="value"
              nameKey="name"
              innerRadius="38%"
              outerRadius="62%"
              paddingAngle={2}
              label={renderSliceLabel}
              labelLine={{ stroke: "rgba(255,255,255,0.3)" }}
              isAnimationActive={false}
            >
              {data.map((entry) => (
                <Cell key={entry.name} fill={ALLOCATION_BUCKET_COLORS[entry.name]} stroke="none" />
              ))}
            </Pie>
            <Tooltip
              contentStyle={{ borderRadius: "16px", border: "1px solid rgba(255,255,255,0.15)", backgroundColor: "#272a36", color: "#fff" }}
              itemStyle={{ color: "#fff" }}
              labelStyle={{ color: "#fff" }}
              formatter={(value: number, name, item) => {
                const payload = item.payload as Slice;
                const amount = visible ? brlFormatter.format(payload.marketValue) : "**";
                return [`${amount} (${value.toFixed(1)}%)`, name];
              }}
              labelFormatter={() => ""}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <ul className="mt-2 grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-1">
        {data.map((slice) => (
          <li key={slice.name} className="flex min-w-0 items-center gap-2">
            <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: ALLOCATION_BUCKET_COLORS[slice.name] }} />
            <span className="truncate text-white/80">{slice.name}</span>
            <span className="ml-auto whitespace-nowrap tabular-nums text-white/60">
              {visible ? brlFormatter.format(slice.marketValue) : "**"}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
