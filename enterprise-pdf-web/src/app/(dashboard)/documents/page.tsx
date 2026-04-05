"use client";

import { Topbar } from "@/components/layout/topbar";
import { Button } from "@/components/ui/button";
import { apiRequest } from "@/lib/api";
import { formatDate, formatNumber } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import type { DocumentItem } from "@/types/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2, UploadCloud } from "lucide-react";
import { ChangeEvent, useMemo, useState } from "react";

function badgeClass(status: DocumentItem["status"]) {
  if (status === "completed") return "bg-mint/10 text-mint";
  if (status === "failed") return "bg-red-100 text-red-600";
  return "bg-gold/20 text-amber-700";
}

export default function DocumentsPage() {
  const { token } = useAuth();
  const queryClient = useQueryClient();
  const [error, setError] = useState("");

  const documentsQuery = useQuery({
    queryKey: ["documents"],
    queryFn: () => apiRequest<{ items: DocumentItem[] }>("/documents", {}, token),
    enabled: !!token,
    refetchInterval: (query) => {
      const items = query.state.data?.items || [];
      return items.some((item) => item.status === "processing") ? 3000 : false;
    }
  });

  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData();
      formData.append("file", file);
      return apiRequest<DocumentItem>("/documents", { method: "POST", body: formData }, token);
    },
    onSuccess: () => {
      setError("");
      queryClient.invalidateQueries({ queryKey: ["documents"] });
      queryClient.invalidateQueries({ queryKey: ["dashboard-overview"] });
    },
    onError: (err) => setError(err instanceof Error ? err.message : "上传失败")
  });

  const deleteMutation = useMutation({
    mutationFn: async (documentId: string) => apiRequest<void>(`/documents/${documentId}`, { method: "DELETE" }, token),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["documents"] });
      queryClient.invalidateQueries({ queryKey: ["dashboard-overview"] });
    }
  });

  async function handleUpload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }
    await uploadMutation.mutateAsync(file);
    event.target.value = "";
  }

  const items = useMemo(() => documentsQuery.data?.items || [], [documentsQuery.data]);

  return (
    <div>
      <Topbar title="Documents" subtitle="上传 PDF，轮询处理状态，并在文档完成向量化后立即进入问答流程。" />

      <section className="mesh-card rounded-[1.75rem] border border-dashed border-line p-6 shadow-panel">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 className="text-2xl font-bold">上传新文档</h2>
            <p className="mt-2 text-sm text-ink/60">支持 PDF 文件，上传后会异步切片、Embedding 并写入向量索引。</p>
          </div>
          <label className="inline-flex cursor-pointer items-center gap-2 rounded-full bg-ink px-5 py-3 text-sm font-semibold text-white transition hover:translate-y-[-1px]">
            <UploadCloud className="h-4 w-4" />
            {uploadMutation.isPending ? "上传中..." : "选择 PDF"}
            <input type="file" accept=".pdf" className="hidden" onChange={handleUpload} />
          </label>
        </div>
        {error ? <p className="mt-4 text-sm text-red-600">{error}</p> : null}
      </section>

      <section className="mt-6 overflow-hidden rounded-[1.75rem] border border-line bg-white/70 shadow-panel">
        <div className="border-b border-line px-6 py-4">
          <h2 className="text-xl font-bold">文档列表</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-panel">
              <tr>
                <th className="px-6 py-4">文件名</th>
                <th className="px-6 py-4">状态</th>
                <th className="px-6 py-4">大小</th>
                <th className="px-6 py-4">切片数</th>
                <th className="px-6 py-4">更新时间</th>
                <th className="px-6 py-4">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id} className="border-t border-line/70">
                  <td className="px-6 py-4">
                    <p className="font-medium text-ink">{item.file_name}</p>
                    {item.error_message ? <p className="mt-1 text-xs text-red-600">{item.error_message}</p> : null}
                  </td>
                  <td className="px-6 py-4">
                    <span className={`rounded-full px-3 py-1 text-xs font-semibold ${badgeClass(item.status)}`}>{item.status}</span>
                  </td>
                  <td className="px-6 py-4">{formatNumber(Math.round(item.file_size / 1024))} KB</td>
                  <td className="px-6 py-4">{formatNumber(item.chunk_count)}</td>
                  <td className="px-6 py-4">{formatDate(item.updated_at)}</td>
                  <td className="px-6 py-4">
                    <Button
                      className="bg-red-600 px-3 py-2 text-xs"
                      onClick={() => deleteMutation.mutate(item.id)}
                      disabled={deleteMutation.isPending}
                    >
                      <Trash2 className="mr-1 h-4 w-4" />
                      删除
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
