"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api";

export function UploadPanel() {
  const router = useRouter();
  const [message, setMessage] = useState<string>("Upload a B3 `.xlsx` or `.csv` export to refresh positions.");
  const [isPending, startTransition] = useTransition();

  const onChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    startTransition(async () => {
      try {
        const body = new FormData();
        body.append("file", file);

        const response = await fetch(`${API_BASE}/portfolio/import-file`, {
          method: "POST",
          body,
        });

        const payload = await response.json().catch(() => ({}));
        if (!response.ok) {
          setMessage(payload.detail ?? "Import failed.");
          return;
        }
        setMessage(payload.detail ?? `Imported ${file.name}.`);
        router.refresh();
      } catch (error) {
        setMessage(
          error instanceof Error
            ? `${error.message}. Check that the API is running and reachable at ${API_BASE}.`
            : `Upload failed. Check that the API is running and reachable at ${API_BASE}.`,
        );
      }
    });
  };

  return (
    <section className="rounded-[2rem] border border-white/15 bg-[#151820] p-6 shadow-[0_18px_60px_rgba(0,0,0,0.25)]">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">B3 Manual Import</p>
          <h2 className="mt-2 text-2xl font-semibold">Load a fresh portfolio snapshot</h2>
        </div>
        <label className="inline-flex cursor-pointer items-center justify-center rounded-full border border-white bg-white px-5 py-3 text-sm font-semibold text-[#0d0f14] transition hover:bg-transparent hover:text-white">
          {isPending ? "Importing..." : "Choose B3 file"}
          <input className="hidden" type="file" accept=".xlsx,.xlsm,.csv" onChange={onChange} disabled={isPending} />
        </label>
      </div>
      <p className="mt-4 text-sm text-white/65">{message}</p>
    </section>
  );
}
