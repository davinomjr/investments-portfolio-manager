export default function Loading() {
  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-8 px-6 py-10 md:px-10">
      <section className="grid gap-8 md:grid-cols-[1.2fr_0.8fr] md:items-end">
        <div className="flex flex-col gap-4">
          <div className="h-3 w-24 animate-pulse rounded-full bg-white/15" />
          <div className="h-12 w-3/4 animate-pulse rounded-2xl bg-white/10" />
          <div className="h-5 w-2/3 animate-pulse rounded-full bg-white/10" />
        </div>
        <div className="h-32 animate-pulse rounded-[2rem] bg-white/10" />
      </section>

      <section className="rounded-[2rem] border border-white/15 bg-[#222530] p-6">
        <div className="mb-6 flex flex-col gap-3">
          <div className="h-3 w-28 animate-pulse rounded-full bg-white/15" />
          <div className="h-7 w-64 animate-pulse rounded-xl bg-white/10" />
        </div>
        <div className="grid gap-4 xl:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="rounded-[1.75rem] border border-white/10 bg-[#272a36] p-5">
              <div className="flex items-start justify-between gap-3">
                <div className="flex flex-col gap-3">
                  <div className="flex items-center gap-3">
                    <div className="h-8 w-24 animate-pulse rounded-xl bg-white/15" />
                    <div className="h-6 w-16 animate-pulse rounded-full bg-white/10" />
                  </div>
                  <div className="h-4 w-40 animate-pulse rounded-full bg-white/10" />
                </div>
                <div className="flex flex-col items-end gap-2">
                  <div className="h-3 w-12 animate-pulse rounded-full bg-white/10" />
                  <div className="h-5 w-16 animate-pulse rounded-full bg-white/15" />
                </div>
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                {Array.from({ length: 4 }).map((_, j) => (
                  <div key={j} className="rounded-[1.25rem] border border-white/10 bg-[#272a36] p-4">
                    <div className="h-3 w-20 animate-pulse rounded-full bg-white/10" />
                    <div className="mt-3 h-8 w-28 animate-pulse rounded-xl bg-white/15" />
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}
