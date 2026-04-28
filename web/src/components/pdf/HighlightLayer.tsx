// ============================================================
// NotebookMind - 高亮图层组件
// ============================================================

'use client';

import React, { memo, useEffect, useMemo, useRef, useState } from 'react';
import { cn } from '@/lib/utils';
import type { HighlightTarget } from '@/types/api';

interface HighlightLayerProps {
  pageNumber: number;
  pageWidth: number;
  pageHeight: number;
  scale: number;
  highlights: HighlightTarget[];
  activeHighlightId?: string | null;
  onHighlightClick?: (highlight: HighlightTarget) => void;
  className?: string;
}

export const HighlightLayer = memo(function HighlightLayer({
  pageNumber,
  pageWidth,
  pageHeight,
  scale,
  highlights,
  activeHighlightId,
  onHighlightClick,
  className,
}: HighlightLayerProps) {
  const layerRef = useRef<HTMLDivElement>(null);
  const [textMatchRects, setTextMatchRects] = useState<Record<string, TextMatchRect[]>>({});
  const pageHighlights = useMemo(
    () => highlights.filter((h) => h.pageNumber === pageNumber),
    [highlights, pageNumber]
  );
  const boxHighlights = useMemo(
    () => pageHighlights.filter((h) => shouldUseBackendBox(h) && isValidBoundingBox(h.boundingBox, pageWidth, pageHeight)),
    [pageHeight, pageHighlights, pageWidth]
  );

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const layer = layerRef.current;
      const page = layer?.closest('.react-pdf__Page');
      const textLayer = page?.querySelector('.react-pdf__Page__textContent');
      if (!layer || !textLayer) {
        setTextMatchRects({});
        return;
      }

      const next: Record<string, TextMatchRect[]> = {};
      for (const highlight of pageHighlights) {
        if (shouldUseBackendBox(highlight)) {
          continue;
        }
        const rects = findTextMatchRects(textLayer, layer, highlight.content);
        if (rects.length > 0) {
          next[highlight.sourceId] = rects;
        }
      }
      setTextMatchRects(next);
    }, 80);
    return () => window.clearTimeout(timer);
  }, [pageHighlights, pageWidth, pageHeight]);

  if (pageHighlights.length === 0) return null;

  return (
    <div
      className={cn(
        'absolute inset-0 pointer-events-none z-10',
        className
      )}
      ref={layerRef}
      style={{ width: '100%', height: '100%' }}
    >
      {boxHighlights.map((highlight) => {
        const [x0, y0, x1, y1] = highlight.boundingBox;
        const isActive = activeHighlightId === highlight.sourceId;

        // 计算高亮区域样式（基于页面百分比）
        const style: React.CSSProperties = {
          position: 'absolute',
          left: `${(x0 / pageWidth) * 100}%`,
          top: `${(y0 / pageHeight) * 100}%`,
          width: `${((x1 - x0) / pageWidth) * 100}%`,
          height: `${((y1 - y0) / pageHeight) * 100}%`,
          backgroundColor: isActive
            ? 'rgba(253, 224, 71, 0.6)'
            : 'rgba(254, 240, 138, 0.5)',
          backdropFilter: 'blur(2px)',
          borderRadius: '2px',
          pointerEvents: 'auto',
          cursor: 'pointer',
          transition: 'all 0.2s ease-out',
          boxShadow: isActive
            ? '0 0 0 2px rgba(250, 204, 21, 0.5)'
            : '0 0 0 2px rgba(250, 204, 21, 0.3)',
        };

        return (
          <div
            key={highlight.sourceId}
            style={style}
            onClick={() => onHighlightClick?.(highlight)}
            title={highlight.content?.slice(0, 100)}
            className={cn(
              'group',
              isActive && 'ring-2 ring-yellow-400 ring-offset-1'
            )}
          >
            {/* 悬停提示 */}
            <div
              className={cn(
                'absolute left-0 bottom-full mb-1 px-2 py-1 text-xs',
                'bg-popover text-popover-foreground rounded shadow-lg',
                'opacity-0 group-hover:opacity-100 transition-opacity',
                'whitespace-nowrap max-w-xs truncate',
                'pointer-events-none'
              )}
            >
              {highlight.documentName}
              {highlight.pageNumber > 0 && ` - P.${highlight.pageNumber}`}
            </div>
          </div>
        );
      })}
      {pageHighlights.flatMap((highlight) => {
        const rects = textMatchRects[highlight.sourceId] || [];
        const isActive = activeHighlightId === highlight.sourceId;
        return rects.map((rect, index) => (
          <div
            key={`${highlight.sourceId}-text-${index}`}
            style={{
              position: 'absolute',
              left: rect.left,
              top: rect.top,
              width: rect.width,
              height: rect.height,
              backgroundColor: isActive
                ? 'rgba(253, 224, 71, 0.55)'
                : 'rgba(254, 240, 138, 0.45)',
              borderRadius: '2px',
              pointerEvents: 'auto',
              cursor: 'pointer',
              boxShadow: isActive
                ? '0 0 0 2px rgba(250, 204, 21, 0.45)'
                : '0 0 0 1px rgba(250, 204, 21, 0.25)',
            }}
            onClick={() => onHighlightClick?.(highlight)}
            title={highlight.content?.slice(0, 100)}
          />
        ));
      })}
    </div>
  );
});

