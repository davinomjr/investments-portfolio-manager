"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import type { ImportJobResponse } from "@/lib/api";

const API_BASE = (process.env.NEXT_PUBLIC_BASE_PATH ?? "") + (process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api");

type ImportMethod = "upload" | "b3" | "ibkr";

const TABS: { id: ImportMethod; label: string; shortLabel: string }[] = [
  { id: "upload", label: "Upload file", shortLabel: "Upload" },
  { id: "b3", label: "B3 sync", shortLabel: "B3" },
  { id: "ibkr", label: "IBKR sync", shortLabel: "IBKR" },
];

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
        Session expired — re-authenticate via B3 sync
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
  const [activeMethod, setActiveMethod] = useState<ImportMethod>("upload");
  const [message, setMessage] = useState<string | null>(null);
  const [syncResult, setSyncResult] = useState<string | null>(null);
  const [isSyncing, startSyncTransition] = useTransition();
  const [ibkrResult, setIbkrResult] = useState<string | null>(null);
  const [isIbkrSyncing, startIbkrSyncTransition] = useTransition();

  const onFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    startSyncTransition(async () => {
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
        setMessage(payload.detail ?? `Imported ${file.name}. Refreshing...`);
        router.refresh();
      } catch (error) {
        setMessage(
          error instanceof Error
            ? `${error.message}. Check that the API is running and reachable at ${API_BASE}.`
            : `Upload failed. Check that the API is running and reachable at ${API_BASE}.`,
        );
      } finally {
        event.target.value = "";
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
    <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-5 shadow-[0_18px_60px_rgba(0,0,0,0.25)] md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.3em] text-white/55">Import</p>
          <h2 className="mt-2 text-xl font-semibold md:text-2xl">Import positions</h2>
        </div>
        {latestJob ? <ImportStatusBadge job={latestJob} /> : null}
      </div>

      <div
        role="tablist"
        aria-label="Import method"
        className="mt-4 flex w-full rounded-full border border-white/10 bg-black/20 p-1 sm:w-auto sm:self-start"
      >
        {TABS.map((tab) => {
          const isActive = activeMethod === tab.id;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={isActive}
              onClick={() => setActiveMethod(tab.id)}
              className={`flex-1 rounded-full px-4 py-2 text-sm font-medium transition sm:flex-none ${
                isActive ? "bg-white text-[#1a1d25]" : "text-white/65 hover:text-white"
              }`}
            >
              <span className="sm:hidden">{tab.shortLabel}</span>
              <span className="hidden sm:inline">{tab.label}</span>
            </button>
          );
        })}
      </div>

      <div className="mt-5">
        {activeMethod === "upload" && (
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <p className="text-sm text-white/65">Upload a `.xlsx` or `.csv` export from B3.</p>
            <label className="inline-flex cursor-pointer items-center justify-center rounded-full border border-white bg-white px-5 py-3 text-sm font-semibold text-[#1a1d25] transition hover:bg-transparent hover:text-white">
              {isSyncing ? "Uploading..." : "Select file"}
              <input type="file" accept=".xlsx,.xlsm,.csv" onChange={onFileSelect} className="hidden" disabled={isSyncing} />
            </label>
          </div>
        )}

        {activeMethod === "b3" && (
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <p className="text-sm text-white/65">Sync positions directly from the B3 investor portal.</p>
            <button
              type="button"
              onClick={onSyncB3}
              disabled={isSyncing}
              className="inline-flex cursor-pointer items-center justify-center rounded-full border border-white bg-white px-5 py-3 text-sm font-semibold text-[#1a1d25] transition hover:bg-transparent hover:text-white disabled:opacity-50"
            >
              {isSyncing ? "Syncing..." : "Sync from B3"}
            </button>
          </div>
        )}

        {activeMethod === "ibkr" && (
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <p className="text-sm text-white/65">Pull the latest positions from your IBKR Flex query.</p>
            <button
              type="button"
              onClick={onSyncIBKR}
              disabled={isIbkrSyncing}
              className="inline-flex cursor-pointer items-center justify-center rounded-full border border-white bg-white px-5 py-3 text-sm font-semibold text-[#1a1d25] transition hover:bg-transparent hover:text-white disabled:opacity-50"
            >
              {isIbkrSyncing ? "Syncing..." : "Sync from IBKR"}
            </button>
          </div>
        )}

        {activeMethod === "upload" && message ? <p className="mt-3 text-sm text-white/65">{message}</p> : null}
        {activeMethod === "b3" && syncResult ? <p className="mt-3 text-sm text-white/65">{syncResult}</p> : null}
        {activeMethod === "ibkr" && ibkrResult ? <p className="mt-3 text-sm text-white/65">{ibkrResult}</p> : null}
      </div>
    </section>
  );
}
