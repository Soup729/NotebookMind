// ============================================================
// Enterprise PDF AI - 右侧笔记面板组件
// ============================================================

'use client';

import React, { memo, useState, useCallback } from 'react';
import {
  ChevronRight,
  ChevronLeft,
  Pin,
  PinOff,
  Search,
  Tag,
  Trash2,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useNotes, useTogglePinNote, useDeleteNote } from '@/hooks/useNotes';
import { formatDate } from '@/lib/utils';
import type { Note } from '@/types/api';

interface NotesPanelProps {
  isOpen: boolean;
  onToggle: () => void;
  notebookId?: string;
  className?: string;
}

// ============================================================
// 笔记卡片组件 — 使用 flex 布局确保内容不溢出
// ============================================================

interface NoteCardProps {
  note: Note;
  onPinToggle: (noteId: string) => void;
  onDelete: (noteId: string) => void;
}

const NoteCard = memo(function NoteCard({
  note,
  onPinToggle,
  onDelete,
}: NoteCardProps) {
  return (
    <article
      className={cn(
        'group rounded-lg border bg-card text-card-foreground shadow-sm transition-all hover:shadow-md',
        'flex flex-col', // 关键：使用纵向 flex 布局，子元素自然填充宽度
        note.is_pinned && 'border-primary/50 bg-primary/5'
      )}
    >
      {/* 头部：标题 + 操作按钮（横向排列） */}
      <header className="flex items-start gap-1.5 px-3 pt-3 pb-0">
        {/* 标题区 */}
        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-1 min-w-0">
            {note.is_pinned && (
              <Pin className="w-3 h-3 text-primary flex-shrink-0" aria-hidden />
            )}
            <h4 className="text-sm font-medium leading-snug truncate" title={note.title}>
              {note.title}
            </h4>
          </div>
          <time className="block text-[11px] text-muted-foreground mt-0.5 leading-none">
            {formatDate(note.created_at)}
          </time>
        </div>

        {/* 操作按钮组 */}
        <nav className="flex items-center gap-0 flex-shrink-0 ml-1" aria-label="笔记操作">
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0"
            onClick={(e) => {
              e.stopPropagation();
              onPinToggle(note.id);
            }}
            aria-label={note.is_pinned ? '取消钉住' : '钉住'}
          >
            {note.is_pinned ? (
              <PinOff className="w-3 h-3" aria-hidden />
            ) : (
              <Pin className="w-3 h-3" aria-hidden />
            )}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0 text-muted-foreground/60 hover:text-destructive hover:bg-destructive/10"
            onClick={(e) => {
              e.stopPropagation();
              onDelete(note.id);
            }}
            aria-label="删除笔记"
          >
            <Trash2 className="w-3 h-3" aria-hidden />
          </Button>
        </nav>
      </header>

      {/* 内容区 */}
      <p
        className="text-[13px] text-muted-foreground leading-relaxed line-clamp-3 break-words px-3 py-2"
        title={note.content}
      >
        {note.content}
      </p>

      {/* 标签 */}
      {note.tags && note.tags.length > 0 && (
        <footer className="px-3 pb-2.5">
          <ul className="flex flex-wrap gap-1" role="list">
            {note.tags.map((tag) => (
              <li key={tag}>
                <span className="inline-block px-1.5 py-0 text-[11px] leading-tight rounded-full bg-secondary/70 text-secondary-foreground select-none">
                  {tag}
                </span>
              </li>
            ))}
          </ul>
        </footer>
      )}
    </article>
  );
});

// ============================================================
// 笔记面板主容器
// ============================================================

