// ============================================================
// NotebookMind - 笔记本工作台页面 (核心)
// ============================================================

'use client';

import React, { useState, useCallback, useEffect, useRef } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { ArrowLeft, Plus, Loader2, GripHorizontal, MessageSquare, Trash2, ChevronDown, Pencil, Check, X, LayoutDashboard } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

// 布局组件
import { SourcesPanel } from '@/components/layout/SourcesPanel';
import { NotesPanel } from '@/components/layout/NotesPanel';
import { ChatPanel } from '@/components/chat/ChatPanel';
import { ExportDialog } from '@/components/export/ExportDialog';
import { ExportFormatMenu } from '@/components/export/ExportFormatMenu';
import { ExportTaskTray } from '@/components/export/ExportTaskTray';
import { NotebookWorkspace } from '@/components/workspace/NotebookWorkspace';

// PDF 查看器
import { PdfViewer } from '@/components/pdf/PdfViewer';

// 指南组件
import { DocumentGuide } from '@/components/guide/DocumentGuide';

// Hooks
import { useNotebook } from '@/hooks/useNotebook';
import { useDocuments, useUploadDocument, useDeleteDocument, useUpdateNotebook } from '@/hooks/useNotebook';
import { useSessions, useCreateSession, useDeleteSession } from '@/hooks/useNotebook';
import { useNotebookStore } from '@/store/useNotebookStore';

import type { Session, Document, ExportFormat, NotebookArtifact } from '@/types/api';

// ============================================================
// 指南视图组件
// ============================================================

interface GuideViewProps {
  notebookId: string;
  documents: Document[];
  /** 点击建议问题时的回调 */
  onSuggestedQueryClick?: (query: string) => void;
}

function GuideView({ notebookId, documents, onSuggestedQueryClick }: GuideViewProps) {
  const completedDocs = documents.filter((d) => d.status === 'completed');

  if (completedDocs.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-center p-8">
        <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center mb-4">
          <svg
            className="w-8 h-8 text-muted-foreground"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium mb-2">准备开始</h3>
        <p className="text-sm text-muted-foreground max-w-md">
          上传 PDF 文档后，我将自动生成摘要、FAQ 和关键要点，帮助你快速了解文档内容。
        </p>
      </div>
    );
  }

  // 显示第一个已完成的文档指南
  return (
    <div className="h-full overflow-auto custom-scrollbar p-6">
      <div className="max-w-4xl mx-auto space-y-6">
        <h2 className="text-2xl font-semibold mb-4">文档指南</h2>
        {completedDocs.map((doc) => (
          <DocumentGuide
            key={doc.id}
            notebookId={notebookId}
            documentId={doc.id}
            onSuggestedQueryClick={onSuggestedQueryClick}
          />
        ))}
      </div>
    </div>
  );
}

// ============================================================
// 笔记本工作台页面
// ============================================================

