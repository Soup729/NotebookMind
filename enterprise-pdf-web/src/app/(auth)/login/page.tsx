"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiRequest } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import type { AuthResponse } from "@/types/api";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

export default function LoginPage() {
  const router = useRouter();
  const { login } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");

    try {
      const response = await apiRequest<AuthResponse>("/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password })
      });
      login(response);
      router.push("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="glass-panel w-full max-w-md rounded-[2rem] border border-line p-8 shadow-panel">
      <p className="text-xs font-semibold uppercase tracking-[0.32em] text-accent">Sign in</p>
      <h1 className="mt-3 text-4xl font-bold">登录企业问答工作台</h1>
      <p className="mt-3 text-sm leading-6 text-ink/65">输入你的账号后，即可进入文档管理、会话和用量面板。</p>

      <form className="mt-8 space-y-4" onSubmit={handleSubmit}>
        <Input type="email" placeholder="you@company.com" value={email} onChange={(e) => setEmail(e.target.value)} />
        <Input type="password" placeholder="至少 8 位密码" value={password} onChange={(e) => setPassword(e.target.value)} />
        {error ? <p className="text-sm text-red-600">{error}</p> : null}
        <Button type="submit" className="w-full" disabled={loading}>
          {loading ? "登录中..." : "登录"}
        </Button>
      </form>

      <p className="mt-6 text-sm text-ink/65">
        还没有账号？{" "}
        <Link href="/register" className="font-semibold text-accent">
          立即注册
        </Link>
      </p>
    </section>
  );
}
