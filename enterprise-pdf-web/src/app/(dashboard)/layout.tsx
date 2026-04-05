"use client";

import { Sidebar } from "@/components/layout/sidebar";
import { useAuth } from "@/providers/auth-provider";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const { ready, token } = useAuth();

  useEffect(() => {
    if (ready && !token) {
      router.replace("/login");
    }
  }, [ready, router, token]);

  if (!ready) {
    return <div className="flex min-h-screen items-center justify-center text-sm text-ink/60">加载中...</div>;
  }

  if (!token) {
    return null;
  }

  return (
    <div className="mx-auto grid min-h-screen max-w-[1600px] gap-6 px-4 py-4 lg:grid-cols-[280px_1fr]">
      <Sidebar />
      <div className="glass-panel rounded-[2rem] border border-line p-6 shadow-panel">{children}</div>
    </div>
  );
}
