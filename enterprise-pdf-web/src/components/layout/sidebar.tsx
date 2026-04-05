"use client";

import { useAuth } from "@/providers/auth-provider";
import { BarChart3, Files, LogOut, MessageSquare, Search, Sparkles } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

const items = [
  { href: "/dashboard", label: "Dashboard", icon: BarChart3 },
  { href: "/documents", label: "Documents", icon: Files },
  { href: "/chat", label: "Chat", icon: MessageSquare },
  { href: "/search", label: "Search", icon: Search },
  { href: "/usage", label: "Usage", icon: Sparkles }
];

export function Sidebar() {
  const pathname = usePathname();
  const router = useRouter();
  const { logout, user } = useAuth();

  return (
    <aside className="glass-panel flex h-full w-full max-w-xs flex-col rounded-[2rem] border border-white/50 bg-ink p-5 text-white shadow-panel">
      <div className="mb-8 rounded-[1.5rem] border border-white/10 bg-white/5 p-4">
        <p className="text-xs uppercase tracking-[0.35em] text-white/55">Workspace</p>
        <h2 className="mt-3 text-2xl font-bold">Enterprise PDF AI</h2>
        <p className="mt-2 text-sm text-white/65">RAG 工作台、文档资产库与企业问答面板。</p>
      </div>

      <nav className="space-y-2">
        {items.map((item) => {
          const Icon = item.icon;
          const active = pathname === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`flex items-center gap-3 rounded-2xl px-4 py-3 text-sm transition ${
                active ? "bg-white text-ink" : "text-white/75 hover:bg-white/10 hover:text-white"
              }`}
            >
              <Icon className="h-4 w-4" />
              <span>{item.label}</span>
            </Link>
          );
        })}
      </nav>

      <div className="mt-auto rounded-[1.5rem] border border-white/10 bg-white/5 p-4">
        <div className="mb-4">
          <p className="text-sm font-semibold">{user?.name || "Unknown user"}</p>
          <p className="text-xs text-white/60">{user?.email || "Not signed in"}</p>
        </div>
        <button
          className="flex w-full items-center justify-center gap-2 rounded-full border border-white/20 px-4 py-2 text-sm text-white transition hover:bg-white/10"
          onClick={() => {
            logout();
            router.push("/login");
          }}
        >
          <LogOut className="h-4 w-4" />
          退出登录
        </button>
      </div>
    </aside>
  );
}
