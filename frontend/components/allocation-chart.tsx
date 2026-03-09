"use client";

import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";

import type { Allocation } from "@/lib/api";
import { getAssetStyle } from "@/lib/asset-style";

export function AllocationChart({ allocations }: { allocations: Allocation[] }) {
  const data = allocations.slice(0, 8).map((item) => ({
    name: item.ticker,
    value: Number((item.weight * 100).toFixed(2)),
    assetType: item.asset_type,
  }));

  return (
    <section className="rounded-[2rem] border border-white/15 bg-[#151820] p-6">
      <div className="mb-6">
        <p className="text-xs uppercase tracking-[0.3em] text-white/55">Allocation</p>
        <h2 className="mt-2 text-2xl font-semibold">Top portfolio weights</h2>
      </div>
      <div className="h-72">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={data}
              dataKey="value"
              nameKey="name"
              innerRadius={62}
              outerRadius={108}
              paddingAngle={2}
            >
              {data.map((entry, index) => (
                <Cell key={`${entry.name}-${index}`} fill={getAssetStyle(entry.assetType).color} />
              ))}
            </Pie>
            <Tooltip
              contentStyle={{ borderRadius: "16px", border: "1px solid rgba(255,255,255,0.15)", backgroundColor: "#1e2028", color: "#fff" }}
              formatter={(value: number, _name, item) => {
                const payload = item.payload as { assetType: string };
                return [`${value.toFixed(2)}%`, getAssetStyle(payload.assetType).label];
              }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}
