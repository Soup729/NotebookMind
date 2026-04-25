'use client';

import { Download, Loader2, X, XCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useExportArtifact, useNotebookExports } from '@/hooks/useNotebookExports';

interface ExportTaskTrayProps {
  notebookId: string;
  artifactId: string | null;
  onClear: () => void;
}

export function ExportTaskTray({ notebookId, artifactId, onClear }: ExportTaskTrayProps) {
  const { artifact } = useExportArtifact(notebookId, artifactId);
  const { downloadExport } = useNotebookExports(notebookId);

  if (!artifactId || !artifact) return null;

  const isGenerating = artifact.status === 'generating';
  const isCompleted = artifact.status === 'completed';
  const isFailed = artifact.status === 'failed';

  return (
    <div className="fixed right-4 bottom-4 z-40 w-80 rounded-lg border bg-background shadow-lg p-3">
      <div className="flex items-start gap-3">
        <div className="pt-0.5">
          {isGenerating && <Loader2 className="w-4 h-4 animate-spin text-primary" />}
          {isCompleted && <Download className="w-4 h-4 text-green-600" />}
          {isFailed && <XCircle className="w-4 h-4 text-destructive" />}
        </div>
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium truncate">{artifact.title || '导出任务'}</div>
          <div className="text-xs text-muted-foreground mt-0.5">
            {isGenerating && '正在生成，可以继续聊天或浏览文档'}
            {isCompleted && '生成完成，可以下载'}
            {isFailed && (artifact.error_msg || '生成失败')}
          </div>
          {isCompleted && (
            <Button size="sm" className="mt-2 gap-2" onClick={() => downloadExport(artifact)}>
              <Download className="w-4 h-4" />
              下载
            </Button>
          )}
        </div>
        <Button variant="ghost" size="icon" className="h-7 w-7" onClick={onClear} title="关闭">
          <X className="w-4 h-4" />
        </Button>
      </div>
    </div>
  );
}
