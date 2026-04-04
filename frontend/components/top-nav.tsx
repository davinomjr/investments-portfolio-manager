"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useVisibility } from "@/components/visibility-context";

const ITEMS = [
  { href: "/", label: "Portfolio" },
  { href: "/results", label: "Stocks" },
  { href: "/fiis", label: "FIIs" },
  { href: "/simulator", label: "Simulator" },
] as const;

export function TopNav() {
  const pathname = usePathname();
  const router = useRouter();
  const { visible, toggle } = useVisibility();

  async function handleLogout() {
    const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
    await fetch(`${base}/api/auth/logout`, { method: "POST", credentials: "include" });
    window.location.replace(`${base}/login`);
  }

  return (
    <header className="sticky top-0 z-20 border-b border-white/15 bg-[#1a1d25]/90 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-between gap-3 px-4 py-3 md:px-10 md:py-4">
        <div className="min-w-0">
          <p className="text-[11px] uppercase tracking-[0.35em] text-white/45">Portfolio Manager</p>
          <p className="mt-1 hidden text-sm text-white/65 sm:block">
            B3 import, holdings review, quarterly checks, and Monte Carlo simulation.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2 md:gap-4">
          <nav className="flex items-center gap-1 rounded-full border border-white/15 p-1">
            {ITEMS.map((item) => {
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={
                    active
                      ? "rounded-full bg-white px-3 py-1.5 text-xs font-semibold text-[#1a1d25] md:px-4 md:py-2 md:text-sm"
                      : "rounded-full px-3 py-1.5 text-xs font-semibold text-white/65 transition hover:bg-white/10 hover:text-white md:px-4 md:py-2 md:text-sm"
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
            className="flex items-center gap-1.5 rounded-full border border-white/20 bg-white/10 px-3 py-1.5 text-xs font-semibold text-white/80 transition hover:border-white/40 hover:text-white md:gap-2 md:px-4 md:py-2 md:text-sm"
          >
            <span className={`h-2 w-2 shrink-0 rounded-full ${visible ? "bg-pine" : "bg-white/30"}`} />
            <span className="hidden sm:inline">{visible ? "Hide values" : "Show values"}</span>
            <span className="sm:hidden">{visible ? "Hide" : "Show"}</span>
          </button>
          <button
            onClick={handleLogout}
            className="hidden rounded-full border border-white/15 px-4 py-2 text-sm font-semibold text-white/50 transition hover:border-white/30 hover:text-white/80 sm:block"
          >
            Sign out
          </button>
        </div>
      </div>
    </header>
  );
}
