"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useVisibility } from "@/components/visibility-context";

const ITEMS = [
  { href: "/", label: "Portfolio" },
  { href: "/results", label: "Stocks" },
  { href: "/fiis", label: "FIIs" },
  { href: "/simulator", label: "Simulator" },
] as const;

export function TopNav() {
  const pathname = usePathname();
  const { visible, toggle } = useVisibility();

  return (
    <header className="sticky top-0 z-20 border-b border-white/15 bg-[#1a1d25]/90 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-4 md:px-10">
        <div>
          <p className="text-[11px] uppercase tracking-[0.35em] text-white/45">Portfolio Manager</p>
          <p className="mt-1 text-sm text-white/65">
            B3 import, holdings review, quarterly checks, and Monte Carlo simulation.
          </p>
        </div>
        <div className="flex items-center gap-4">
          <nav className="flex items-center gap-2 rounded-full border border-white/15 p-1">
            {ITEMS.map((item) => {
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={
                    active
                      ? "rounded-full bg-white px-4 py-2 text-sm font-semibold text-[#1a1d25]"
                      : "rounded-full px-4 py-2 text-sm font-semibold text-white/65 transition hover:bg-white/10 hover:text-white"
                  }
                >
                  {item.label}
                </Link>
              );
            })}
          </nav>
          <button
            onClick={toggle}
            aria-pressed={visible}
            className="flex items-center gap-2 rounded-full border border-white/20 bg-white/10 px-4 py-2 text-sm font-semibold text-white/80 transition hover:border-white/40 hover:text-white"
          >
            <span className={`h-2 w-2 rounded-full ${visible ? "bg-pine" : "bg-white/30"}`} />
            {visible ? "Hide values" : "Show values"}
          </button>
        </div>
      </div>
    </header>
  );
}
