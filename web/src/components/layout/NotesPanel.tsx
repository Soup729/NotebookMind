// ============================================================
// NotebookMind - 右侧笔记面板组件
// ============================================================

'use client';

import React, { memo, useState, useCallback, useMemo } from 'react';
import {
  ChevronRight, ChevronLeft, Pin, PinOff, Search, Tag, Trash2, Pencil, Check, X, Expand, Shrink
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { useNotes, useTogglePinNote, useDeleteNote, useUpdateNote } from '@/hooks/useNotes';
import { formatDate } from '@/lib/utils';
import { useNotebookStore } from '@/store/useNotebookStore'; // 引入全局状态
import type { Note } from '@/types/api';

interface NotesPanelProps {
  isOpen: boolean;
  onToggle: () => void;
  notebookId?: string;
  className?: string;
}

// ============================================================
// 笔记卡片组件 — 支持展开/编辑/引用高亮跳转
// ============================================================

interface NoteCardProps {
  note: Note;
  onPinToggle: (noteId: string) => void;
  onDelete: (noteId: string) => void;
  onUpdate: (noteId: string, data: { title?: string; content?: string }) => Promise<boolean>;
}

const NoteCard = memo(function NoteCard({ note, onPinToggle, onDelete, onUpdate }: NoteCardProps) {
  const [expanded, setExpanded] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editTitle, setEditTitle] = useState(note.title);
  const [editContent, setEditContent] = useState(note.content);
  const [saving, setSaving] = useState(false);

  // 获取全局跳转动作
  const setMainViewToPdf = useNotebookStore((state) => state.setMainViewToPdf);

  const startEdit = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setEditTitle(note.title);
    setEditContent(note.content);
    setEditing(true);
    setExpanded(true);
  }, [note.title, note.content]);

  const saveEdit = useCallback(async (e?: React.MouseEvent | React.KeyboardEvent) => {
    e?.stopPropagation();
    const trimmedTitle = editTitle.trim();
    const trimmedContent = editContent.trim();
    if (!trimmedTitle) return;

    setSaving(true);
    try {
      const ok = await onUpdate(note.id, {
        title: trimmedTitle !== note.title ? trimmedTitle : undefined,
        content: trimmedContent !== note.content ? trimmedContent : undefined,
      });
      if (ok) setEditing(false);
    } finally {
      setSaving(false);
    }
  }, [note.id, note.title, note.content, editTitle, editContent, onUpdate]);

  const cancelEdit = useCallback((e?: React.MouseEvent | React.KeyboardEvent) => {
    e?.stopPropagation();
    setEditing(false);
    setEditTitle(note.title);
    setEditContent(note.content);
  }, [note.title, note.content]);

  const toggleExpand = useCallback(() => {
    if (!editing) setExpanded((prev) => !prev);
  }, [editing]);

  // 解析并渲染带有溯源标记的文本
  const renderContentWithCitations = useCallback((text: string) => {
    if (!text) return <span className="italic text-muted-foreground/50">(无内容)</span>;
    // 正则匹配 [来源: xxx, 页码: x]
    const parts = text.split(/(\[来源:.*?, 页码: \d+\])/g);
    
    return parts.map((part, idx) => {
      const match = part.match(/\[来源:(.*?), 页码: (\d+)\]/);
      if (match) {
        const [, docName, pageStr] = match;
        return (
          <Badge
            key={idx}
            variant="outline"
            className="mx-1 cursor-pointer hover:bg-primary hover:text-primary-foreground transition-colors align-middle"
            onClick={(e) => {
              e.stopPropagation();
              // 触发全局联动，跳转 PDF
              setMainViewToPdf('doc-id-placeholder', {
                pageNumber: parseInt(pageStr, 10),
                boundingBox: [], // 实际场景可通过 Note.metadata 还原 bbox
                sourceId: ''
              });
            }}
          >
            {docName.slice(0, 8)}... p.{pageStr}
          </Badge>
        );
      }
      return <span key={idx}>{part}</span>;
    });
  }, [setMainViewToPdf]);

  return (
    <article
      className={cn(
        'group rounded-lg border bg-card text-card-foreground shadow-sm transition-all hover:shadow-md flex flex-col cursor-pointer',
        expanded && 'ring-1 ring-primary/20',
        note.is_pinned && 'border-primary/50 bg-primary/5'
      )}
      onClick={toggleExpand}
    >
      {/* 头部区省略，修复了 propagation 和布局 */}
      <header className="flex items-start justify-between px-3 pt-3 pb-2 border-b border-transparent group-hover:border-border/50 transition-colors">
        <div className="flex items-center gap-2 min-w-0 flex-1">
          {note.is_pinned && <Pin className="w-3.5 h-3.5 text-primary shrink-0" />}
          {editing ? (
            <Input
              value={editTitle}
              onChange={(e) => setEditTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') saveEdit(e);
                if (e.key === 'Escape') cancelEdit(e);
              }}
              className="h-7 text-sm font-medium px-2 py-0"
              autoFocus
              onClick={(e) => e.stopPropagation()}
            />
          ) : (
            <h4 className={cn("text-sm font-medium leading-snug", !expanded && "truncate")}>
              {note.title}
            </h4>
          )}
        </div>

        <div className="flex items-center gap-1 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity ml-2">
          {editing ? (
            <>
              <Button size="icon" variant="ghost" className="h-6 w-6 text-green-600" onClick={saveEdit} disabled={saving}>
                <Check className="w-3.5 h-3.5" />
              </Button>
              <Button size="icon" variant="ghost" className="h-6 w-6 text-red-500" onClick={cancelEdit}>
                <X className="w-3.5 h-3.5" />
              </Button>
            </>
          ) : (
            <>
              <Button size="icon" variant="ghost" className="h-6 w-6" onClick={startEdit} title="编辑">
                <Pencil className="w-3.5 h-3.5" />
              </Button>
              <Button size="icon" variant="ghost" className="h-6 w-6" onClick={(e) => { e.stopPropagation(); onPinToggle(note.id); }} title={note.is_pinned ? "取消置顶" : "置顶"}>
                {note.is_pinned ? <PinOff className="w-3.5 h-3.5 text-primary" /> : <Pin className="w-3.5 h-3.5" />}
              </Button>
              <Button size="icon" variant="ghost" className="h-6 w-6 hover:text-red-500" onClick={(e) => { e.stopPropagation(); onDelete(note.id); }} title="删除">
                <Trash2 className="w-3.5 h-3.5" />
              </Button>
            </>
          )}
        </div>
      </header>

      {/* 内容区 */}
      {editing ? (
        <div className="px-3 py-2" onClick={(e) => e.stopPropagation()}>
          <Textarea
            value={editContent}
            onChange={(e) => setEditContent(e.target.value)}
            className="min-h-[120px] text-[13px] leading-relaxed resize-y"
            placeholder="笔记内容..."
          />
        </div>
      ) : (
        <div className="px-3 py-2">
          <p className={cn(
            "text-[13px] text-muted-foreground leading-relaxed break-words",
            !expanded ? "line-clamp-3" : ""
          )}>
            {renderContentWithCitations(note.content)}
          </p>
        </div>
      )}

      {/* 底部区：时间与标签 */}
      <footer className="px-3 pb-3 flex items-center justify-between mt-auto">
        <time className="text-[11px] text-muted-foreground/70">
          {formatDate(note.updated_at || note.created_at)}
        </time>
        {note.tags?.length > 0 && (
          <div className="flex flex-wrap gap-1 justify-end max-w-[60%]">
            {note.tags.slice(0, expanded ? undefined : 2).map((tag) => (
              <Badge key={tag} variant="secondary" className="px-1.5 py-0 text-[10px] h-4 leading-none font-normal">
                {tag}
              </Badge>
            ))}
            {!expanded && note.tags.length > 2 && (
              <Badge variant="secondary" className="px-1 py-0 text-[10px] h-4">+{note.tags.length - 2}</Badge>
            )}
          </div>
        )}
      </footer>
    </article>
  );
});

