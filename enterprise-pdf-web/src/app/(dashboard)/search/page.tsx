"use client";

import { Topbar } from "@/components/layout/topbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiRequest } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import type { ChatSource } from "@/types/api";
import { useMutation } from "@tanstack/react-query";
import { SearchIcon } from "lucide-react";
import { FormEvent, useState } from "react";

export default function SearchPage() {
  const { token } = useAuth();
  const [query, setQuery] = useState("");

  const searchMutation = useMutation({
    mutationFn: (submittedQuery: string) =>
      apiRequest<{ query: string; items: ChatSource[] }>(`/search?q=${encodeURIComponent(submittedQuery)}`, {}, token)
  });

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!query.trim()) {
      return;
    }
    await searchMutation.mutateAsync(query.trim());
  }

  const items = searchMutation.data?.items || [];

  return (
    <div>
      <Topbar title="Search" subtitle="对整个个人知识库执行语义搜索，先验证召回结果，再进入问答。" />

      <form className="mesh-card flex flex-wrap items-center gap-3 rounded-[1.75rem] border border-line p-4 shadow-panel" onSubmit={handleSubmit}>
        <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="输入一个你想检索的主题或问题" />
        <Button type="submit" className="bg-accent" disabled={searchMutation.isPending}>
          <SearchIcon className="mr-2 h-4 w-4" />
          {searchMutation.isPending ? "检索中..." : "开始搜索"}
        </Button>
      </form>

      <div className="mt-6 space-y-4">
        {items.map((item) => (
          <article key={`${item.document_id}-${item.chunk_index}`} className="rounded-[1.5rem] border border-line bg-white/75 p-5 shadow-panel">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-3 text-xs text-ink/55">
              <span>{item.file_name || "Unknown file"}</span>
              <span>Score {item.score.toFixed(3)}</span>
            </div>
            <p className="text-sm leading-7 text-ink/80">{item.content}</p>
          </article>
        ))}
        {searchMutation.isSuccess && !items.length ? (
          <div className="rounded-[1.5rem] border border-dashed border-line bg-panel p-8 text-center text-sm text-ink/60">
            没有找到结果，试试换一种表述或者先上传更多文档。
          </div>
        ) : null}
      </div>
    </div>
  );
}
