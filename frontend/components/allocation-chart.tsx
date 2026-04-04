"use client";

import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";

import type { Allocation } from "@/lib/api";
import { getAssetStyle } from "@/lib/asset-style";

export function AllocationChart({ allocations }: { allocations: Allocation[] }) {
  const grouped = (allocations ?? []).reduce<Record<string, number>>((acc, item) => {
    const key = item.asset_type || "Other";
    acc[key] = (acc[key] ?? 0) + item.weight * 100;
    return acc;
  }, {});

  const data = Object.entries(grouped).map(([assetType, weight]) => ({
    name: assetType,
    value: Number(weight.toFixed(2)),
    assetType,
  }));

  return (
    <section className="overflow-hidden rounded-[2rem] border border-white/15 bg-[#222530] p-4 md:p-6">
      <div className="mb-4 md:mb-6">
        <p className="text-xs uppercase tracking-[0.3em] text-white/55">Allocation</p>
        <h2 className="mt-2 text-2xl font-semibold">Top portfolio weights</h2>
      </div>
      <div className="h-56 sm:h-64 md:h-72">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={data}
              dataKey="value"
              nameKey="name"
              innerRadius="40%"
              outerRadius="70%"
              paddingAngle={2}
            >
              {data.map((entry, index) => (
                <Cell key={`${entry.name}-${index}`} fill={getAssetStyle(entry.assetType).color} />
              ))}
            </Pie>
            <Tooltip
              contentStyle={{ borderRadius: "16px", border: "1px solid rgba(255,255,255,0.15)", backgroundColor: "#272a36", color: "#fff" }}
              itemStyle={{ color: "#fff" }}
              labelStyle={{ color: "#fff" }}
              formatter={(value: number, _name, item) => {
                const payload = item.payload as { assetType: string };
                return [`${value.toFixed(2)}%`, getAssetStyle(payload.assetType).label];
              }}
              labelFormatter={() => ""}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}
