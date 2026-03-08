"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const ITEMS = [
  { href: "/", label: "Portfolio" },
  { href: "/results", label: "Results" },
];

export function TopNav() {
  const pathname = usePathname();

  return (
    <header className="sticky top-0 z-20 border-b border-black bg-white/90 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-4 md:px-10">
        <div>
          <p className="text-[11px] uppercase tracking-[0.35em] text-black/45">Portfolio Manager</p>
          <p className="mt-1 text-sm text-black/65">B3 import, holdings review, and quarterly checks.</p>
        </div>
        <nav className="flex items-center gap-2 rounded-full border border-black p-1">
          {ITEMS.map((item) => {
            const active = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={
                  active
                    ? "rounded-full bg-black px-4 py-2 text-sm font-semibold text-white"
                    : "rounded-full px-4 py-2 text-sm font-semibold text-black/65 transition hover:bg-black hover:text-white"
                }
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
      </div>
    </header>
  );
}
