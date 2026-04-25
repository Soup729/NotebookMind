// ============================================================
// NotebookMind - PDF 单页组件
// ============================================================

'use client';

import React, { memo, useState, useEffect, useRef } from 'react';
import { Document, Page, pdfjs } from 'react-pdf';
import 'react-pdf/dist/Page/AnnotationLayer.css';
import 'react-pdf/dist/Page/TextLayer.css';
import { cn } from '@/lib/utils';
import { Skeleton } from '@/components/ui/skeleton';
import { HighlightLayer } from './HighlightLayer';
import type { HighlightTarget } from '@/types/api';

type LoadedPdfPage = Parameters<NonNullable<React.ComponentProps<typeof Page>['onLoadSuccess']>>[0];

// 配置 PDF.js worker
pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`;

interface PdfPageProps {
  pageNumber: number;
  numPages: number;
  fileUrl: string;
  scale: number;
  highlights: HighlightTarget[];
  activeHighlightId?: string | null;
  onLoadSuccess?: (page: LoadedPdfPage) => void;
  onHighlightClick?: (highlight: HighlightTarget) => void;
  className?: string;
}

export const PdfPage = memo(function PdfPage({
  pageNumber,
  numPages,
  fileUrl,
  scale,
  highlights,
  activeHighlightId,
  onLoadSuccess,
  onHighlightClick,
  className,
}: PdfPageProps) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pageDimensions, setPageDimensions] = useState({ width: 0, height: 0 });
  const containerRef = useRef<HTMLDivElement>(null);

  // 计算页面尺寸
  useEffect(() => {
    const updateDimensions = () => {
      if (containerRef.current) {
        const containerWidth = containerRef.current.offsetWidth;
        // 保持原始宽高比，假设 PDF 页面 standard ratio
        const aspectRatio = 1.414; // A4 ratio
        const width = Math.min(containerWidth, 800);
        const height = width * aspectRatio;
        setPageDimensions({ width, height });
      }
    };

    updateDimensions();
    window.addEventListener('resize', updateDimensions);
    return () => window.removeEventListener('resize', updateDimensions);
  }, []);

  const handleLoadSuccess = (page: LoadedPdfPage) => {
    setLoading(false);
    setError(null);

    // 更新页面尺寸
    const viewport = page.getViewport({ scale: 1 });
    setPageDimensions({
      width: viewport.width,
      height: viewport.height,
    });

    onLoadSuccess?.(page);
  };

  const handleLoadError = (err: Error) => {
    setLoading(false);
    setError(err.message);
    console.error('PDF page load error:', err);
  };

  return (
    <div
      ref={containerRef}
      className={cn('relative flex justify-center', className)}
    >
      {/* 加载骨架 */}
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Skeleton className="w-full max-w-2xl h-full" />
        </div>
      )}

      {/* 错误提示 */}
      {error && (
        <div className="absolute inset-0 flex items-center justify-center bg-muted">
          <p className="text-sm text-destructive">Failed to load page: {error}</p>
        </div>
      )}

      {/* PDF 文档 */}
      <Document
        file={fileUrl}
        onLoadSuccess={() => setLoading(false)}
        onLoadError={handleLoadError}
        loading={null}
        className={cn('shadow-lg', loading && 'invisible')}
      >
        <Page
          pageNumber={pageNumber}
          scale={scale}
          onLoadSuccess={handleLoadSuccess}
          onLoadError={handleLoadError}
          renderTextLayer={true}
          renderAnnotationLayer={true}
          className="relative"
          width={pageDimensions.width}
        >
          {/* 高亮图层 */}
          <HighlightLayer
            pageNumber={pageNumber}
            pageWidth={pageDimensions.width}
            pageHeight={pageDimensions.height}
            scale={scale}
            highlights={highlights}
            activeHighlightId={activeHighlightId}
            onHighlightClick={onHighlightClick}
          />
        </Page>
      </Document>

      {/* 页码指示器 */}
      <div className="absolute bottom-2 right-2 px-2 py-1 bg-background/80 backdrop-blur rounded text-xs text-muted-foreground">
        {pageNumber} / {numPages}
      </div>
    </div>
  );
});

// ============================================================
// PDF 缩放控制
// ============================================================

interface PdfZoomControlProps {
  scale: number;
  onScaleChange: (scale: number) => void;
  className?: string;
}

export const PdfZoomControl = memo(function PdfZoomControl({
  scale,
  onScaleChange,
  className,
}: PdfZoomControlProps) {
  const zoomLevels = [0.5, 0.75, 1, 1.25, 1.5, 2];

  return (
    <div className={cn('flex items-center gap-1', className)}>
      <button
        onClick={() => onScaleChange(Math.max(0.5, scale - 0.25))}
        disabled={scale <= 0.5}
        className={cn(
          'px-2 py-1 text-sm rounded hover:bg-muted',
          'disabled:opacity-50 disabled:cursor-not-allowed'
        )}
      >
        -
      </button>
      <span className="text-sm w-14 text-center">{Math.round(scale * 100)}%</span>
      <button
        onClick={() => onScaleChange(Math.min(2, scale + 0.25))}
        disabled={scale >= 2}
        className={cn(
          'px-2 py-1 text-sm rounded hover:bg-muted',
          'disabled:opacity-50 disabled:cursor-not-allowed'
        )}
      >
        +
      </button>
      <div className="w-px h-4 bg-border mx-1" />
      {zoomLevels.map((level) => (
        <button
          key={level}
          onClick={() => onScaleChange(level)}
          className={cn(
            'px-1.5 py-0.5 text-xs rounded',
            scale === level && 'bg-primary text-primary-foreground'
          )}
        >
          {level * 100}%
        </button>
      ))}
    </div>
  );
});
