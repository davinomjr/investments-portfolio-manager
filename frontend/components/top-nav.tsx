"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
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
  const [pendingHref, setPendingHref] = useState<string | null>(null);

  useEffect(() => {
    setPendingHref(null);
  }, [pathname]);

  async function handleLogout() {
    const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
    await fetch(`${base}/api/auth/logout`, { method: "POST", credentials: "include" });
    window.location.replace(`${base}/login`);
  }

  return (
    <header className="sticky top-0 z-20 border-b border-white/15 bg-[#1a1d25]/90 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-end gap-3 px-4 py-3 md:px-10 md:py-4">
        <div className="flex shrink-0 items-center gap-2 md:gap-4">
          <nav className="flex items-center gap-1 rounded-full border border-white/15 p-1">
            <span className="px-3 py-1.5 text-[10px] uppercase tracking-[0.3em] text-white/35 md:px-4 md:py-2">
              Portfolio Manager
            </span>
            <span className="h-4 w-px bg-white/15" />
            {ITEMS.map((item) => {
              const active = pathname === item.href;
              const pending = pendingHref === item.href && !active;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={() => { if (!active) setPendingHref(item.href); }}
                  className={
                    active
                      ? "rounded-full bg-white px-3 py-1.5 text-xs font-semibold text-[#1a1d25] md:px-4 md:py-2 md:text-sm"
                      : pending
                      ? "animate-pulse rounded-full bg-white/20 px-3 py-1.5 text-xs font-semibold text-white md:px-4 md:py-2 md:text-sm"
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
            className="rounded-full border border-white/15 px-3 py-1.5 text-xs font-semibold text-white/50 transition hover:border-white/30 hover:text-white/80 md:px-4 md:py-2 md:text-sm"
          >
            <span className="hidden sm:inline">Sign out</span>
            <span className="sm:hidden">Exit</span>
          </button>
        </div>
      </div>
    </header>
  );
}
