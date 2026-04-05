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
    <nav className="fixed bottom-0 left-0 right-0 z-20 md:hidden">
      <div
        className="mx-4 mb-4 flex items-center justify-around rounded-full border border-white/10 bg-[#1a1d25]/95 px-2 py-2 shadow-xl backdrop-blur"
        style={{ marginBottom: "max(1rem, env(safe-area-inset-bottom))" }}
      >
        {ITEMS.map((item) => {
          const active = pathname === item.href;
          const pending = pendingHref === item.href && !active;
          return (
            <Link
              key={item.href}
              href={item.href}
              onClick={() => { if (!active) setPendingHref(item.href); }}
              aria-label={item.label}
              className={`flex items-center justify-center rounded-full p-3 transition-all ${
                active
                  ? "bg-white text-[#1a1d25]"
                  : pending
                  ? "animate-pulse text-white/50"
                  : "text-white/35 hover:text-white/70"
              }`}
            >
              <item.Icon size={22} strokeWidth={1.75} />
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