export const NotesPanel = memo(function NotesPanel({
  isOpen,
  onToggle,
  notebookId,
  className,
}: NotesPanelProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [activeTag, setActiveTag] = useState<string | null>(null);

  // 数据获取
  const { notes, isLoading, mutate } = useNotes({ notebook_id: notebookId });

  const { togglePin } = useTogglePinNote();
  const { deleteNote } = useDeleteNote();

  // 过滤笔记
  const filteredNotes = notes.filter((note) => {
    const matchesSearch =
      !searchQuery ||
      note.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
      note.content.toLowerCase().includes(searchQuery.toLowerCase());
    const matchesTag = !activeTag || note.tags?.includes(activeTag);
    return matchesSearch && matchesTag;
  });

  // 所有标签
  const allTags = Array.from(
    new Set(notes.flatMap((note) => note.tags || []))
  );

  // 操作处理
  const handlePinToggle = useCallback(
    async (noteId: string) => {
      await togglePin(noteId);
      mutate();
    },
    [togglePin, mutate]
  );

  const handleDelete = useCallback(
    async (noteId: string) => {
      await deleteNote(noteId);
      mutate();
    },
    [deleteNote, mutate]
  );

  // 钉住优先排序
  const sortedNotes = [...filteredNotes].sort((a, b) => {
    if (a.is_pinned && !b.is_pinned) return -1;
    if (!a.is_pinned && b.is_pinned) return 1;
    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
  });

  return (
    <>
      {/* 展开的抽屉 */}
      <aside
        className={cn(
          'h-full flex flex-col border-l bg-background transition-all duration-300 ease-in-out',
          isOpen ? 'w-80' : 'w-0 overflow-hidden border-transparent',
          className
        )}
        role="complementary"
        aria-label="研究笔记面板"
      >
        {/* 头部 */}
        <header className="flex items-center justify-between px-4 py-3 border-b shrink-0">
          <h2 className="font-semibold text-sm flex items-center gap-2">
            <Pin className="w-4 h-4" aria-hidden />
            研究笔记
          </h2>
          <Button variant="ghost" size="icon" onClick={onToggle} className="h-8 w-8" aria-label="收起笔记面板">
            <ChevronRight className="w-4 h-4" aria-hidden />
          </Button>
        </header>

        {/* 搜索 */}
        <div className="px-4 py-2.5 border-b shrink-0">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground pointer-events-none" aria-hidden />
            <Input
              placeholder="搜索笔记..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 h-9 text-sm"
              aria-label="搜索笔记"
            />
          </div>
        </div>

        {/* 标签筛选 */}
        {allTags.length > 0 && (
          <nav className="px-4 py-2 border-b shrink-0" aria-label="标签筛选">
            <div className="flex items-center gap-1.5 flex-wrap">
              <Tag className="w-3 h-3 text-muted-foreground flex-shrink-0" aria-hidden />
              {allTags.map((tag) => (
                <button
                  key={tag}
                  onClick={() => setActiveTag(activeTag === tag ? null : tag)}
                  className={cn(
                    'px-2 py-0.5 text-xs rounded-full transition-colors cursor-pointer',
                    activeTag === tag
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-secondary/70 text-secondary-foreground hover:bg-secondary'
                  )}
                  aria-pressed={activeTag === tag}
                >
                  {tag}
                </button>
              ))}
            </div>
          </nav>
        )}

        {/* 笔记列表 — 使用原生 overflow 替代 Radix ScrollArea 解决裁剪问题 */}
        <main
          className="flex-1 overflow-y-auto overscroll-contain"
          style={{ WebkitOverflowScrolling: 'touch' }}
        >
          <div className="p-3 space-y-2.5 w-full">
            {isLoading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <div
                  key={i}
                  className="rounded-lg border bg-card shadow-sm p-3 animate-pulse"
                  aria-hidden
                >
                  <Skeleton className="h-4 w-3/4 mb-2" />
                  <Skeleton className="h-3 w-1/2 mb-3" />
                  <Skeleton className="h-3 w-full mb-1.5" />
                  <Skeleton className="h-3 w-2/3" />
                </div>
              ))
            ) : sortedNotes.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <Pin className="w-8 h-8 text-muted-foreground/40 mb-3" aria-hidden />
                <p className="text-sm font-medium text-muted-foreground">
                  {searchQuery || activeTag ? '未找到匹配的笔记' : '暂无笔记'}
                </p>
                {!searchQuery && !activeTag && (
                  <p className="text-xs text-muted-foreground mt-1.5 max-w-[200px]">
                    在对话中点击消息右侧的 📌 图标即可存为笔记
                  </p>
                )}
              </div>
            ) : (
              sortedNotes.map((note) => (
                <NoteCard
                  key={note.id}
                  note={note}
                  onPinToggle={handlePinToggle}
                  onDelete={handleDelete}
                />
              ))
            )}
          </div>
        </main>

        {/* 底部统计 */}
        <footer className="px-4 py-2 border-t text-xs text-muted-foreground shrink-0">
          <span>{sortedNotes.length} 条笔记</span>
          {notes.filter((n) => n.is_pinned).length > 0 && (
            <span> · {notes.filter((n) => n.is_pinned).length} 条已钉住</span>
          )}
        </footer>
      </aside>

      {/* 折叠时的入口按钮 */}
      {!isOpen && (
        <Button
          variant="outline"
          size="sm"
          onClick={onToggle}
          className={cn(
            'fixed right-4 top-1/2 -translate-y-1/2 z-10',
            'flex items-center gap-1.5 px-3 h-9 shadow-md'
          )}
          aria-label="展开笔记面板"
        >
          <ChevronLeft className="w-4 h-4" aria-hidden />
          <Pin className="w-3.5 h-3.5" aria-hidden />
          {notes.length > 0 && (
            <span className="text-xs tabular-nums">{notes.length}</span>
          )}
        </Button>
      )}
    </>
  );
});
