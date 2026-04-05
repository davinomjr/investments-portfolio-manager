"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useVisibility } from "@/components/visibility-context";
import { Home, ChartNoAxesCombined, Building2, ChartCandlestick } from "lucide-react";

const ITEMS = [
  { href: "/", label: "Portfolio", Icon: Home },
  { href: "/results", label: "Stocks", Icon: ChartNoAxesCombined },
  { href: "/fiis", label: "FIIs", Icon: Building2 },
  { href: "/simulator", label: "Simulator", Icon: ChartCandlestick },
] as const;

export function TopNav() {
  const pathname = usePathname();
  const router = useRouter();
  const { visible, toggle } = useVisibility();
  const [pendingHref, setPendingHref] = useState<string | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setPendingHref(null);
  }, [pathname]);

  useEffect(() => {
    if (!menuOpen) return;
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [menuOpen]);

  async function handleLogout() {
    const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
    await fetch(`${base}/api/auth/logout`, { method: "POST", credentials: "include" });
    window.location.replace(`${base}/login`);
  }

  return (
    <header className="sticky top-0 z-20 border-b border-white/15 bg-[#1a1d25]/90 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-end gap-3 px-4 py-3 md:px-10 md:py-4">
        <div className="flex shrink-0 items-center gap-2 md:gap-4">
          {/* Nav pills — hidden on mobile, bottom nav used instead */}
          <nav className="hidden md:flex items-center gap-1 rounded-full border border-white/15 p-1">
            <span className="px-4 py-2 text-[10px] uppercase tracking-[0.3em] text-white/35">
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
                  title={item.label}
                  onClick={() => { if (!active) setPendingHref(item.href); }}
                  className={
                    active
                      ? "rounded-full bg-white p-2.5 text-[#1a1d25]"
                      : pending
                      ? "animate-pulse rounded-full bg-white/20 p-2.5 text-white"
                      : "rounded-full p-2.5 text-white/50 transition hover:bg-white/10 hover:text-white"
                  }
                >
                  <item.Icon size={18} strokeWidth={1.75} />
                </Link>
              );
            })}
          </nav>

          {/* Hide/Show values — always visible */}
          <button
            onClick={toggle}
            aria-pressed={visible}
            className="flex items-center gap-1.5 rounded-full border border-white/20 bg-white/10 px-3 py-1.5 text-xs font-semibold text-white/80 transition hover:border-white/40 hover:text-white md:gap-2 md:px-4 md:py-2 md:text-sm"
          >
            <span className={`h-2 w-2 shrink-0 rounded-full ${visible ? "bg-pine" : "bg-white/30"}`} />
            <span className="hidden sm:inline">{visible ? "Hide values" : "Show values"}</span>
            <span className="sm:hidden">{visible ? "Hide" : "Show"}</span>
          </button>

          {/* Overflow menu: refresh + sign out */}
          <div className="relative" ref={menuRef}>
            <button
              onClick={() => setMenuOpen((o) => !o)}
              aria-label="More options"
              className="flex items-center justify-center rounded-full border border-white/15 px-3 py-1.5 text-white/50 transition hover:border-white/30 hover:text-white/80 md:px-4 md:py-2"
            >
              <span className="text-sm font-bold leading-none tracking-widest">···</span>
            </button>

            {menuOpen && (
              <div className="absolute right-0 top-full mt-2 min-w-[140px] rounded-2xl border border-white/15 bg-[#1a1d25] py-1 shadow-xl">
                <button
                  onClick={() => { router.refresh(); setMenuOpen(false); }}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-white/60 transition hover:bg-white/5 hover:text-white"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-4 w-4 shrink-0">
                    <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 0 1-9.201 2.466l-.312-.311h2.433a.75.75 0 0 0 0-1.5H3.989a.75.75 0 0 0-.75.75v4.242a.75.75 0 0 0 1.5 0v-2.43l.31.31a7 7 0 0 0 11.712-3.138.75.75 0 0 0-1.449-.39Zm1.23-3.723a.75.75 0 0 0 .219-.53V2.929a.75.75 0 0 0-1.5 0v2.43l-.31-.31A7 7 0 0 0 3.239 8.188a.75.75 0 1 0 1.448.389A5.5 5.5 0 0 1 13.89 6.11l.311.31h-2.432a.75.75 0 0 0 0 1.5h4.243a.75.75 0 0 0 .53-.219Z" clipRule="evenodd" />
                  </svg>
                  Refresh
                </button>
                <div className="mx-3 my-1 h-px bg-white/10" />
                <button
                  onClick={() => { setMenuOpen(false); handleLogout(); }}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-white/60 transition hover:bg-white/5 hover:text-white"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-4 w-4 shrink-0">
                    <path fillRule="evenodd" d="M3 4.25A2.25 2.25 0 0 1 5.25 2h5.5A2.25 2.25 0 0 1 13 4.25v2a.75.75 0 0 1-1.5 0v-2a.75.75 0 0 0-.75-.75h-5.5a.75.75 0 0 0-.75.75v11.5c0 .414.336.75.75.75h5.5a.75.75 0 0 0 .75-.75v-2a.75.75 0 0 1 1.5 0v2A2.25 2.25 0 0 1 10.75 18h-5.5A2.25 2.25 0 0 1 3 15.75V4.25Z" clipRule="evenodd" />
                    <path fillRule="evenodd" d="M19 10a.75.75 0 0 0-.75-.75H8.704l1.048-1.007a.75.75 0 1 0-1.004-1.117l-2.5 2.25a.75.75 0 0 0 0 1.118l2.5 2.25a.75.75 0 1 0 1.004-1.117L8.704 10.75H18.25A.75.75 0 0 0 19 10Z" clipRule="evenodd" />
                  </svg>
                  Sign out
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </header>
  );
}