interface TextMatchRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

function shouldUseBackendBox(highlight: HighlightTarget): boolean {
  const type = (highlight.chunkType || '').toLowerCase();
  return type === 'image';
}

function isValidBoundingBox(
  box: HighlightTarget['boundingBox'] | null | undefined,
  pageWidth: number,
  pageHeight: number
): box is HighlightTarget['boundingBox'] {
  if (!box || box.length !== 4 || pageWidth <= 0 || pageHeight <= 0) {
    return false;
  }
  const [x0, y0, x1, y1] = box;
  if (![x0, y0, x1, y1].every(Number.isFinite)) {
    return false;
  }
  if (x1 <= x0 || y1 <= y0) {
    return false;
  }
  if (x0 === 0 && y0 === 0 && x1 === 0 && y1 === 0) {
    return false;
  }
  return x1 > 0 && y1 > 0 && x0 < pageWidth && y0 < pageHeight;
}

function findTextMatchRects(textLayer: Element, layer: HTMLElement, content: string): TextMatchRect[] {
  const words = tokenize(content);
  if (words.length < 3) {
    return [];
  }

  const spans = Array.from(textLayer.querySelectorAll('span')) as HTMLElement[];
  const pageWords: Array<{ word: string; span: HTMLElement }> = [];
  for (const span of spans) {
    for (const word of tokenize(span.textContent || '')) {
      pageWords.push({ word, span });
    }
  }
  if (pageWords.length === 0) {
    return [];
  }

  const windows = candidateWordWindows(words);
  for (const candidate of windows) {
    const start = findWordSequence(pageWords, candidate);
    if (start < 0) {
      continue;
    }
    const matchedSpans = new Set<HTMLElement>();
    for (let i = start; i < start + candidate.length; i++) {
      matchedSpans.add(pageWords[i].span);
    }
    const layerRect = layer.getBoundingClientRect();
    return Array.from(matchedSpans)
      .slice(0, 8)
      .map((span) => {
        const rect = span.getBoundingClientRect();
        return {
          left: rect.left - layerRect.left,
          top: rect.top - layerRect.top,
          width: rect.width,
          height: rect.height,
        };
      })
      .filter((rect) => rect.width > 0 && rect.height > 0);
  }
  return [];
}

function candidateWordWindows(words: string[]): string[][] {
  const windows: string[][] = [];
  const sizes = [10, 8, 6, 4];
  const starts = [0, Math.floor(words.length * 0.25), Math.floor(words.length * 0.5)];
  for (const size of sizes) {
    if (words.length < size) {
      continue;
    }
    for (const start of starts) {
      const safeStart = Math.min(start, words.length - size);
      const candidate = words.slice(safeStart, safeStart + size);
      if (candidate.length === size) {
        windows.push(candidate);
      }
    }
  }
  return windows;
}

function findWordSequence(pageWords: Array<{ word: string; span: HTMLElement }>, candidate: string[]): number {
  outer:
  for (let i = 0; i <= pageWords.length - candidate.length; i++) {
    for (let j = 0; j < candidate.length; j++) {
      if (pageWords[i + j].word !== candidate[j]) {
        continue outer;
      }
    }
    return i;
  }
  return -1;
}

function tokenize(value: string): string[] {
  return value
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .split(/\s+/)
    .map((word) => word.trim())
    .filter((word) => word.length >= 2);
}

// ============================================================
// 高亮标记组件（用于列表展示）
// ============================================================

interface HighlightMarkerProps {
  highlight: HighlightTarget;
  onClick?: (highlight: HighlightTarget) => void;
  isActive?: boolean;
  className?: string;
}

export const HighlightMarker = memo(function HighlightMarker({
  highlight,
  onClick,
  isActive = false,
  className,
}: HighlightMarkerProps) {
  return (
    <button
      onClick={() => onClick?.(highlight)}
      className={cn(
        'flex items-start gap-2 w-full p-2 rounded-md text-left',
        'hover:bg-muted/50 transition-colors',
        isActive && 'bg-muted',
        className
      )}
    >
      <div
        className={cn(
          'w-2 h-2 mt-1.5 rounded-full flex-shrink-0',
          isActive ? 'bg-yellow-400' : 'bg-yellow-200'
        )}
      />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium truncate">
          {highlight.documentName}
        </p>
        {highlight.pageNumber > 0 && (
          <p className="text-xs text-muted-foreground">
            Page {highlight.pageNumber}
          </p>
        )}
        {highlight.content && (
          <p className="text-xs text-muted-foreground mt-1 line-clamp-2">
            {highlight.content}
          </p>
        )}
      </div>
    </button>
  );
});
