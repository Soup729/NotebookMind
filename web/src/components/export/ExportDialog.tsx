'use client';

import { useEffect, useMemo, useState } from 'react';
import { Loader2, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { ExportOutlineEditor } from './ExportOutlineEditor';
import { useNotebookExports } from '@/hooks/useNotebookExports';
import type {
  Document,
  ExportFormat,
  ExportOutlineRequest,
  ExportOutlineSection,
  NotebookArtifact,
} from '@/types/api';

interface ExportDialogProps {
  open: boolean;
  notebookId: string;
  documents: Document[];
  selectedDocumentIds: string[];
  initialFormat: ExportFormat;
  initialRequirements?: string;
  onClose: () => void;
  onTaskStart: (artifact: NotebookArtifact) => void;
}

const FORMAT_LABELS: Record<ExportFormat, string> = {
  markdown: 'Markdown',
  mindmap: '思维导图',
  docx: 'Word',
  pptx: 'PPT',
  pdf: 'PDF',
};

function defaultLength(format: ExportFormat) {
  if (format === 'pptx') return '8-10 slides';
  if (format === 'docx' || format === 'pdf') return '3-5 pages';
  return 'concise';
}

function defaultStyle(format: ExportFormat) {
  if (format === 'pptx') return '简洁商务';
  if (format === 'docx') return '正式报告';
  if (format === 'pdf') return '正式阅读版';
  if (format === 'mindmap') return '层级清晰';
  return '结构化研究笔记';
}

export function ExportDialog({
  open,
  notebookId,
  documents,
  selectedDocumentIds,
  initialFormat,
  initialRequirements,
  onClose,
  onTaskStart,
}: ExportDialogProps) {
  const completedDocs = useMemo(() => documents.filter((doc) => doc.status === 'completed'), [documents]);
  const defaultDocIds = useMemo(() => {
    const completedIds = new Set(completedDocs.map((doc) => doc.id));
    const selectedCompleted = selectedDocumentIds.filter((id) => completedIds.has(id));
    return selectedCompleted.length > 0 ? selectedCompleted : completedDocs.map((doc) => doc.id);
  }, [completedDocs, selectedDocumentIds]);

  const [format, setFormat] = useState<ExportFormat>(initialFormat);
  const [documentIds, setDocumentIds] = useState<string[]>(defaultDocIds);
  const [language, setLanguage] = useState('中文');
  const [style, setStyle] = useState(defaultStyle(initialFormat));
  const [length, setLength] = useState(defaultLength(initialFormat));
  const [requirements, setRequirements] = useState(initialRequirements || '');
  const [includeCitations, setIncludeCitations] = useState(true);
  const [artifact, setArtifact] = useState<NotebookArtifact | null>(null);
  const [outline, setOutline] = useState<ExportOutlineSection[]>([]);
  const [isCreatingOutline, setIsCreatingOutline] = useState(false);
  const [isConfirming, setIsConfirming] = useState(false);

  const { createOutline, confirmExport } = useNotebookExports(notebookId);

  useEffect(() => {
    if (!open) return;
    setFormat(initialFormat);
    setDocumentIds(defaultDocIds);
    setLanguage('中文');
    setStyle(defaultStyle(initialFormat));
    setLength(defaultLength(initialFormat));
    setRequirements(initialRequirements || '');
    setIncludeCitations(true);
    setArtifact(null);
    setOutline([]);
  }, [open, initialFormat, initialRequirements, defaultDocIds]);

  if (!open) return null;

  const handleCreateOutline = async () => {
    if (documentIds.length === 0) {
      toast.error('请选择至少一个已完成文档');
      return;
    }
    setIsCreatingOutline(true);
    const request: ExportOutlineRequest = {
      format,
      document_ids: documentIds,
      language,
      style,
      length,
      requirements,
      include_citations: includeCitations,
    };
    const nextArtifact = await createOutline(request);
    setIsCreatingOutline(false);
    if (nextArtifact?.content?.outline) {
      setArtifact(nextArtifact);
      setOutline(nextArtifact.content.outline);
    }
  };

  const handleConfirm = async () => {
    if (!artifact) return;
    setIsConfirming(true);
    const nextArtifact = await confirmExport(artifact.id, { outline });
    setIsConfirming(false);
    if (nextArtifact) {
      onTaskStart(nextArtifact);
      onClose();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="w-full max-w-3xl max-h-[86vh] overflow-hidden rounded-lg border bg-background shadow-lg flex flex-col">
        <div className="h-12 px-4 border-b flex items-center justify-between">
          <div className="font-semibold">导出 {FORMAT_LABELS[format]}</div>
          <Button variant="ghost" size="icon" className="h-8 w-8" onClick={onClose}>
            <X className="w-4 h-4" />
          </Button>
        </div>

        <div className="flex-1 overflow-auto p-4 space-y-4">
          {!artifact ? (
            <>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <label className="space-y-1 text-sm">
                  <span>语言</span>
                  <Input value={language} onChange={(event) => setLanguage(event.target.value)} />
                </label>
                <label className="space-y-1 text-sm">
                  <span>长度 / 页数</span>
                  <Input value={length} onChange={(event) => setLength(event.target.value)} />
                </label>
                <label className="space-y-1 text-sm sm:col-span-2">
                  <span>风格</span>
                  <Input value={style} onChange={(event) => setStyle(event.target.value)} />
                </label>
              </div>

              <label className="space-y-1 text-sm block">
                <span>具体要求</span>
                <Textarea
                  value={requirements}
                  onChange={(event) => setRequirements(event.target.value)}
                  placeholder="例如：面向管理层，突出风险、结论和下一步行动。"
                  className="min-h-[110px]"
                />
              </label>

              <div className="space-y-2">
                <div className="text-sm font-medium">文档范围</div>
                <div className="max-h-36 overflow-auto border rounded-md p-2 space-y-2">
                  {completedDocs.map((doc) => (
                    <label key={doc.id} className="flex items-center gap-2 text-sm">
                      <Checkbox
                        checked={documentIds.includes(doc.id)}
                        onCheckedChange={() => {
                          setDocumentIds((prev) =>
                            prev.includes(doc.id) ? prev.filter((id) => id !== doc.id) : [...prev, doc.id]
                          );
                        }}
                      />
                      <span className="truncate">{doc.file_name}</span>
                    </label>
                  ))}
                </div>
              </div>

              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={includeCitations}
                  onCheckedChange={(checked) => setIncludeCitations(Boolean(checked))}
                />
                包含引用来源
              </label>
            </>
          ) : (
            <ExportOutlineEditor outline={outline} onChange={setOutline} />
          )}
        </div>

        <div className="p-4 border-t flex justify-end gap-2">
          {artifact ? (
            <>
              <Button variant="outline" onClick={() => setArtifact(null)} disabled={isConfirming}>
                返回修改要求
              </Button>
              <Button onClick={handleConfirm} disabled={isConfirming || outline.length === 0}>
                {isConfirming && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                确认并生成
              </Button>
            </>
          ) : (
            <Button onClick={handleCreateOutline} disabled={isCreatingOutline || completedDocs.length === 0}>
              {isCreatingOutline && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
              生成大纲
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
