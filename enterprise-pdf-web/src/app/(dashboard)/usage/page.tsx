"use client";

import { Topbar } from "@/components/layout/topbar";
import { formatNumber } from "@/lib/utils";
import { apiRequest } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import type { UsageSummary } from "@/types/api";
import { useQuery } from "@tanstack/react-query";
import { BarChart3 } from "lucide-react";
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

export default function UsagePage() {
  const { token } = useAuth();

  const { data, isLoading } = useQuery({
    queryKey: ["usage-summary"],
    queryFn: () => apiRequest<UsageSummary>("/usage/summary", {}, token),
    enabled: !!token
  });

  return (
    <div>
      <Topbar title="Usage" subtitle="跟踪最近 14 天的 Token 使用趋势，为配额和计费打基础。" />

      <div className="grid gap-4 lg:grid-cols-[280px_1fr]">
        <section className="mesh-card rounded-[1.75rem] border border-line p-6 shadow-panel">
          <div className="flex items-center justify-between">
            <p className="text-sm text-ink/60">累计 Token</p>
            <BarChart3 className="h-5 w-5 text-accent" />
          </div>
          <p className="mt-4 text-4xl font-bold">{formatNumber(data?.total_tokens ?? 0)}</p>
        </section>

        <section className="mesh-card rounded-[1.75rem] border border-line p-6 shadow-panel">
          {isLoading ? <p className="text-sm text-ink/60">正在加载用量数据...</p> : null}
          <div className="h-[320px]">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data?.daily_tokens || []}>
                <XAxis dataKey="date" tick={{ fontSize: 12 }} />
                <YAxis tick={{ fontSize: 12 }} />
                <Tooltip />
                <Area type="monotone" dataKey="tokens" stroke="#1d7d66" fill="#1d7d66" fillOpacity={0.16} strokeWidth={3} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </section>
      </div>
    </div>
  );
}
