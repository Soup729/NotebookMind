// ============================================================
// NotebookMind - PDF 阅读器组件 (核心)
// ============================================================

'use client';

import React, {
  useState,
  useCallback,
  useEffect,
  useMemo,
} from 'react';
import {
  ChevronLeft,
  ChevronRight,
  X,
  ZoomIn,
  ZoomOut,
  Maximize,
  FileText,
  Loader2,
} from 'lucide-react';
import { Document, Page, pdfjs } from 'react-pdf';
import 'react-pdf/dist/Page/AnnotationLayer.css';
import 'react-pdf/dist/Page/TextLayer.css';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { HighlightLayer, HighlightMarker } from './HighlightLayer';
import { useNotebookStore } from '@/store/useNotebookStore';
import type { HighlightTarget } from '@/types/api';

type LoadedPdfPage = Parameters<NonNullable<React.ComponentProps<typeof Page>['onLoadSuccess']>>[0];

// 配置 PDF.js worker
pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url
).toString();

// ============================================================
// 类型定义
// ============================================================

interface PdfViewerProps {
  documentId: string;
  fileUrl?: string;
  className?: string;
}

interface PdfViewerState {
  numPages: number;
  currentPage: number;
  scale: number;
  isLoading: boolean;
  error: string | null;
  pageDimensions: { width: number; height: number };
}

// ============================================================
// PDF 阅读器组件
// ============================================================

