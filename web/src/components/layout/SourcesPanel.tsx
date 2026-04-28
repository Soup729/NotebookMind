// ============================================================
// NotebookMind - 左侧文档面板组件
// ============================================================

'use client';

import React, { memo, useCallback, useState } from 'react';
import {
  ChevronLeft,
  ChevronRight,
  Upload,
  FileText,
  CheckCircle,
  Clock,
  XCircle,
  Trash2,
  RefreshCw,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { formatFileSize, formatDate } from '@/lib/utils';
import type { Document } from '@/types/api';

interface SourcesPanelProps {
  notebookId: string;
  documents: Document[];
  isLoading: boolean;
  selectedIds: string[];
  activeDocumentId?: string | null;
  onSelectionChange: (ids: string[]) => void;
  onUpload?: (file: File) => void;
  onOpen?: (docId: string) => void;
  onRemove?: (docId: string) => void;
  className?: string;
}

// ============================================================
// 文档状态图标
// ============================================================

const StatusIcon = memo(function StatusIcon({
  status,
  className,
}: {
  status: Document['status'];
  className?: string;
}) {
  switch (status) {
    case 'completed':
      return <CheckCircle className={cn('w-4 h-4 text-green-500', className)} />;
    case 'processing':
      return (
        <RefreshCw className={cn('w-4 h-4 text-blue-500 animate-spin', className)} />
      );
    case 'failed':
      return <XCircle className={cn('w-4 h-4 text-red-500', className)} />;
    default:
      return null;
  }
});

// ============================================================
// 单个文档项
// ============================================================

interface DocumentItemProps {
  document: Document;
  isSelected: boolean;
  isActive?: boolean;
  onToggle: () => void;
  onOpen?: () => void;
  onRemove?: () => void;
}

const DocumentItem = memo(function DocumentItem({
  document,
  isSelected,
  isActive = false,
  onToggle,
  onOpen,
  onRemove,
}: DocumentItemProps) {
  const canOpen = document.status === 'completed' && Boolean(onOpen);

  return (
    <div
      className={cn(
        'group relative flex items-start gap-3 p-3 pb-8 rounded-lg border border-transparent',
        'hover:bg-muted/50 transition-colors',
        canOpen && 'cursor-pointer',
        isSelected && 'bg-muted',
        isActive && 'border-primary/40 bg-primary/5'
      )}
      onClick={canOpen ? onOpen : undefined}
      role={canOpen ? 'button' : undefined}
      tabIndex={canOpen ? 0 : undefined}
      onKeyDown={(event) => {
        if (!canOpen) return;
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onOpen?.();
        }
      }}
    >
      {/* 选择框 */}
      <Checkbox
        checked={isSelected}
        onClick={(event) => event.stopPropagation()}
        onCheckedChange={onToggle}
        className="mt-0.5"
        disabled={document.status !== 'completed'}
      />

      {/* 文件图标 */}
      <div className="flex-shrink-0 mt-0.5">
        <FileText className="w-5 h-5 text-muted-foreground" />
      </div>

      {/* 文档信息 */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <p className="text-sm font-medium truncate" title={document.file_name}>
            {document.file_name}
          </p>
          <StatusIcon status={document.status} />
        </div>

        <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
          <span>{formatFileSize(document.file_size)}</span>
          <span>·</span>
          <span>{document.chunk_count} chunks</span>
        </div>

        {document.status === 'processing' && (
          <div className="flex items-center gap-1 mt-1">
            <div className="h-1 w-20 bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-blue-500 animate-pulse"
                style={{ width: '60%' }}
              />
            </div>
            <span className="text-xs text-blue-500">处理中</span>
          </div>
        )}
      </div>

      {/* 删除按钮 */}
      {onRemove && (
        <Button
          variant="ghost"
          size="icon"
          className="absolute bottom-2 right-2 h-6 w-6 opacity-0 group-hover:opacity-100 hover:!opacity-100 hover:text-destructive hover:bg-destructive/10"
          onClick={(event) => {
            event.stopPropagation();
            onRemove();
          }}
          title="删除文档"
        >
          <Trash2 className="w-3 h-3" />
        </Button>
      )}
    </div>
  );
});

// ============================================================
// 左侧文档面板
// ============================================================

export const SourcesPanel = memo(function SourcesPanel({
  notebookId,
  documents,
  isLoading,
  selectedIds,
  activeDocumentId,
  onSelectionChange,
  onUpload,
  onOpen,
  onRemove,
  className,
}: SourcesPanelProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isDragging, setIsDragging] = useState(false);

  // 切换文档选中
  const handleToggle = useCallback(
    (docId: string) => {
      const newSelection = selectedIds.includes(docId)
        ? selectedIds.filter((id) => id !== docId)
        : [...selectedIds, docId];
      onSelectionChange(newSelection);
    },
    [selectedIds, onSelectionChange]
  );

  // 全选
  const handleSelectAll = useCallback(() => {
    const completedIds = documents
      .filter((d) => d.status === 'completed')
      .map((d) => d.id);
    onSelectionChange(
      selectedIds.length === completedIds.length ? [] : completedIds
    );
  }, [documents, selectedIds, onSelectionChange]);

  // 拖放上传
  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragging(false);

      const files = Array.from(e.dataTransfer.files).filter(
        (file) => file.type === 'application/pdf'
      );

      if (files.length > 0 && onUpload) {
        files.forEach((file) => onUpload(file));
      }
    },
    [onUpload]
  );

  // 文件选择上传
  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(e.target.files || []);
      if (files.length > 0 && onUpload) {
        files.forEach((file) => onUpload(file));
      }
      e.target.value = '';
    },
    [onUpload]
  );

  if (isCollapsed) {
    return (
      <div
        className={cn(
          'h-full flex flex-col border-r bg-background',
          'w-12 flex-shrink-0',
          className
        )}
      >
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setIsCollapsed(false)}
          className="h-12 w-full rounded-none"
        >
          <ChevronRight className="w-4 h-4" />
        </Button>
        <div className="flex-1 flex flex-col items-center py-4 gap-2">
          {documents.slice(0, 5).map((doc) => (
            <FileText
              key={doc.id}
              className={cn(
                'w-5 h-5',
                selectedIds.includes(doc.id)
                  ? 'text-primary'
                  : 'text-muted-foreground'
              )}
            />
          ))}
          {documents.length > 5 && (
            <span className="text-xs text-muted-foreground">
              +{documents.length - 5}
            </span>
          )}
        </div>
      </div>
    );
  }

  return (
    <div
      className={cn(
        'h-full flex flex-col border-r bg-background',
        'w-72 flex-shrink-0 transition-all duration-300',
        className
      )}
    >
      {/* 头部 */}
      <div className="flex items-center justify-between px-4 py-3 border-b">
        <h2 className="font-semibold">Sources</h2>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setIsCollapsed(true)}
          className="h-8 w-8"
        >
          <ChevronLeft className="w-4 h-4" />
        </Button>
      </div>

      {/* 上传区域 */}
      <div
        className={cn(
          'm-3 p-4 border-2 border-dashed rounded-lg',
          'transition-colors',
          isDragging
            ? 'border-primary bg-primary/5'
            : 'border-muted-foreground/30 hover:border-muted-foreground/50',
          onUpload && 'cursor-pointer'
        )}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => document.getElementById('file-input')?.click()}
      >
        <input
          id="file-input"
          type="file"
          accept=".pdf"
          multiple
          onChange={handleFileInput}
          className="hidden"
        />
        <div className="flex flex-col items-center text-center">
          <Upload className="w-6 h-6 text-muted-foreground mb-2" />
          <p className="text-sm font-medium">上传 PDF</p>
          <p className="text-xs text-muted-foreground mt-1">
            拖拽或点击上传
          </p>
        </div>
      </div>

      {/* 文档列表 */}
      <div className="flex items-center justify-between px-3 py-2">
        <span className="text-xs text-muted-foreground">
          {selectedIds.length} / {documents.filter((d) => d.status === 'completed').length} 已选
        </span>
        <Button
          variant="ghost"
          size="sm"
          onClick={handleSelectAll}
          className="h-6 text-xs"
        >
          {selectedIds.length ===
          documents.filter((d) => d.status === 'completed').length
            ? '取消全选'
            : '全选'}
        </Button>
      </div>

      <ScrollArea className="flex-1">
        {isLoading ? (
          <div className="p-3 space-y-3">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex gap-3">
                <Skeleton className="w-4 h-4" />
                <Skeleton className="w-5 h-5" />
                <div className="flex-1 space-y-2">
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-3 w-24" />
                </div>
              </div>
            ))}
          </div>
        ) : documents.length === 0 ? (
          <div className="p-8 text-center">
            <FileText className="w-8 h-8 text-muted-foreground mx-auto mb-2" />
            <p className="text-sm text-muted-foreground">暂无文档</p>
            <p className="text-xs text-muted-foreground mt-1">
              上传 PDF 开始分析
            </p>
          </div>
        ) : (
          <div className="p-2">
            {documents.map((doc) => (
              <DocumentItem
                key={doc.id}
                document={doc}
                isSelected={selectedIds.includes(doc.id)}
                isActive={activeDocumentId === doc.id}
                onToggle={() => handleToggle(doc.id)}
                onOpen={onOpen ? () => onOpen(doc.id) : undefined}
                onRemove={onRemove ? () => onRemove(doc.id) : undefined}
              />
            ))}
          </div>
        )}
      </ScrollArea>

      {/* 底部提示 */}
      {selectedIds.length > 0 && (
        <div className="p-3 border-t bg-muted/30">
          <p className="text-xs text-muted-foreground text-center">
            选中的文档将用于 AI 问答检索
          </p>
        </div>
      )}
    </div>
  );
});