export default function NotebookPage() {
  const params = useParams();
  const router = useRouter();
  const notebookId = params.id as string;

  // 状态
  const [notesPanelOpen, setNotesPanelOpen] = useState(false);

  // Store
  const {
    mainView,
    activePdfId,
    selectedDocumentIds,
    highlightTarget,
    activeSessionId: storedSessionId,
    _hasHydrated,
    currentNotebookId: storedNotebookId,
    setNotebookAndSession,
    setActiveSession,
    setSelectedDocuments,
    toggleDocumentSelection,
    setMainViewToPdf,
    setMainViewToGuide,
    setMainViewToWorkspace,
  } = useNotebookStore();

  // 数据获取
  const { notebook, isLoading: notebookLoading } = useNotebook(notebookId);
  const { documents, isLoading: docsLoading } = useDocuments(notebookId);
  const { sessions, isLoading: sessionsLoading } = useSessions(notebookId);

  // 操作
  const { uploadDocument } = useUploadDocument();
  const { deleteDocument } = useDeleteDocument();
  const { createSession } = useCreateSession();
  const { deleteSession } = useDeleteSession();
  const { updateNotebook } = useUpdateNotebook();

  // 当前会话
  const [currentSession, setCurrentSession] = useState<Session | null>(null);
  // 防止重复创建会话的锁
  const isCreatingRef = useRef(false);

  // 从文档指南点击的建议问题（传递给 ChatPanel 填充输入框）
  const [pendingSuggestedQuery, setPendingSuggestedQuery] = useState<string | null>(null);

  // 笔记本改名状态
  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);

  // 导出状态
  const [exportDialogOpen, setExportDialogOpen] = useState(false);
  const [exportFormat, setExportFormat] = useState<ExportFormat>('markdown');
  const [exportRequirements, setExportRequirements] = useState('');
  const [activeExportArtifactId, setActiveExportArtifactId] = useState<string | null>(null);

  // ChatPanel 高度拖动调整 (带记忆功能)
  const [chatHeight, setChatHeight] = useState(() => {
    if (typeof window !== 'undefined') {
      const saved = localStorage.getItem('chatPanelHeight');
      if (saved) {
        const parsed = parseInt(saved, 10);
        if (!isNaN(parsed) && parsed >= 250 && parsed <= 700) {
          return parsed;
        }
      }
    }
    return 500; // 默认 500px
  });
  const [isDragging, setIsDragging] = useState(false);
  const dragStartY = useRef(0);
  const dragStartHeight = useRef(0);
  const containerRef = useRef<HTMLDivElement>(null);

  const handleDragStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setIsDragging(true);
    dragStartY.current = e.clientY;
    dragStartHeight.current = chatHeight;
  }, [chatHeight]);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging) return;
      const delta = dragStartY.current - e.clientY;
      const newHeight = dragStartHeight.current + delta;
      const clampedHeight = Math.max(250, Math.min(newHeight, 700));
      setChatHeight(clampedHeight);
      localStorage.setItem('chatPanelHeight', String(clampedHeight));
    };

    const handleMouseUp = () => {
      setIsDragging(false);
    };

    if (isDragging) {
      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isDragging]);

  // 初始化：设置当前笔记本（保留属于当前 notebook 的 activeSessionId）
  // 关键：必须等 _hasHydrated=true 后再判断，否则 storedNotebookId 还是初始值 null，
  // 导致每次进入页面都错误地认为"切换了新笔记本"，从而把 activeSessionId 重置为 null
  useEffect(() => {
    if (notebookId && _hasHydrated) {
      if (storedNotebookId !== notebookId) {
        // 切换到新笔记本时，只保留属于当前 notebook 的 session ID
        // 这样后续的 session 恢复逻辑能正确从 storedSessionId 恢复
        const sessionForThisNotebook = (storedNotebookId === notebookId ? storedSessionId : undefined) || undefined;
        setNotebookAndSession(notebookId, sessionForThisNotebook);
      }
    }
  }, [notebookId, storedNotebookId, storedSessionId, _hasHydrated, setNotebookAndSession]);

  // 文档选择由用户手动控制，不再自动全选
  // 如需恢复自动选中，取消下方注释即可：
  // useEffect(() => {
  //   const completedIds = documents
  //     .filter((d) => d.status === 'completed')
  //     .map((d) => d.id);
  //   if (completedIds.length > 0 && selectedDocumentIds.length === 0) {
  //     setSelectedDocuments(completedIds);
  //   }
  // }, [documents, selectedDocumentIds.length, setSelectedDocuments]);

  // 自动创建或选择会话
  // 依赖 _hasHydrated 确保 Zustand persist 已从 localStorage 恢复
  // 优先级：1. Store 中持久化的 activeSessionId（上次使用的）→ 2. 最近活跃的 session
  useEffect(() => {
    if (sessions.length > 0 && !currentSession && _hasHydrated) {
      // 尝试从 Store 恢复上次使用的 session（且属于当前 notebook）
      if (storedSessionId && storedNotebookId === notebookId) {
        const found = sessions.find((s) => s.id === storedSessionId);
        if (found) {
          console.debug('[NotebookPage] Restored previous session:', storedSessionId);
          setCurrentSession(found);
          return;
        }
        console.debug('[NotebookPage] Stored session not found in sessions list, falling back to latest');
      }

      // 回退：选择最近活跃的会话
      const sorted = [...sessions].sort(
        (a, b) =>
          new Date(b.last_message_at).getTime() -
          new Date(a.last_message_at).getTime()
      );
      setCurrentSession(sorted[0]);
    }
  }, [sessions, currentSession, storedSessionId, storedNotebookId, notebookId, _hasHydrated]);

  // ============================================================
  // 文档操作
  // ============================================================

  const handleUpload = useCallback(
    async (file: File) => {
      const docId = await uploadDocument(notebookId, file);
      if (docId) {
        toast.success(`文档 "${file.name}" 上传成功`);
      }
    },
    [notebookId, uploadDocument]
  );

  const handleRemoveDocument = useCallback(
    async (docId: string) => {
      const success = await deleteDocument(notebookId, docId);
      if (success) {
        toast.success('文档已移除');
        // 取消选中
        if (selectedDocumentIds.includes(docId)) {
          toggleDocumentSelection(docId);
        }
      }
    },
    [notebookId, deleteDocument, selectedDocumentIds, toggleDocumentSelection]
  );

  // ============================================================
  // 会话操作
  // ============================================================

  const handleCreateSession = useCallback(async () => {
    // 防止重复创建
    if (isCreatingRef.current) return;
    isCreatingRef.current = true;

    try {
      const session = await createSession(notebookId, {
        title: `新对话 ${new Date().toLocaleString()}`,
      });
      if (session) {
        setCurrentSession(session);
        setActiveSession(session.id);
        toast.success('新会话已创建');
      }
    } finally {
      // 延迟释放锁，防止快速连续点击
      setTimeout(() => { isCreatingRef.current = false; }, 500);
    }
  }, [notebookId, createSession]);

  const handleDeleteSession = useCallback(
    async (e: React.MouseEvent, sessionId: string) => {
      e.stopPropagation();
      if (!confirm('确定要删除这个对话吗？删除后无法恢复。')) {
        return;
      }
      const success = await deleteSession(notebookId, sessionId);
      if (success && currentSession?.id === sessionId) {
        setCurrentSession(null);
      }
    },
    [notebookId, deleteSession, currentSession]
  );

  const handleSelectSession = useCallback(
    (session: Session) => {
      setCurrentSession(session);
      // 同步到 Store 以便刷新后恢复
      setActiveSession(session.id);
    },
    [setActiveSession]
  );

  // ============================================================
  // 文档选择
  // ============================================================

  const handleSelectionChange = useCallback(
    (ids: string[]) => {
      setSelectedDocuments(ids);
    },
    [setSelectedDocuments]
  );

  // ============================================================
  // 建议问题点击（从文档指南 → ChatPanel 输入框）
  // ============================================================

  const handleSuggestedQueryClick = useCallback((query: string) => {
    setPendingSuggestedQuery(query);
    // 切换到 PDF 视图（如果不在的话）让用户看到输入框
    if (mainView === 'guide') {
      setMainViewToPdf(activePdfId || (documents.length > 0 ? documents[0].id : ''));
    }
  }, [mainView, activePdfId, documents, setMainViewToPdf]);

  // ============================================================
  // 导出
  // ============================================================

  const completedDocuments = documents.filter((doc) => doc.status === 'completed');

  const openExportDialog = useCallback(
    (format: ExportFormat, requirements = '') => {
      if (completedDocuments.length === 0) {
        toast.error('请先上传并等待文档处理完成');
        return;
      }
      setExportFormat(format);
      setExportRequirements(requirements);
      setExportDialogOpen(true);
    },
    [completedDocuments.length]
  );

  const handleExportTaskStart = useCallback((artifact: NotebookArtifact) => {
    setActiveExportArtifactId(artifact.id);
  }, []);

  const handleExportIntent = useCallback(
    (intent: { format: ExportFormat; requirements: string }) => {
      openExportDialog(intent.format, intent.requirements);
    },
    [openExportDialog]
  );

  // ============================================================
  // 笔记本改名
  // ============================================================

  const startRename = useCallback(() => {
    setRenameValue(notebook?.title || '');
    setIsRenaming(true);
    setTimeout(() => renameInputRef.current?.select(), 50);
  }, [notebook?.title]);

  const confirmRename = useCallback(async () => {
    const trimmed = renameValue.trim();
    if (!trimmed || trimmed === notebook?.title) {
      setIsRenaming(false);
      return;
    }
    const ok = await updateNotebook(notebookId, { title: trimmed });
    if (ok) setIsRenaming(false);
  }, [notebookId, notebook?.title, renameValue, updateNotebook]);

  const cancelRename = useCallback(() => {
    setIsRenaming(false);
    setRenameValue('');
  }, []);

  // Enter 键确认改名
  const handleRenameKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        confirmRename();
      } else if (e.key === 'Escape') {
        cancelRename();
      }
    },
    [confirmRename, cancelRename]
  );

  // ============================================================
  // 主视图切换
  // ============================================================

  const handleSourceClick = useCallback(
    (source: { document_id: string; page_number: number; content: string; bounding_box?: [number, number, number, number] }) => {
      setMainViewToPdf(source.document_id, {
        pageNumber: source.page_number,
        boundingBox: source.bounding_box && source.bounding_box.length === 4 ? source.bounding_box : [0, 0, 0, 0],
        sourceId: source.document_id,
        documentId: source.document_id,
        documentName: documents.find((d) => d.id === source.document_id)?.file_name || '',
        content: source.content,
      });
    },
    [setMainViewToPdf, documents]
  );

  // ============================================================
  // 加载状态
  // ============================================================

  if (notebookLoading) {
    return (
      <div className="h-screen w-screen flex items-center justify-center">
        <div className="flex flex-col items-center gap-4">
          <Loader2 className="w-8 h-8 animate-spin text-primary" />
          <p className="text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  if (!notebook) {
    return (
      <div className="h-screen w-screen flex items-center justify-center">
        <div className="flex flex-col items-center gap-4">
          <p className="text-lg font-medium">笔记本不存在</p>
          <Button onClick={() => router.push('/')}>返回首页</Button>
        </div>
      </div>
    );
  }

  // ============================================================
  // 渲染
  // ============================================================

  return (
    <div className="h-screen w-screen flex overflow-hidden bg-background">
      {/* 左侧文档面板 */}
      <SourcesPanel
        notebookId={notebookId}
        documents={documents}
        isLoading={docsLoading}
        selectedIds={selectedDocumentIds}
        onSelectionChange={handleSelectionChange}
        onUpload={handleUpload}
        onRemove={handleRemoveDocument}
        className="flex-shrink-0"
      />

      {/* 中间主视图 */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* 顶部导航栏 */}
        <header className="h-14 flex items-center justify-between px-4 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => router.push('/')}
              className="h-8 w-8"
              title="返回笔记本列表"
            >
              <ArrowLeft className="w-4 h-4" />
            </Button>
            <div>
              {isRenaming ? (
                <div className="flex items-center gap-1">
                  <input
                    ref={renameInputRef}
                    type="text"
                    value={renameValue}
                    onChange={(e) => setRenameValue(e.target.value)}
                    onKeyDown={handleRenameKeyDown}
                    onBlur={confirmRename}
                    className="font-semibold text-sm bg-background border border-primary/50 rounded px-2 py-0.5 outline-none focus:ring-1 focus:ring-primary/30 max-w-[200px]"
                    maxLength={255}
                  />
                  <button onClick={confirmRename} className="p-0.5 hover:bg-muted rounded" title="确认">
                    <Check className="w-3.5 h-3.5 text-green-600" />
                  </button>
                  <button onClick={cancelRename} className="p-0.5 hover:bg-muted rounded" title="取消">
                    <X className="w-3.5 h-3.5 text-muted-foreground" />
                  </button>
                </div>
              ) : (
                <div className="flex items-center gap-1 group">
                  <h1 className="font-semibold text-sm">{notebook.title}</h1>
                  <button
                    onClick={startRename}
                    className="p-0.5 opacity-0 group-hover:opacity-100 hover:opacity-100 transition-opacity"
                    title="重命名笔记本"
                  >
                    <Pencil className="w-3 h-3 text-muted-foreground" />
                  </button>
                </div>
              )}
              <p className="text-xs text-muted-foreground">
                {documents.length} 个文档 · {sessions.length} 个会话
              </p>
            </div>
          </div>

          {/* 会话选择下拉菜单 */}
          <div className="flex items-center gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className="gap-2">
                  <MessageSquare className="w-4 h-4" />
                  <span className="max-w-[150px] truncate">
                    {currentSession?.title || '选择对话'}
                  </span>
                  <ChevronDown className="w-4 h-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-64 max-h-80 overflow-auto">
                {sessions.length === 0 ? (
                  <DropdownMenuItem disabled>暂无对话</DropdownMenuItem>
                ) : (
                  sessions.map((session) => (
                    <DropdownMenuItem
                      key={session.id}
                      onClick={() => handleSelectSession(session)}
                      className="flex items-center justify-between gap-2 group"
                    >
                      <span className="flex-1 truncate">{session.title}</span>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 opacity-0 group-hover:opacity-100 hover:!opacity-100 hover:text-destructive hover:bg-destructive/10"
                        onClick={(e) => handleDeleteSession(e, session.id)}
                        title="删除对话"
                      >
                        <Trash2 className="w-3 h-3" />
                      </Button>
                    </DropdownMenuItem>
                  ))
                )}
                <DropdownMenuItem onClick={handleCreateSession} className="text-primary">
                  <Plus className="w-4 h-4 mr-2" />
                  新建对话
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>

            {/* 主视图切换 */}
            <ExportFormatMenu
              disabled={completedDocuments.length === 0}
              onSelect={(format) => openExportDialog(format)}
            />
            <Button
              variant={mainView === 'workspace' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setMainViewToWorkspace()}
              className="gap-2"
            >
              <LayoutDashboard className="w-4 h-4" />
              工作台
            </Button>
            <Button
              variant={mainView === 'guide' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setMainViewToGuide()}
            >
              指南
            </Button>
            <Button
              variant={mainView === 'pdf' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => {
                if (activePdfId) {
                  setMainViewToPdf(activePdfId);
                } else if (documents.length > 0) {
                  setMainViewToPdf(documents[0].id);
                }
              }}
              disabled={documents.length === 0}
            >
              PDF
            </Button>
          </div>
        </header>

        {/* 主视图内容 */}
        <main className="flex-1 overflow-hidden">
          {mainView === 'workspace' ? (
            <NotebookWorkspace
              notebookId={notebookId}
              session={currentSession}
              documents={documents}
              selectedDocumentIds={selectedDocumentIds}
              onOpenExport={openExportDialog}
            />
          ) : mainView === 'guide' ? (
            <GuideView
              notebookId={notebookId}
              documents={documents}
              onSuggestedQueryClick={handleSuggestedQueryClick}
            />
          ) : activePdfId ? (
            <PdfViewer
              documentId={activePdfId}
              fileUrl={`/api/documents/${activePdfId}/pdf`}
            />
          ) : (
            <div className="flex items-center justify-center h-full text-center p-8">
              <div>
                <p className="text-muted-foreground mb-2">暂无 PDF 文档</p>
                <p className="text-sm text-muted-foreground">
                  从左侧上传 PDF 开始分析
                </p>
              </div>
            </div>
          )}
        </main>

        {/* 可拖动分隔条 + 底部对话面板 */}
        <div
          ref={containerRef}
          className="border-t bg-background flex flex-col"
          style={{ height: chatHeight }}
        >
          {/* 拖动把手 */}
          <div
            className={cn(
              'flex items-center justify-center h-6 cursor-row-resize select-none',
              'hover:bg-muted/50 transition-colors',
              isDragging && 'bg-primary/10'
            )}
            onMouseDown={handleDragStart}
          >
            <GripHorizontal className="w-4 h-4 text-muted-foreground" />
          </div>

          {/* 对话面板 */}
          <div className="flex-1 min-h-0">
            {/* 等待 Zustand hydrate 完成且 session 已选定后再渲染 ChatPanel */}
            {/* 避免在 sessionId 为 null 时渲染 ChatPanel 导致显示空状态（EmptyState） */}
            {!_hasHydrated ? (
              <div className="flex items-center justify-center h-full">
                <div className="flex flex-col items-center gap-3 text-muted-foreground">
                  <Loader2 className="w-6 h-6 animate-spin" />
                  <p className="text-sm">正在恢复会话...</p>
                </div>
              </div>
            ) : (
              <ChatPanel
                key={currentSession?.id || 'no-session'}
                notebookId={notebookId}
                sessionId={currentSession?.id || null}
                onSessionCreate={handleCreateSession}
                pendingQuery={pendingSuggestedQuery}
                onExportIntent={handleExportIntent}
                className="h-full border-0 rounded-none"
              />
            )}
          </div>
        </div>
      </div>

      {/* 右侧笔记面板 */}
      <div className="relative">
        <NotesPanel
          isOpen={notesPanelOpen}
          onToggle={() => setNotesPanelOpen(!notesPanelOpen)}
          notebookId={notebookId}
        />
      </div>

      <ExportDialog
        open={exportDialogOpen}
        notebookId={notebookId}
        documents={documents}
        selectedDocumentIds={selectedDocumentIds}
        initialFormat={exportFormat}
        initialRequirements={exportRequirements}
        onClose={() => setExportDialogOpen(false)}
        onTaskStart={handleExportTaskStart}
      />
      <ExportTaskTray
        notebookId={notebookId}
        artifactId={activeExportArtifactId}
        onClear={() => setActiveExportArtifactId(null)}
      />
    </div>
  );
}
