// ============================================================
// NotebookMind - 聊天消息组件
// ============================================================

'use client';

import React, { memo, useMemo, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { User, Bot } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { SourceList } from './SourceBadge';
import type { ChatMessage as ChatMessageType, ChatSource } from '@/types/api';

// 来源正则表达式
const SOURCE_PATTERN = /\[Source:\s*([^,\]]+)(?:,\s*Page\s*(\d+))?\]/gi;

interface ParsedContent {
  type: 'text' | 'source';
  content: string;
  source?: {
    documentName: string;
    pageNumber?: number;
    source: ChatSource;
  };
}

/**
 * 解析消息内容，提取来源标记
 */
function parseMessageContent(
  content: string,
  sources: ChatSource[]
): ParsedContent[] {
  if (!content) return [];

  // 创建文档名到来源的映射
  const sourceMap = new Map<string, ChatSource>();
  sources.forEach((source) => {
    sourceMap.set(source.document_name.toLowerCase(), source);
  });

  const parts: ParsedContent[] = [];
  let lastIndex = 0;
  let match;

  // 重置正则
  SOURCE_PATTERN.lastIndex = 0;

  while ((match = SOURCE_PATTERN.exec(content)) !== null) {
    // 添加匹配前的文本
    if (match.index > lastIndex) {
      const text = content.slice(lastIndex, match.index);
      if (text.trim()) {
        parts.push({ type: 'text', content: text });
      }
    }

    const documentName = match[1].trim();
    const pageNumber = match[2] ? parseInt(match[2], 10) : undefined;
    const matchedSource = sourceMap.get(documentName.toLowerCase());

    // 添加来源标记
    parts.push({
      type: 'source',
      content: match[0],
      source: {
        documentName,
        pageNumber,
        source: matchedSource || {
          document_id: '',
          document_name: documentName,
          page_number: pageNumber || 0,
          chunk_index: 0,
          content: '',
          score: 0,
        },
      },
    });

    lastIndex = match.index + match[0].length;
  }

  // 添加剩余文本
  if (lastIndex < content.length) {
    const text = content.slice(lastIndex);
    if (text.trim()) {
      parts.push({ type: 'text', content: text });
    }
  }

  return parts;
}

interface ChatMessageProps {
  message: ChatMessageType;
  onSourceClick?: (source: ChatSource) => void;
  onSaveAsNote?: (message: ChatMessageType) => void;
  isStreaming?: boolean;
}

export const ChatMessage = memo(function ChatMessage({
  message,
  onSourceClick,
  onSaveAsNote,
  isStreaming = false,
}: ChatMessageProps) {
  const isUser = message.role === 'user';

  // 解析内容
  const parsedContent = useMemo(
    () => parseMessageContent(message.content, message.sources || []),
    [message.content, message.sources]
  );

  // 处理来源点击
  const handleSourceClick = useCallback(
    (source: ChatSource) => {
      onSourceClick?.(source);
    },
    [onSourceClick]
  );

  return (
    <div
      className={cn(
        'flex gap-3 p-4 animate-fade-in',
        isUser ? 'flex-row-reverse' : 'flex-row'
      )}
    >
      {/* 头像 */}
      <Avatar className="w-8 h-8 flex-shrink-0">
        {isUser ? (
          <>
            <AvatarImage src="/avatars/user.png" alt="User" />
            <AvatarFallback className="bg-primary text-primary-foreground">
              <User className="w-4 h-4" />
            </AvatarFallback>
          </>
        ) : (
          <>
            <AvatarImage src="/avatars/assistant.png" alt="AI" />
            <AvatarFallback className="bg-secondary">
              <Bot className="w-4 h-4" />
            </AvatarFallback>
          </>
        )}
      </Avatar>

      {/* 消息内容 */}
      <div
        className={cn(
          'flex flex-col gap-2 max-w-[80%]',
          isUser ? 'items-end' : 'items-start'
        )}
      >
        {/* 角色标签 */}
        <span className="text-xs text-muted-foreground px-1">
          {isUser ? 'You' : 'AI Assistant'}
        </span>

        {/* 气泡 */}
        <div
          className={cn(
            'relative px-4 py-3 rounded-2xl shadow-sm',
            isUser
              ? 'bg-primary text-primary-foreground rounded-br-md'
              : 'bg-muted rounded-bl-md'
          )}
        >
          {/* 消息文本（支持 Markdown） */}
          <div className="prose prose-sm max-w-none dark:prose-invert">
            {parsedContent.length > 0 ? (
              <div className="leading-relaxed">
                {parsedContent.map((part, index) => {
                  if (part.type === 'source' && part.source) {
                    return (
                      <span
                        key={index}
                        className={cn(
                          'inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-xs font-medium',
                          'bg-yellow-100/70 text-yellow-800 border border-yellow-200/50',
                          'hover:bg-yellow-200/80 cursor-pointer transition-colors',
                          onSourceClick && 'cursor-pointer'
                        )}
                        onClick={() => handleSourceClick(part.source!.source)}
                        title={part.source.source.content?.slice(0, 100)}
                      >
                        <FileTextIcon className="w-3 h-3" />
                        {part.source.documentName}
                        {part.source.pageNumber && `, P.${part.source.pageNumber}`}
                      </span>
                    );
                  }
                  // 文本部分，使用 Markdown 渲染
                  return (
                    <ReactMarkdown
                      key={index}
                      remarkPlugins={[remarkGfm]}
                      className="inline"
                    >
                      {part.content}
                    </ReactMarkdown>
                  );
                })}
                {/* 打字机光标 */}
                {isStreaming && (
                  <span className="inline-block w-0.5 h-4 ml-0.5 bg-current animate-pulse" />
                )}
              </div>
            ) : (
              <div className="leading-relaxed">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>
                  {message.content}
                </ReactMarkdown>
                {isStreaming && (
                  <span className="inline-block w-0.5 h-4 ml-0.5 bg-current animate-pulse" />
                )}
              </div>
            )}
          </div>
        </div>

        {/* 来源列表 */}
        {!isUser && message.sources && message.sources.length > 0 && (
          <SourceList
            sources={message.sources}
            onSourceClick={handleSourceClick}
            className="mt-1"
          />
        )}

        {/* 操作按钮 */}
        {!isUser && onSaveAsNote && (
          <Button
            variant="ghost"
            size="sm"
            className="text-xs text-muted-foreground hover:text-foreground"
            onClick={() => onSaveAsNote(message)}
          >
            <PinIcon className="w-3 h-3 mr-1" />
            存为笔记
          </Button>
        )}
      </div>
    </div>
  );
});

// ============================================================
// 图标组件
// ============================================================

function FileTextIcon({ className }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
      <polyline points="14 2 14 8 20 8" />
      <line x1="16" y1="13" x2="8" y2="13" />
      <line x1="16" y1="17" x2="8" y2="17" />
      <line x1="10" y1="9" x2="8" y2="9" />
    </svg>
  );
}

function PinIcon({ className }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      <line x1="12" y1="17" x2="12" y2="22" />
      <path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z" />
    </svg>
  );
}
