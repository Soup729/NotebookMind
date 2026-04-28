// ============================================================
// NotebookMind - 来源徽章组件
// ============================================================

'use client';

import React, { memo } from 'react';
import { FileText, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ChatSource } from '@/types/api';

function getDocumentName(source: Partial<ChatSource> | null | undefined): string {
  const maybeSource = source as (Partial<ChatSource> & {
    file_name?: unknown;
    documentName?: unknown;
  }) | null | undefined;
  const value =
    maybeSource?.document_name ??
    maybeSource?.file_name ??
    maybeSource?.documentName ??
    '';
  return String(value || '').trim() || '未知来源';
}

interface SourceBadgeProps {
  source: ChatSource;
  onClick?: (source: ChatSource) => void;
  isActive?: boolean;
  className?: string;
}

export const SourceBadge = memo(function SourceBadge({
  source,
  onClick,
  isActive = false,
  className,
}: SourceBadgeProps) {
  const documentName = getDocumentName(source);
  const displayPage = Number(source.page_number || 0) + 1;
  const handleClick = () => {
    onClick?.(source);
  };

  return (
    <button
      type="button"
      onClick={handleClick}
      className={cn(
        'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-xs font-medium',
        'bg-yellow-100/70 text-yellow-800 border border-yellow-200/50',
        'hover:bg-yellow-200/80 hover:border-yellow-300',
        'transition-all duration-200 ease-out',
        'cursor-pointer select-none',
        isActive && 'bg-yellow-200/80 border-yellow-400 shadow-sm',
        className
      )}
      title={source.content ? `${source.content.slice(0, 100)}...` : documentName}
    >
      <FileText className="w-3 h-3 flex-shrink-0" />
      <span className="truncate max-w-[120px]">
        {documentName}
        {source.page_number >= 0 && `, P.${displayPage}`}
      </span>
      {onClick && <ExternalLink className="w-2.5 h-2.5 opacity-50" />}
    </button>
  );
});

// ============================================================
// 来源列表组件
// ============================================================

interface SourceListProps {
  sources: ChatSource[];
  onSourceClick?: (source: ChatSource) => void;
  className?: string;
}

export function SourceList({ sources, onSourceClick, className }: SourceListProps) {
  if (!sources || sources.length === 0) return null;

  const documentSources = Array.from(
    sources.reduce((acc, source) => {
      const key = source.document_id || getDocumentName(source);
      const existing = acc.get(key);
      if (!existing || source.score > existing.score) {
        acc.set(key, source);
      }
      return acc;
    }, new Map<string, ChatSource>()).values()
  );

  return (
    <div className={cn('flex flex-wrap gap-2', className)}>
      {documentSources.map((source, index) => (
        <SourceBadge
          key={`${source.document_id || getDocumentName(source)}-${index}`}
          source={source}
          onClick={onSourceClick}
        />
      ))}
    </div>
  );
}

// ============================================================
// 内联来源引用组件（用于 Markdown 内容中）
// ============================================================

interface InlineSourceBadgeProps {
  documentName: string;
  pageNumber?: number;
  source: ChatSource;
  onClick?: (source: ChatSource) => void;
}

export function InlineSourceBadge({
  documentName,
  pageNumber,
  source,
  onClick,
}: InlineSourceBadgeProps) {
  return (
    <SourceBadge
      source={{
        ...source,
        document_name: documentName,
        page_number: pageNumber || source.page_number,
      }}
      onClick={onClick}
      className="inline-flex mx-0.5"
    />
  );
}
