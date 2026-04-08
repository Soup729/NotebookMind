// ============================================================
// Enterprise PDF AI - 聊天面板组件 (核心)
// ============================================================

'use client';

import React, {
  useState,
  useRef,
  useCallback,
  useEffect,
} from 'react';
import type { KeyboardEvent } from 'react';
import { Send, Loader2, Plus, MessageSquare } from 'lucide-react';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Card } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { ChatMessage } from './ChatMessage';
import { useChat, useSourceCitation } from '@/hooks/useChat';
import { useCreateNote } from '@/hooks/useNotes';
import { useNotebookStore } from '@/store/useNotebookStore';
import type { ChatMessage as ChatMessageType, ChatSource, Session } from '@/types/api';

// ============================================================
// 类型定义
// ============================================================

interface ChatPanelProps {
  notebookId: string;
  sessionId: string | null;
  onSessionCreate?: (session: Session) => void;
  className?: string;
}

interface SuggestedQueriesProps {
  queries: string[];
  onQueryClick: (query: string) => void;
  isLoading?: boolean;
}

// ============================================================
// 建议问题组件
// ============================================================

function SuggestedQueries({ queries, onQueryClick, isLoading }: SuggestedQueriesProps) {
  if (isLoading) {
    return (
      <div className="flex flex-col gap-2 p-4">
        <Skeleton className="h-4 w-32" />
        <div className="flex gap-2">
          <Skeleton className="h-8 w-24" />
          <Skeleton className="h-8 w-28" />
          <Skeleton className="h-8 w-20" />
        </div>
      </div>
    );
  }

  if (!queries || queries.length === 0) return null;

  return (
    <div className="flex flex-col gap-2 p-4 border-t bg-muted/30">
      <p className="text-xs text-muted-foreground font-medium">Suggested Questions</p>
      <div className="flex flex-wrap gap-2">
        {queries.map((query, index) => (
          <button
            key={index}
            onClick={() => onQueryClick(query)}
            className={cn(
              'px-3 py-1.5 text-sm rounded-full',
              'bg-secondary text-secondary-foreground',
              'hover:bg-secondary/80 transition-colors',
              'border border-transparent hover:border-primary/20'
            )}
          >
            {query}
          </button>
        ))}
      </div>
    </div>
  );
}

// ============================================================
// 空状态组件
// ============================================================

function EmptyState({ onNewChat, hasDocuments }: { onNewChat?: () => void; hasDocuments?: boolean }) {
  return (
    <div className="flex flex-col items-center justify-center h-full p-8 text-center">
      <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center mb-4">
        <MessageSquare className="w-8 h-8 text-muted-foreground" />
      </div>
      <h3 className="text-lg font-medium mb-2">开始新对话</h3>
      <p className="text-sm text-muted-foreground mb-4 max-w-xs">
        {hasDocuments
          ? '输入问题开始对话。可选择文档进行基于文档的精准回答，或直接提问使用通用知识'
          : '输入问题开始 AI 对话'}
      </p>
      {onNewChat && (
        <Button variant="outline" size="sm" onClick={onNewChat}>
          <Plus className="w-4 h-4 mr-2" />
          新建会话
        </Button>
      )}
    </div>
  );
}

// ============================================================
// 聊天面板组件
// ============================================================

