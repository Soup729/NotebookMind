"use client";

import { Topbar } from "@/components/layout/topbar";
import { formatNumber } from "@/lib/utils";
import { apiRequest } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import type { DashboardOverview } from "@/types/api";
import { useQuery } from "@tanstack/react-query";
import { FileText, MessageSquare, Sparkles, WalletCards } from "lucide-react";
import { Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

const statCards = [
  { key: "total_documents", label: "总文档数", icon: FileText },
  { key: "completed_documents", label: "已完成解析", icon: Sparkles },
  { key: "total_sessions", label: "会话数量", icon: MessageSquare },
  { key: "total_tokens", label: "Token 用量", icon: WalletCards }
] as const;

export default function DashboardPage() {
  const { token } = useAuth();

  const { data, isLoading } = useQuery({
    queryKey: ["dashboard-overview"],
    queryFn: () => apiRequest<DashboardOverview>("/dashboard/overview", {}, token),
    enabled: !!token
  });

  return (
    <div>
      <Topbar title="Dashboard" subtitle="查看文档处理情况、会话规模和最近 7 天的 Token 消耗走势。" />

      {isLoading ? <p className="text-sm text-ink/60">正在加载仪表盘...</p> : null}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {statCards.map((item) => {
          const Icon = item.icon;
          const value = data?.[item.key] ?? 0;
          return (
            <div key={item.key} className="mesh-card rounded-[1.75rem] border border-line p-5 shadow-panel">
              <div className="flex items-center justify-between">
                <p className="text-sm text-ink/60">{item.label}</p>
                <Icon className="h-5 w-5 text-accent" />
              </div>
              <p className="mt-4 text-4xl font-bold text-ink">{formatNumber(Number(value))}</p>
            </div>
          );
        })}
      </div>

      <section className="mt-6 mesh-card rounded-[1.75rem] border border-line p-6 shadow-panel">
        <div className="mb-4">
          <h2 className="text-2xl font-bold">最近 7 天 Token 趋势</h2>
          <p className="mt-2 text-sm text-ink/60">用于观察问答活跃度和模型调用规模。</p>
        </div>
        <div className="h-[320px]">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data?.daily_tokens || []}>
              <XAxis dataKey="date" tick={{ fontSize: 12 }} />
              <YAxis tick={{ fontSize: 12 }} />
              <Tooltip />
              <Line type="monotone" dataKey="tokens" stroke="#db6b2d" strokeWidth={3} dot={{ r: 4 }} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </section>
    </div>
  );
}
