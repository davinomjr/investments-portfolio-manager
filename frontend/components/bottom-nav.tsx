"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState, useEffect } from "react";

const ITEMS = [
  {
    href: "/",
    label: "Portfolio",
    icon: (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-6 w-6">
        <path fillRule="evenodd" d="M9.293 2.293a1 1 0 0 1 1.414 0l7 7A1 1 0 0 1 17 11h-1v6a1 1 0 0 1-1 1h-2a1 1 0 0 1-1-1v-3a1 1 0 0 0-1-1H9a1 1 0 0 0-1 1v3a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1v-6H3a1 1 0 0 1-.707-1.707l7-7Z" clipRule="evenodd" />
      </svg>
    ),
  },
  {
    href: "/results",
    label: "Stocks",
    icon: (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-6 w-6">
        <path d="M15.5 2A1.5 1.5 0 0 0 14 3.5v13a1.5 1.5 0 0 0 3 0v-13A1.5 1.5 0 0 0 15.5 2ZM9.5 6A1.5 1.5 0 0 0 8 7.5v9a1.5 1.5 0 0 0 3 0v-9A1.5 1.5 0 0 0 9.5 6ZM3.5 10A1.5 1.5 0 0 0 2 11.5v5a1.5 1.5 0 0 0 3 0v-5A1.5 1.5 0 0 0 3.5 10Z" />
      </svg>
    ),
  },
  {
    href: "/fiis",
    label: "FIIs",
    icon: (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-6 w-6">
        <path fillRule="evenodd" d="M4 16.5v-13h-.25a.75.75 0 0 1 0-1.5h12.5a.75.75 0 0 1 0 1.5H16v13h.25a.75.75 0 0 1 0 1.5h-3.5a.75.75 0 0 1-.75-.75v-2.5a.75.75 0 0 0-.75-.75h-2.5a.75.75 0 0 0-.75.75v2.5a.75.75 0 0 1-.75.75h-3.5a.75.75 0 0 1 0-1.5H4Zm3-11a.75.75 0 0 1 .75-.75h.5a.75.75 0 0 1 0 1.5h-.5A.75.75 0 0 1 7 5.5Zm.75 2.25a.75.75 0 0 0 0 1.5h.5a.75.75 0 0 0 0-1.5h-.5ZM7 11.5a.75.75 0 0 1 .75-.75h.5a.75.75 0 0 1 0 1.5h-.5a.75.75 0 0 1-.75-.75Zm3.75-4.75a.75.75 0 0 0 0 1.5h.5a.75.75 0 0 0 0-1.5h-.5Zm-.75 4.75a.75.75 0 0 1 .75-.75h.5a.75.75 0 0 1 0 1.5h-.5a.75.75 0 0 1-.75-.75ZM10.75 5a.75.75 0 0 0 0 1.5h.5a.75.75 0 0 0 0-1.5h-.5Z" clipRule="evenodd" />
      </svg>
    ),
  },
  {
    href: "/simulator",
    label: "Simulator",
    icon: (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-6 w-6">
        <path d="M15.98 1.804a1 1 0 0 0-1.96 0l-.24 1.192a1 1 0 0 1-.784.785l-1.192.238a1 1 0 0 0 0 1.962l1.192.238a1 1 0 0 1 .785.785l.238 1.192a1 1 0 0 0 1.962 0l.238-1.192a1 1 0 0 1 .785-.785l1.192-.238a1 1 0 0 0 0-1.962l-1.192-.238a1 1 0 0 1-.785-.785l-.238-1.192ZM6.949 5.684a1 1 0 0 0-1.898 0l-.683 2.051a1 1 0 0 1-.633.633l-2.051.683a1 1 0 0 0 0 1.898l2.051.684a1 1 0 0 1 .633.632l.683 2.051a1 1 0 0 0 1.898 0l.683-2.051a1 1 0 0 1 .633-.633l2.051-.683a1 1 0 0 0 0-1.898l-2.051-.683a1 1 0 0 1-.633-.633L6.95 5.684ZM13.949 13.684a1 1 0 0 0-1.898 0l-.184.551a1 1 0 0 1-.632.633l-.551.183a1 1 0 0 0 0 1.898l.551.183a1 1 0 0 1 .633.633l.183.551a1 1 0 0 0 1.898 0l.184-.551a1 1 0 0 1 .632-.633l.551-.183a1 1 0 0 0 0-1.898l-.551-.184a1 1 0 0 1-.633-.632l-.183-.551Z" />
      </svg>
    ),
  },
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
              {item.icon}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