export function ChatPanel({
  notebookId,
  sessionId,
  onSessionCreate,
  className,
}: ChatPanelProps) {
  // State
  const [inputValue, setInputValue] = useState('');
  const [suggestedQueries] = useState<string[]>([
    '总结这篇文档的主要内容',
    '文档中有哪些关键要点？',
    '请解释文中的核心概念',
  ]);
  // 待发送消息：用于无会话时先创建会话再自动发送
  const [pendingMessage, setPendingMessage] = useState<string | null>(null);
  const [isCreatingSession, setIsCreatingSession] = useState(false);

  // Refs
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const chatContainerRef = useRef<HTMLDivElement>(null);

  // Store
  const selectedDocumentIds = useNotebookStore((state) => state.selectedDocumentIds);
  const setMainViewToPdf = useNotebookStore((state) => state.setMainViewToPdf);

  // Hooks
  const {
    messages,
    isStreaming,
    isLoadingHistory,
    currentContent,
    currentSources,
    sendMessage,
    clearMessages,
  } = useChat(notebookId, sessionId);

  const { createNote } = useCreateNote();
  const { handleSourceClick } = useSourceCitation();

  // ============================================================
  // 滚动到底部
  // ============================================================

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, currentContent, scrollToBottom]);

  // ============================================================
  // 发送消息
  // ============================================================

  const handleSendMessage = useCallback(async () => {
    const trimmedValue = inputValue.trim();
    if (!trimmedValue || isStreaming) return;

    // 如果没有会话，先创建新会话，再自动发送消息
    if (!sessionId && onSessionCreate) {
      setInputValue('');
      setPendingMessage(trimmedValue);
      setIsCreatingSession(true);
      onSessionCreate();
      return;
    }

    setInputValue('');
    await sendMessage(trimmedValue, selectedDocumentIds);
  }, [inputValue, isStreaming, sessionId, onSessionCreate, sendMessage, selectedDocumentIds]);

  // ============================================================
  // 当 sessionId 从 null 变为有值时，如果有待发送消息则自动发送
  // ============================================================
  useEffect(() => {
    if (sessionId && pendingMessage) {
      // 延迟发送，确保 useChat 的 useEffect 已完成历史加载
      const timer = setTimeout(() => {
        sendMessage(pendingMessage, selectedDocumentIds);
        setPendingMessage(null);
        setIsCreatingSession(false);
      }, 150);
      return () => clearTimeout(timer);
    }
  }, [sessionId, pendingMessage, sendMessage, selectedDocumentIds]);

  // ============================================================
  // 键盘事件
  // ============================================================

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSendMessage();
      }
    },
    [handleSendMessage]
  );

  // 自动调整 textarea 高度
  useEffect(() => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      textarea.style.height = `${Math.min(textarea.scrollHeight, 120)}px`;
    }
  }, [inputValue]);

  // ============================================================
  // 来源点击处理
  // ============================================================

  const onSourceClick = useCallback(
    (source: ChatSource) => {
      // 使用 store 的方法切换到 PDF 视图
      setMainViewToPdf(source.document_id, {
        pageNumber: source.page_number,
        boundingBox: [0, 0, 0, 0],
        sourceId: source.document_id,
        documentId: source.document_id,
        documentName: source.document_name,
        content: source.content,
      });
    },
    [setMainViewToPdf]
  );

  // ============================================================
  // 存为笔记
  // ============================================================

  const handleSaveAsNote = useCallback(
    async (message: ChatMessageType) => {
      // 清理 sources 数据，确保数值字段正确
      const cleanedSources = (message.sources || []).map((source) => ({
        document_id: String(source.document_id),
        document_name: String(source.document_name),
        page_number: Number(source.page_number),
        chunk_index: Number(source.chunk_index),
        content: String(source.content),
        score: Number(source.score),
      }));

      const note = await createNote({
        notebook_id: notebookId,
        session_id: sessionId || undefined,
        title: message.content.slice(0, 50) + (message.content.length > 50 ? '...' : ''),
        content: message.content,
        type: 'ai_response',
        tags: [],
        metadata: {
          sources: cleanedSources,
        },
      });

      if (note) {
        toast.success('已保存为笔记');
      }
    },
    [notebookId, sessionId, createNote]
  );

  // ============================================================
  // 建议问题点击
  // ============================================================

  const handleSuggestedQueryClick = useCallback(
    (query: string) => {
      setInputValue(query);
      textareaRef.current?.focus();
    },
    []
  );

  // ============================================================
  // 新建会话
  // ============================================================

  const handleNewChat = useCallback(() => {
    if (messages.length > 0) {
      clearMessages();
    }
    onSessionCreate?.();
  }, [messages.length, clearMessages, onSessionCreate]);

  // ============================================================
  // 判断是否显示空状态
  // ============================================================

  const showEmptyState = messages.length === 0 && !isStreaming && !isLoadingHistory;

  return (
    <div
      className={cn(
        'flex flex-col h-full bg-background rounded-none border-0 shadow-lg',
        className
      )}
    >
      {/* 头部 */}
      <div className="flex items-center justify-between px-4 py-3 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-center gap-2">
          <MessageSquare className="w-5 h-5 text-muted-foreground" />
          <h2 className="font-semibold">AI 对话</h2>
        </div>
        <Button variant="ghost" size="sm" onClick={handleNewChat}>
          <Plus className="w-4 h-4 mr-1" />
          新对话
        </Button>
      </div>

      {/* 消息列表 */}
      <div
        ref={chatContainerRef}
        className="flex-1 overflow-y-auto custom-scrollbar"
      >
        {showEmptyState ? (
          <EmptyState onNewChat={handleNewChat} hasDocuments={selectedDocumentIds.length > 0} />
        ) : (
          <div className="flex flex-col">
            {/* 历史消息加载指示器 */}
            {isLoadingHistory && messages.length === 0 && (
              <div className="flex flex-col items-center justify-center py-12 gap-3">
                <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
                <p className="text-sm text-muted-foreground">加载对话记录...</p>
              </div>
            )}

            {/* 历史消息 */}
            {messages.map((message) => (
              <ChatMessage
                key={message.id}
                message={message}
                onSourceClick={onSourceClick}
                onSaveAsNote={handleSaveAsNote}
              />
            ))}

            {/* 流式内容 */}
            {isStreaming && currentContent && (
              <ChatMessage
                message={{
                  id: 'streaming',
                  session_id: sessionId || '',
                  role: 'assistant',
                  content: currentContent,
                  sources: currentSources,
                  created_at: new Date().toISOString(),
                }}
                onSourceClick={onSourceClick}
                isStreaming
              />
            )}

            {/* 加载指示器 */}
            {isStreaming && !currentContent && (
              <div className="flex gap-3 p-4">
                <Skeleton className="w-8 h-8 rounded-full" />
                <div className="flex flex-col gap-2 flex-1">
                  <Skeleton className="h-4 w-48" />
                  <Skeleton className="h-4 w-32" />
                </div>
              </div>
            )}

            {/* 滚动锚点 */}
            <div ref={messagesEndRef} />
          </div>
        )}
      </div>

      {/* 建议问题 */}
      {showEmptyState && (
        <SuggestedQueries
          queries={suggestedQueries}
          onQueryClick={handleSuggestedQueryClick}
        />
      )}

      {/* 输入区域 */}
      <div className="p-4 border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-end gap-2">
          {/* 输入框 */}
          <div className="relative flex-1">
            <Textarea
              ref={textareaRef}
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="输入问题... (Shift+Enter 换行)"
              className="pr-12 resize-none min-h-[44px] max-h-[120px]"
              disabled={isStreaming}
              rows={1}
            />
          </div>

          {/* 发送按钮 */}
          <Button
            size="icon"
            onClick={handleSendMessage}
            disabled={!inputValue.trim() || isStreaming}
            className="h-11 w-11 flex-shrink-0"
          >
            {isStreaming ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Send className="w-4 h-4" />
            )}
          </Button>
        </div>

        {/* 底部提示 */}
        <p className="text-xs text-muted-foreground mt-2 text-center">
          {selectedDocumentIds.length > 0
            ? `基于 ${selectedDocumentIds.length} 个文档进行 RAG 精准回答`
            : '通用 AI 对话（选择文档可启用文档增强模式）'}
        </p>
      </div>
    </div>
  );
}

// ============================================================
// 导出
// ============================================================

export default ChatPanel;