export function PdfViewer({
  documentId,
  fileUrl,
  className,
}: PdfViewerProps) {
  // 状态
  const [state, setState] = useState<PdfViewerState>({
    numPages: 0,
    currentPage: 1,
    scale: 1,
    isLoading: true,
    error: null,
    pageDimensions: { width: 0, height: 0 },
  });
  const [documentFile, setDocumentFile] = useState<string | null>(null);

  // Store
  const highlightTarget = useNotebookStore((state) => state.highlightTarget);
  const setMainViewToGuide = useNotebookStore((state) => state.setMainViewToGuide);

  useEffect(() => {
    if (!fileUrl) {
      setDocumentFile(null);
      return;
    }

    const controller = new AbortController();
    let objectUrl: string | null = null;

    setState((prev) => ({
      ...prev,
      isLoading: true,
      error: null,
    }));
    setDocumentFile(null);

    const loadPdf = async () => {
      try {
        const token = localStorage.getItem('auth_token');
        const response = await fetch(fileUrl, {
          signal: controller.signal,
          headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        });

        if (!response.ok) {
          if (response.status === 401) {
            throw new Error('登录状态已失效，请重新登录后再打开引用文档。');
          }
          if (response.status === 404) {
            throw new Error('无法打开该引用文档。文档可能已被删除，或原始 PDF 文件不存在。');
          }
          throw new Error(`PDF 加载失败，HTTP ${response.status}`);
        }

        const blob = await response.blob();
        objectUrl = URL.createObjectURL(blob);
        setDocumentFile(objectUrl);
      } catch (err) {
        if (controller.signal.aborted) {
          return;
        }
        const message = err instanceof Error ? err.message : 'PDF 加载失败';
        setState((prev) => ({
          ...prev,
          isLoading: false,
          error: message,
        }));
      }
    };

    loadPdf();

    return () => {
      controller.abort();
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [fileUrl]);

  // 计算高亮列表
  const highlights = useMemo(() => {
    if (!highlightTarget || highlightTarget.documentId !== documentId) {
      return [];
    }
    return [highlightTarget];
  }, [highlightTarget, documentId]);

  useEffect(() => {
    if (!highlightTarget || highlightTarget.documentId !== documentId) {
      return;
    }
    setState((prev) => ({
      ...prev,
      currentPage: Math.max(1, highlightTarget.pageNumber),
    }));
  }, [highlightTarget, documentId]);

  // ============================================================
  // 文档加载成功
  // ============================================================

  const onDocumentLoadSuccess = useCallback(
    ({ numPages }: { numPages: number }) => {
      setState((prev) => ({
        ...prev,
        numPages,
        isLoading: false,
        error: null,
      }));
    },
    []
  );

  // ============================================================
  // 页面加载成功
  // ============================================================

  const onPageLoadSuccess = useCallback((page: LoadedPdfPage) => {
    const viewport = page.getViewport({ scale: 1 });
    setState((prev) => ({
      ...prev,
      pageDimensions: {
        width: viewport.width,
        height: viewport.height,
      },
    }));
  }, []);

  // ============================================================
  // 翻页
  // ============================================================

  const goToPrevPage = useCallback(() => {
    setState((prev) => ({
      ...prev,
      currentPage: Math.max(1, prev.currentPage - 1),
    }));
  }, []);

  const goToNextPage = useCallback(() => {
    setState((prev) => ({
      ...prev,
      currentPage: Math.min(prev.numPages, prev.currentPage + 1),
    }));
  }, []);

  const goToPage = useCallback((page: number) => {
    setState((prev) => ({
      ...prev,
      currentPage: Math.max(1, Math.min(prev.numPages, page)),
    }));
  }, []);

  // ============================================================
  // 缩放
  // ============================================================

  const zoomIn = useCallback(() => {
    setState((prev) => ({
      ...prev,
      scale: Math.min(2, prev.scale + 0.25),
    }));
  }, []);

  const zoomOut = useCallback(() => {
    setState((prev) => ({
      ...prev,
      scale: Math.max(0.5, prev.scale - 0.25),
    }));
  }, []);

  const resetZoom = useCallback(() => {
    setState((prev) => ({
      ...prev,
      scale: 1,
    }));
  }, []);

  // ============================================================
  // 高亮点击
  // ============================================================

  const handleHighlightClick = useCallback(
    (highlight: HighlightTarget) => {
      // 跳转到对应页面
      if (highlight.pageNumber !== state.currentPage) {
        goToPage(highlight.pageNumber);
      }
    },
    [state.currentPage, goToPage]
  );

  // ============================================================
  // 键盘快捷键
  // ============================================================

  useEffect(() => {
    const handleKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'ArrowLeft') {
        goToPrevPage();
      } else if (e.key === 'ArrowRight') {
        goToNextPage();
      } else if (e.key === '+' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        zoomIn();
      } else if (e.key === '-' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        zoomOut();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [goToPrevPage, goToNextPage, zoomIn, zoomOut]);

  // ============================================================
  // 如果没有 fileUrl，显示提示
  // ============================================================

  if (!fileUrl) {
    return (
      <Card className={cn('flex flex-col items-center justify-center h-full', className)}>
        <FileText className="w-12 h-12 text-muted-foreground mb-4" />
        <p className="text-muted-foreground">无法加载 PDF</p>
        <p className="text-sm text-muted-foreground mt-1">文档URL不可用</p>
      </Card>
    );
  }

  return (
    <div className={cn('flex flex-col h-full bg-muted/20', className)}>
      {/* 工具栏 */}
      <div className="flex items-center justify-between px-4 py-2 border-b bg-background/95 backdrop-blur">
        {/* 左侧：文档信息 */}
        <div className="flex items-center gap-2 text-sm">
          <FileText className="w-4 h-4 text-muted-foreground" />
          <span className="font-medium truncate max-w-[200px]">
            {documentId.slice(0, 8)}...
          </span>
        </div>

        {/* 中间：页面导航 */}
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={goToPrevPage}
            disabled={state.currentPage <= 1}
            className="h-8 w-8"
            title="上一页"
          >
            <ChevronLeft className="w-4 h-4" />
          </Button>

          <div className="flex items-center gap-1 text-sm">
            <input
              type="number"
              value={state.currentPage}
              onChange={(e) => goToPage(parseInt(e.target.value, 10))}
              min={1}
              max={state.numPages}
              className={cn(
                'w-12 h-8 text-center rounded border bg-background',
                'focus:outline-none focus:ring-2 focus:ring-primary'
              )}
            />
            <span className="text-muted-foreground">/</span>
            <span className="text-muted-foreground">{state.numPages}</span>
          </div>

          <Button
            variant="ghost"
            size="icon"
            onClick={goToNextPage}
            disabled={state.currentPage >= state.numPages}
            className="h-8 w-8"
            title="下一页"
          >
            <ChevronRight className="w-4 h-4" />
          </Button>
        </div>

        {/* 右侧：缩放控制 */}
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={zoomOut}
            disabled={state.scale <= 0.5}
            className="h-8 w-8"
            title="缩小"
          >
            <ZoomOut className="w-4 h-4" />
          </Button>

          <span className="text-sm w-14 text-center">
            {Math.round(state.scale * 100)}%
          </span>

          <Button
            variant="ghost"
            size="icon"
            onClick={zoomIn}
            disabled={state.scale >= 2}
            className="h-8 w-8"
            title="放大"
          >
            <ZoomIn className="w-4 h-4" />
          </Button>

          <Button
            variant="ghost"
            size="icon"
            onClick={resetZoom}
            className="h-8 w-8"
            title="重置缩放"
          >
            <Maximize className="w-4 h-4" />
          </Button>
        </div>
      </div>

      {/* PDF 内容区域 */}
      <div className="flex-1 overflow-auto custom-scrollbar p-4">
        <div className="flex justify-center">
          {/* 加载状态 */}
          {state.isLoading && (
            <div className="flex flex-col items-center justify-center">
              <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
              <p className="text-sm text-muted-foreground mt-2">加载中...</p>
            </div>
          )}

          {/* 错误状态 */}
          {state.error && (
            <Card className="p-8 text-center">
              <p className="text-destructive">加载失败: {state.error}</p>
            </Card>
          )}

          {/* PDF 文档 */}
          {!state.error && documentFile && (
            <Document
              file={documentFile}
              onLoadSuccess={onDocumentLoadSuccess}
              onLoadError={(err) => {
                const detail = err.message.includes('Missing PDF')
                  ? '无法打开该引用文档。文档可能已被删除，或原始 PDF 文件不存在。'
                  : err.message;
                setState((prev) => ({
                  ...prev,
                  isLoading: false,
                  error: detail,
                }));
              }}
              loading={null}
              className={cn('shadow-lg', state.isLoading && 'invisible')}
            >
              <Page
                pageNumber={state.currentPage}
                scale={state.scale}
                onLoadSuccess={onPageLoadSuccess}
                renderTextLayer={true}
                renderAnnotationLayer={true}
                className="relative bg-white"
              >
                {/* 高亮图层 */}
                <HighlightLayer
                  pageNumber={state.currentPage}
                  pageWidth={state.pageDimensions.width}
                  pageHeight={state.pageDimensions.height}
                  scale={state.scale}
                  highlights={highlights}
                  activeHighlightId={highlightTarget?.sourceId}
                  onHighlightClick={handleHighlightClick}
                />
              </Page>
            </Document>
          )}
        </div>
      </div>

      {/* 高亮列表（底部） */}
      {highlights.length > 0 && (
        <div className="border-t bg-background p-2 max-h-32 overflow-auto">
          <p className="text-xs text-muted-foreground px-2 py-1">相关高亮</p>
          <div className="flex gap-2 overflow-x-auto pb-1">
            {highlights.map((highlight) => (
              <HighlightMarker
                key={highlight.sourceId}
                highlight={highlight}
                onClick={handleHighlightClick}
                isActive={highlight.pageNumber === state.currentPage}
                className="flex-shrink-0"
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ============================================================
// 导出
// ============================================================

export default PdfViewer;
