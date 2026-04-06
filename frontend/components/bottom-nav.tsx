"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState, useEffect } from "react";
import { Home, ChartNoAxesCombined, Building2, ChartCandlestick } from "lucide-react";

const ITEMS = [
  { href: "/", label: "Portfolio", Icon: Home },
  { href: "/results", label: "Stocks", Icon: ChartNoAxesCombined },
  { href: "/fiis", label: "FIIs", Icon: Building2 },
  { href: "/simulator", label: "Simulator", Icon: ChartCandlestick },
] as const;

export function BottomNav() {
  const pathname = usePathname();
  const [pendingHref, setPendingHref] = useState<string | null>(null);

  useEffect(() => {
    setPendingHref(null);
  }, [pathname]);

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-20 md:hidden flex justify-center"
      style={{ paddingBottom: "max(1.25rem, env(safe-area-inset-bottom))" }}
    >
      <div className="flex items-center gap-1 rounded-full bg-[#0d0f14]/90 px-2 py-2 shadow-2xl backdrop-blur border border-white/[0.06]">
        {ITEMS.map((item) => {
          const active = pathname === item.href;
          const pending = pendingHref === item.href && !active;
          return (
            <Link
              key={item.href}
              href={item.href}
              onClick={() => { if (!active) setPendingHref(item.href); }}
              aria-label={item.label}
              className={`flex items-center justify-center rounded-full p-3 transition-all duration-150 ${
                active
                  ? "bg-white/90 text-[#0d0f14]"
                  : pending
                  ? "animate-pulse text-white/40"
                  : "text-white/30 hover:text-white/60 hover:bg-white/5"
              }`}
            >
              <item.Icon size={19} strokeWidth={1.75} />
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
