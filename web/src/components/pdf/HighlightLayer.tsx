// ============================================================
// NotebookMind - 高亮图层组件
// ============================================================

'use client';

import React, { memo, useMemo } from 'react';
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
  // 过滤当前页的高亮
  const pageHighlights = useMemo(
    () => highlights.filter((h) => h.pageNumber === pageNumber),
    [highlights, pageNumber]
  );

  if (pageHighlights.length === 0) return null;

  return (
    <div
      className={cn(
        'absolute inset-0 pointer-events-none z-10',
        className
      )}
      style={{ width: pageWidth, height: pageHeight }}
    >
      {pageHighlights.map((highlight) => {
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
    </div>
  );
});

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
