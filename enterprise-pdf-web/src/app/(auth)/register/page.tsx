"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiRequest } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import type { AuthResponse } from "@/types/api";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

export default function RegisterPage() {
  const router = useRouter();
  const { login } = useAuth();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");

    try {
      const response = await apiRequest<AuthResponse>("/auth/register", {
        method: "POST",
        body: JSON.stringify({ name, email, password })
      });
      login(response);
      router.push("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "注册失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="glass-panel w-full max-w-md rounded-[2rem] border border-line p-8 shadow-panel">
      <p className="text-xs font-semibold uppercase tracking-[0.32em] text-mint">Create account</p>
      <h1 className="mt-3 text-4xl font-bold">创建企业账号</h1>
      <p className="mt-3 text-sm leading-6 text-ink/65">注册后即可开始上传 PDF、创建问答会话并查看用量数据。</p>

      <form className="mt-8 space-y-4" onSubmit={handleSubmit}>
        <Input placeholder="你的名字" value={name} onChange={(e) => setName(e.target.value)} />
        <Input type="email" placeholder="you@company.com" value={email} onChange={(e) => setEmail(e.target.value)} />
        <Input type="password" placeholder="至少 8 位密码" value={password} onChange={(e) => setPassword(e.target.value)} />
        {error ? <p className="text-sm text-red-600">{error}</p> : null}
        <Button type="submit" className="w-full bg-mint" disabled={loading}>
          {loading ? "注册中..." : "注册并进入"}
        </Button>
      </form>

      <p className="mt-6 text-sm text-ink/65">
        已有账号？{" "}
        <Link href="/login" className="font-semibold text-accent">
          去登录
        </Link>
      </p>
    </section>
  );
}
