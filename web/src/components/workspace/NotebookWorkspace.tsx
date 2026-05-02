'use client';

import { useCallback } from 'react';
import dynamic from 'next/dynamic';
import {
  BookOpen,
  Brain,
  Download,
  FileText,
  GitBranch,
  Layers,
  Loader2,
  RefreshCw,
  Sparkles,
  Trash2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { formatDate } from '@/lib/utils';
import { useSessionMemory } from '@/hooks/useSessionMemory';
import { useNotes } from '@/hooks/useNotes';
import { useGenerateNotebookArtifact, useNotebookArtifacts } from '@/hooks/useNotebookExports';
import type { Document, ExportFormat, Session } from '@/types/api';

const KnowledgeGraphPanel = dynamic(
  () => import('@/components/workspace/KnowledgeGraphPanel').then((mod) => mod.KnowledgeGraphPanel),
  {
    ssr: false,
    loading: () => <GraphIslandLoading />,
  }
);

interface NotebookWorkspaceProps {
  notebookId: string;
  session: Session | null;
  documents: Document[];
  selectedDocumentIds: string[];
  onOpenExport: (format: ExportFormat, requirements?: string) => void;
}

const artifactActions = [
  { type: 'briefing', label: 'Briefing', icon: FileText },
  { type: 'comparison', label: '跨文档对比', icon: GitBranch },
  { type: 'timeline', label: '时间线', icon: Layers },
  { type: 'study_pack', label: '学习包', icon: BookOpen },
];

const exportActions: Array<{ format: ExportFormat; label: string }> = [
  { format: 'markdown', label: 'Markdown' },
  { format: 'mindmap', label: '思维导图' },
  { format: 'docx', label: 'Word' },
  { format: 'pptx', label: 'PPT' },
  { format: 'pdf', label: 'PDF' },
];

export function NotebookWorkspace({
  notebookId,
  session,
  documents,
  selectedDocumentIds,
  onOpenExport,
}: NotebookWorkspaceProps) {
  const completedDocs = documents.filter((doc) => doc.status === 'completed');
  const selectedCount = selectedDocumentIds.length || completedDocs.length;
  const { memory, isLoading: memoryLoading, refreshMemory, clearMemory } = useSessionMemory(
    notebookId,
    session?.id || null
  );
  const { notes, isLoading: notesLoading } = useNotes({ notebook_id: notebookId, page_size: 5 });
  const { artifacts, isLoading: artifactsLoading, mutate: mutateArtifacts } = useNotebookArtifacts(notebookId);
  const { generateArtifact } = useGenerateNotebookArtifact(notebookId);

  const handleGenerateArtifact = useCallback(
    async (type: string) => {
      const artifact = await generateArtifact(type);
      if (artifact) {
        await mutateArtifacts();
      }
    },
    [generateArtifact, mutateArtifacts]
  );

  return (
    <div className="h-full overflow-auto custom-scrollbar p-6">
      <div className="mx-auto max-w-6xl space-y-5">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-2xl font-semibold">Notebook 工作台</h2>
            <p className="text-sm text-muted-foreground">
              {completedDocs.length} 个可用文档 · {selectedCount} 个当前范围 · {session?.title || '未选择会话'}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {exportActions.map((action) => (
              <Button
                key={action.format}
                variant="outline"
                size="sm"
                onClick={() => onOpenExport(action.format)}
                disabled={completedDocs.length === 0}
                className="gap-2"
              >
                <Download className="h-4 w-4" />
                {action.label}
              </Button>
            ))}
          </div>
        </div>

        <section className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
          <div className="rounded-lg border bg-background p-4">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <Brain className="h-4 w-4 text-primary" />
                <h3 className="font-medium">长会话记忆</h3>
              </div>
              <div className="flex gap-1">
                <Button variant="ghost" size="icon" title="刷新记忆" onClick={refreshMemory} disabled={!session}>
                  <RefreshCw className="h-4 w-4" />
                </Button>
                <Button variant="ghost" size="icon" title="清空记忆" onClick={clearMemory} disabled={!session}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>

            {memoryLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-4 w-3/4" />
                <Skeleton className="h-4 w-1/2" />
              </div>
            ) : memory?.summary ? (
              <div className="space-y-3 text-sm">
                {memory.goal && <p><span className="font-medium">目标：</span>{memory.goal}</p>}
                <p className="text-muted-foreground">{memory.summary}</p>
                <MemoryList title="决策" items={memory.decisions} />
                <MemoryList title="待确认" items={memory.open_questions} />
                <MemoryList title="偏好" items={memory.preferences} />
                {memory.updated_at && (
                  <p className="text-xs text-muted-foreground">更新于 {formatDate(memory.updated_at)}</p>
                )}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                当前会话还没有可用记忆。继续对话后会自动压缩，也可以手动刷新。
              </p>
            )}
          </div>

          <div className="rounded-lg border bg-background p-4">
            <div className="mb-3 flex items-center gap-2">
              <Sparkles className="h-4 w-4 text-primary" />
              <h3 className="font-medium">一键研究产物</h3>
            </div>
            <div className="grid grid-cols-2 gap-2">
              {artifactActions.map((action) => {
                const Icon = action.icon;
                return (
                  <Button
                    key={action.type}
                    variant="secondary"
                    size="sm"
                    onClick={() => handleGenerateArtifact(action.type)}
                    disabled={completedDocs.length === 0}
                    className="justify-start gap-2"
                  >
                    <Icon className="h-4 w-4" />
                    {action.label}
                  </Button>
                );
              })}
            </div>
          </div>
        </section>

        <section className="grid gap-4 lg:grid-cols-2">
          <div className="rounded-lg border bg-background p-4">
            <h3 className="mb-3 font-medium">最近产物</h3>
            {artifactsLoading ? (
              <LoadingRows />
            ) : artifacts.length > 0 ? (
              <div className="space-y-2">
                {artifacts.slice(0, 6).map((artifact) => (
                  <div key={artifact.id} className="flex items-center justify-between gap-3 rounded-md bg-muted/40 px-3 py-2">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{artifact.title}</p>
                      <p className="text-xs text-muted-foreground">{artifact.type} · {artifact.status}</p>
                    </div>
                    {artifact.status === 'generating' && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">还没有工作台产物。</p>
            )}
          </div>

          <div className="rounded-lg border bg-background p-4">
            <h3 className="mb-3 font-medium">最近笔记</h3>
            {notesLoading ? (
              <LoadingRows />
            ) : notes.length > 0 ? (
              <div className="space-y-2">
                {notes.map((note) => (
                  <div key={note.id} className="rounded-md bg-muted/40 px-3 py-2">
                    <p className="truncate text-sm font-medium">{note.title}</p>
                    <p className="line-clamp-2 text-xs text-muted-foreground">{note.content}</p>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">保存 AI 回答或摘录后会出现在这里。</p>
            )}
          </div>
        </section>

        <KnowledgeGraphPanel notebookId={notebookId} />
      </div>
    </div>
  );
}

function MemoryList({ title, items }: { title: string; items?: string[] }) {
  if (!items?.length) return null;
  return (
    <div>
      <p className="mb-1 text-xs font-medium text-muted-foreground">{title}</p>
      <ul className="space-y-1 text-sm">
        {items.slice(0, 4).map((item) => (
          <li key={item} className="rounded-md bg-muted/40 px-2 py-1">{item}</li>
        ))}
      </ul>
    </div>
  );
}

function GraphIslandLoading() {
  return (
    <section className="rounded-lg border bg-background p-4">
      <div className="mb-3 flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        <p className="text-sm text-muted-foreground">正在加载知识图谱...</p>
      </div>
      <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <Skeleton className="h-[420px] w-full rounded-md" />
        <div className="space-y-2 rounded-md border p-3">
          <Skeleton className="h-4 w-1/3" />
          <Skeleton className="h-6 w-2/3" />
          <Skeleton className="h-16 w-full" />
        </div>
      </div>
    </section>
  );
}

function LoadingRows() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-10 w-4/5" />
      <Skeleton className="h-10 w-3/5" />
    </div>
  );
}
