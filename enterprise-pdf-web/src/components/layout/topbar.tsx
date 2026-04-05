"use client";

import { useAuth } from "@/providers/auth-provider";

export function Topbar({ title, subtitle }: { title: string; subtitle: string }) {
  const { user } = useAuth();

  return (
    <header className="mb-6 flex flex-wrap items-center justify-between gap-4">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.32em] text-accent">Console</p>
        <h1 className="mt-2 text-3xl font-bold text-ink">{title}</h1>
        <p className="mt-2 text-sm text-ink/65">{subtitle}</p>
      </div>
      <div className="rounded-full border border-line bg-white/70 px-4 py-2 text-sm text-ink/65">
        当前用户: {user?.name || "Guest"}
      </div>
    </header>
  );
}