// ============================================================
// 笔记面板主容器
// ============================================================

export const NotesPanel = memo(function NotesPanel({ isOpen, onToggle, notebookId, className }: NotesPanelProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [activeTag, setActiveTag] = useState<string | null>(null);

  const { notes, isLoading, mutate } = useNotes({ notebook_id: notebookId });
  const { togglePin } = useTogglePinNote();
  const { deleteNote } = useDeleteNote();
  const { updateNote } = useUpdateNote();

  // 【优化】使用 useMemo 缓存复杂计算，避免输入搜索词时卡顿
  const { filteredNotes, allTags } = useMemo(() => {
    const query = searchQuery.toLowerCase();
    const tags = new Set<string>();
    
    const filtered = notes.filter((note) => {
      note.tags?.forEach(t => tags.add(t)); // 顺便收集全部 Tag
      const matchesSearch = !query || note.title.toLowerCase().includes(query) || note.content.toLowerCase().includes(query);
      const matchesTag = !activeTag || note.tags?.includes(activeTag);
      return matchesSearch && matchesTag;
    });

    const sorted = filtered.sort((a, b) => {
      if (a.is_pinned !== b.is_pinned) return a.is_pinned ? -1 : 1;
      return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
    });

    return { filteredNotes: sorted, allTags: Array.from(tags) };
  }, [notes, searchQuery, activeTag]);

  const handlePinToggle = useCallback(async (id: string) => { await togglePin(id); mutate(); }, [togglePin, mutate]);
  const handleDelete = useCallback(async (id: string) => { await deleteNote(id); mutate(); }, [deleteNote, mutate]);
  const handleUpdate = useCallback(async (id: string, data: any) => {
    const ok = await updateNote(id, data);
    if (ok) mutate();
    return ok;
  }, [updateNote, mutate]);

  return (
    <>
      {/* 【优化】使用 translate-x 动画而不是修改 w-xx，修复重绘卡顿 */}
      <aside
        className={cn(
          'fixed right-0 top-0 bottom-0 z-40 w-96 bg-background border-l shadow-2xl transition-transform duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] flex flex-col',
          !isOpen && 'translate-x-full',
          className
        )}
      >
        <header className="flex items-center justify-between px-4 py-3.5 border-b shrink-0">
          <h2 className="font-semibold text-sm flex items-center gap-2">
            <Pin className="w-4 h-4 text-primary" /> 研究笔记
          </h2>
          <Button variant="ghost" size="icon" onClick={onToggle} className="h-8 w-8 text-muted-foreground hover:text-foreground">
            <ChevronRight className="w-5 h-5" />
          </Button>
        </header>

        <div className="px-4 py-3 border-b shrink-0 space-y-3">
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <Input
              placeholder="搜索笔记标题或内容..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-8 h-9 text-sm bg-muted/50 focus-visible:bg-background transition-colors"
            />
          </div>
          
          {allTags.length > 0 && (
            <div className="flex items-center gap-1.5 flex-wrap">
              <Tag className="w-3.5 h-3.5 text-muted-foreground/70 shrink-0" />
              {allTags.map((tag) => (
                <Badge
                  key={tag}
                  variant={activeTag === tag ? "default" : "secondary"}
                  className="cursor-pointer font-normal text-[11px] px-2 py-0.5"
                  onClick={() => setActiveTag(activeTag === tag ? null : tag)}
                >
                  {tag}
                </Badge>
              ))}
            </div>
          )}
        </div>

        <main className="flex-1 overflow-y-auto p-4 space-y-3 bg-muted/10">
          {/* 【优化】防止后台 mutate 时导致界面闪烁，只在首次加载时显示 Skeleton */}
          {isLoading && notes.length === 0 ? (
            Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="rounded-lg border bg-card p-4 space-y-3">
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-3 w-full" />
                <Skeleton className="h-3 w-5/6" />
              </div>
            ))
          ) : filteredNotes.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-muted-foreground space-y-3">
              <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center">
                <Pin className="w-6 h-6 opacity-50" />
              </div>
              <p className="text-sm">{searchQuery ? '未搜索到相关笔记' : '暂无研究笔记'}</p>
            </div>
          ) : (
            filteredNotes.map((note) => (
              <NoteCard
                key={note.id}
                note={note}
                onPinToggle={handlePinToggle}
                onDelete={handleDelete}
                onUpdate={handleUpdate}
              />
            ))
          )}
        </main>
      </aside>

      {/* 折叠入口按钮 (固定在右侧中间) */}
      {!isOpen && (
        <Button
          variant="outline"
          size="sm"
          onClick={onToggle}
          className="fixed right-0 top-1/2 -translate-y-1/2 z-30 rounded-r-none rounded-l-xl pr-3 pl-2 h-12 shadow-lg border-r-0 hover:w-16 w-12 transition-all group overflow-hidden"
        >
          <div className="flex items-center justify-between w-full">
            <ChevronLeft className="w-4 h-4 text-muted-foreground group-hover:-translate-x-1 transition-transform" />
            <Pin className="w-4 h-4" />
          </div>
        </Button>
      )}
    </>
  );
});