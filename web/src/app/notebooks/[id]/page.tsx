// ============================================================
// Enterprise PDF AI - 笔记本工作台页面 (核心)
// ============================================================

'use client';

import React, { useState, useCallback, useEffect, useRef } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { ArrowLeft, Plus, Loader2, GripHorizontal, MessageSquare, Trash2, ChevronDown } from 'lucide-react';
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

// PDF 查看器
import { PdfViewer } from '@/components/pdf/PdfViewer';

// 指南组件
import { DocumentGuide } from '@/components/guide/DocumentGuide';

// Hooks
import { useNotebook } from '@/hooks/useNotebook';
import { useDocuments, useUploadDocument, useDeleteDocument } from '@/hooks/useNotebook';
import { useSessions, useCreateSession, useDeleteSession } from '@/hooks/useNotebook';
import { useNotebookStore } from '@/store/useNotebookStore';

import type { Session, Document } from '@/types/api';

// ============================================================
// 指南视图组件
// ============================================================

interface GuideViewProps {
  notebookId: string;
  documents: Document[];
}

function GuideView({ notebookId, documents }: GuideViewProps) {
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
    setNotebookAndSession,
    setSelectedDocuments,
    toggleDocumentSelection,
    setMainViewToPdf,
    setMainViewToGuide,
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

  // 当前会话
  const [currentSession, setCurrentSession] = useState<Session | null>(null);
  // 防止重复创建会话的锁
  const isCreatingRef = useRef(false);

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

  // ============================================================
  // 初始化
  // ============================================================

  useEffect(() => {
    if (notebookId) {
      setNotebookAndSession(notebookId);
    }
  }, [notebookId, setNotebookAndSession]);

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
  useEffect(() => {
    if (sessions.length > 0 && !currentSession) {
      // 选择最近的会话
      const sorted = [...sessions].sort(
        (a, b) =>
          new Date(b.last_message_at).getTime() -
          new Date(a.last_message_at).getTime()
      );
      setCurrentSession(sorted[0]);
    }
  }, [sessions, currentSession]);

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
    },
    []
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
  // 主视图切换
  // ============================================================

  const handleSourceClick = useCallback(
    (source: { document_id: string; page_number: number; content: string }) => {
      setMainViewToPdf(source.document_id, {
        pageNumber: source.page_number,
        boundingBox: [0, 0, 0, 0],
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
              <h1 className="font-semibold text-sm">{notebook.title}</h1>
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
          {mainView === 'guide' ? (
            <GuideView notebookId={notebookId} documents={documents} />
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
            <ChatPanel
              notebookId={notebookId}
              sessionId={currentSession?.id || null}
              onSessionCreate={handleCreateSession}
              className="h-full border-0 rounded-none"
            />
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
    </div>
  );
}
