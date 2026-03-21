"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import type { ImportJobResponse } from "@/lib/api";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api";

function formatTimestamp(iso: string): string {
  try {
    return new Intl.DateTimeFormat("en-US", {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    }).format(new Date(iso));
  } catch {
    return iso;
  }
}

function ImportStatusBadge({ job }: { job: ImportJobResponse }) {
  if (job.status === "completed") {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/15 px-3 py-1 text-xs font-medium text-emerald-400">
        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
        Last synced: {formatTimestamp(job.updated_at)}
      </span>
    );
  }
  if (job.status === "requires_login") {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-500/15 px-3 py-1 text-xs font-medium text-amber-400">
        <span className="h-1.5 w-1.5 rounded-full bg-amber-400" />
        Session expired — click Sync from B3 to re-authenticate
      </span>
    );
  }
  if (job.status === "failed") {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-red-500/15 px-3 py-1 text-xs font-medium text-red-400">
        <span className="h-1.5 w-1.5 rounded-full bg-red-400" />
        Last import failed: {job.detail ?? "unknown error"}
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full bg-white/10 px-3 py-1 text-xs font-medium text-white/55">
      <span className="h-1.5 w-1.5 rounded-full bg-white/40" />
      Status: {job.status}
    </span>
  );
}

export function UploadPanel({ latestJob }: { latestJob?: ImportJobResponse | null }) {
  const router = useRouter();
  const [message, setMessage] = useState<string>("Upload a B3 `.xlsx` or `.csv` export to refresh positions.");
  const [syncResult, setSyncResult] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();
  const [isSyncing, startSyncTransition] = useTransition();
  const [ibkrResult, setIbkrResult] = useState<string | null>(null);
  const [isIbkrSyncing, startIbkrSyncTransition] = useTransition();

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

  const onSyncB3 = () => {
    startSyncTransition(async () => {
      setSyncResult(null);
      try {
        const response = await fetch(`${API_BASE}/portfolio/import-b3`, { method: "POST" });
        if (!response.ok) {
          const payload = await response.json().catch(() => ({}));
          setSyncResult(payload.detail ?? "Sync failed.");
          return;
        }
        // Import runs in the background — poll until it finishes
        setSyncResult("Syncing… this may take a minute.");
        for (let i = 0; i < 60; i++) {
          await new Promise((r) => setTimeout(r, 5000));
          const jobRes = await fetch(`${API_BASE}/portfolio/import-jobs/latest`).catch(() => null);
          if (!jobRes?.ok) continue;
          const job = await jobRes.json().catch(() => null);
          if (!job) continue;
          if (job.status === "completed") {
            setSyncResult(job.detail ?? "Sync complete.");
            router.refresh();
            return;
          }
          if (job.status === "failed" || job.status === "requires_login") {
            setSyncResult(job.detail ?? "Sync failed.");
            return;
          }
        }
        setSyncResult("Sync is taking longer than expected — check back shortly.");
      } catch (error) {
        setSyncResult(
          error instanceof Error ? error.message : `Sync failed. Check that the API is running at ${API_BASE}.`,
        );
      }
    });
  };

  const onSyncIBKR = () => {
    startIbkrSyncTransition(async () => {
      setIbkrResult(null);
      try {
        const response = await fetch(`${API_BASE}/portfolio/import-ibkr`, { method: "POST" });
        if (!response.ok) {
          const payload = await response.json().catch(() => ({}));
          setIbkrResult(payload.detail ?? "IBKR sync failed.");
          return;
        }
        setIbkrResult("Syncing IBKR… this may take a moment.");
        for (let i = 0; i < 30; i++) {
          await new Promise((r) => setTimeout(r, 5000));
          const jobRes = await fetch(`${API_BASE}/portfolio/import-jobs/latest`).catch(() => null);
          if (!jobRes?.ok) continue;
          const job = await jobRes.json().catch(() => null);
          if (!job) continue;
          if (job.status === "completed") {
            setIbkrResult(job.detail ?? "IBKR sync complete.");
            router.refresh();
            return;
          }
          if (job.status === "failed") {
            setIbkrResult(job.detail ?? "IBKR sync failed.");
            return;
          }
        }
        setIbkrResult("IBKR sync is taking longer than expected — check back shortly.");
      } catch (error) {
        setIbkrResult(error instanceof Error ? error.message : "IBKR sync failed.");
      }
    });
  };

  return (
    <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-6 shadow-[0_18px_60px_rgba(0,0,0,0.25)]">
      {/* B3 Sync row */}
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">B3 Sync</p>
          <h2 className="mt-2 text-2xl font-semibold">Sync directly from B3</h2>
          {latestJob && (
            <div className="mt-2">
              <ImportStatusBadge job={latestJob} />
            </div>
          )}
        </div>
        <button
          onClick={onSyncB3}
          disabled={isSyncing}
          className="inline-flex cursor-pointer items-center justify-center rounded-full border border-white bg-white px-5 py-3 text-sm font-semibold text-[#1a1d25] transition hover:bg-transparent hover:text-white disabled:opacity-50"
        >
          {isSyncing ? "Syncing..." : "Sync from B3"}
        </button>
      </div>
      {syncResult && <p className="mt-3 text-sm text-white/65">{syncResult}</p>}

      <hr className="my-5 border-white/10" />

      {/* IBKR Sync row */}
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">IBKR Sync</p>
          <h2 className="mt-2 text-2xl font-semibold">Sync directly from IBKR</h2>
        </div>
        <button
          onClick={onSyncIBKR}
          disabled={isIbkrSyncing}
          className="inline-flex cursor-pointer items-center justify-center rounded-full border border-white bg-white px-5 py-3 text-sm font-semibold text-[#1a1d25] transition hover:bg-transparent hover:text-white disabled:opacity-50"
        >
          {isIbkrSyncing ? "Syncing..." : "Sync from IBKR"}
        </button>
      </div>
      {ibkrResult && <p className="mt-3 text-sm text-white/65">{ibkrResult}</p>}

      <hr className="my-5 border-white/10" />

      {/* Manual file upload row */}
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">B3 Manual Import</p>
          <h2 className="mt-2 text-xl font-semibold">Load a fresh portfolio snapshot</h2>
        </div>
        <label className="inline-flex cursor-pointer items-center justify-center rounded-full border border-white/50 bg-transparent px-5 py-3 text-sm font-semibold text-white transition hover:border-white hover:bg-white hover:text-[#1a1d25]">
          {isPending ? "Importing..." : "Choose B3 file"}
          <input className="hidden" type="file" accept=".xlsx,.xlsm,.csv" onChange={onChange} disabled={isPending} />
        </label>
      </div>
      <p className="mt-4 text-sm text-white/65">{message}</p>
    </section>
  );
}
