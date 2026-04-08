// ============================================================
// Enterprise PDF AI - 聊天 SSE Hook
// ============================================================

'use client';

import { useCallback, useRef, useState, useEffect } from 'react';
import { toast } from 'sonner';
import { streamChat, apiFetch, API_ENDPOINTS, SSEChunk, parseSourceCitations } from '@/lib/utils';
import { useNotebookStore } from '@/store/useNotebookStore';
import type { ChatMessage, ChatSource, HighlightTarget } from '@/types/api';

// ============================================================
// 类型定义
// ============================================================

export interface UseChatOptions {
  onMessageStart?: () => void;
  onMessageEnd?: (fullContent: string) => void;
  onSourcesUpdate?: (sources: ChatSource[]) => void;
}

export interface UseChatReturn {
  // 状态
  messages: ChatMessage[];
  isStreaming: boolean;
  isLoadingHistory: boolean;
  currentContent: string;
  currentSources: ChatSource[];

  // 方法
  sendMessage: (question: string, documentIds?: string[]) => Promise<void>;
  clearMessages: () => void;
  regenerateLastMessage: () => Promise<void>;
}

// ============================================================
// Chat Hook 实现
// ============================================================

export function useChat(notebookId: string, sessionId: string | null): UseChatReturn {
  // 消息列表
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [isLoadingHistory, setIsLoadingHistory] = useState(false);
  const [currentContent, setCurrentContent] = useState('');
  const [currentSources, setCurrentSources] = useState<ChatSource[]>([]);

  // Refs
  const abortControllerRef = useRef<AbortController | null>(null);
  const lastMessageRef = useRef<ChatMessage | null>(null);

  // Store actions
  const setMainViewToPdf = useNotebookStore((state) => state.setMainViewToPdf);

  // ============================================================
  // 切换会话时加载历史消息
  // ============================================================

  useEffect(() => {
    if (!sessionId || !notebookId) {
      setMessages([]);
      return;
    }

    let cancelled = false;
    setIsLoadingHistory(true);

    const loadMessages = async () => {
      const token = localStorage.getItem('auth_token');
      if (!token) {
        if (!cancelled) {
          setMessages([]);
          setIsLoadingHistory(false);
        }
        return;
      }

      try {
        const data = await apiFetch<{ items: ChatMessage[] }>(
          API_ENDPOINTS.sessionMessages(sessionId),
          { token }
        );

        if (!cancelled) {
          setMessages(data.items || []);
          setIsLoadingHistory(false);
        }
      } catch (err) {
        if (!cancelled) {
          const msg = err instanceof Error ? err.message : String(err);
          // 新建的会话可能还没有历史记录，后端可能返回 not found / session not found
          // 这种情况属于正常现象，静默处理为空消息列表
          const isNewSessionError =
            msg.toLowerCase().includes('not found') ||
            msg.toLowerCase().includes('session');
          if (isNewSessionError) {
            console.debug('新建会话无历史记录:', msg);
            setMessages([]);
          } else {
            console.error('Failed to load messages:', err);
            setMessages([]);
          }
          setIsLoadingHistory(false);
        }
      }
    };

    loadMessages();

    return () => { cancelled = true; };
  }, [notebookId, sessionId]);

  // ============================================================
  // 发送消息
  // ============================================================

  const sendMessage = useCallback(
    async (question: string, documentIds?: string[]) => {
      // 检查 session — 如果无 session 则静默返回（由调用方 ChatPanel 负责自动创建）
      if (!sessionId) {
        return;
      }

      // 取消之前的请求
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      // 创建新的 AbortController
      abortControllerRef.current = new AbortController();

      // 添加用户消息
      const userMessage: ChatMessage = {
        id: `user-${Date.now()}`,
        session_id: sessionId,
        role: 'user',
        content: question,
        sources: [],
        created_at: new Date().toISOString(),
      };

      setMessages((prev) => [...prev, userMessage]);
      setCurrentContent('');
      setCurrentSources([]);
      setIsStreaming(true);

      // 获取 token
      const token = localStorage.getItem('auth_token');
      if (!token) {
        toast.error('请先登录');
        setIsStreaming(false);
        return;
      }

      try {
        let accumulatedContent = '';
        let finalSources: ChatSource[] = [];

        await streamChat(
          API_ENDPOINTS.chat(notebookId, sessionId),
          {
            question,
            document_ids: documentIds,
          },
          {
            token,
            onChunk: (chunk: SSEChunk) => {
              accumulatedContent = chunk.content;
              finalSources = chunk.sources || [];

              setCurrentContent(chunk.content);
              setCurrentSources(chunk.sources || []);
            },
            onDone: (content: string) => {
              // 创建 assistant 消息
              const assistantMessage: ChatMessage = {
                id: `assistant-${Date.now()}`,
                session_id: sessionId,
                role: 'assistant',
                content,
                sources: finalSources,
                created_at: new Date().toISOString(),
              };

              lastMessageRef.current = assistantMessage;
              setMessages((prev) => [...prev, assistantMessage]);
              setCurrentContent('');
              setCurrentSources([]);
              setIsStreaming(false);
            },
            onError: (error: Error) => {
              const isSessionGone = 'sessionGone' in error && (error as { sessionGone: boolean }).sessionGone;
              if (isSessionGone) {
                toast.error('对话已失效', { description: '该对话已被删除或不存在' });
              } else {
                toast.error('发送消息失败', { description: error.message });
              }
              setIsStreaming(false);
              setCurrentContent('');
              setCurrentSources([]);
            },
          }
        );
      } catch (error) {
        const err = error instanceof Error ? error : new Error('发送失败');
        if (err.message !== 'Aborted') {
          // 检测 SESSION_GONE（会话已被删除/不存在）
          const isSessionGone = 'sessionGone' in err && (err as { sessionGone: boolean }).sessionGone;
          if (isSessionGone) {
            toast.error('对话已失效', {
              description: '该对话已被删除或不存在，请创建新对话',
              duration: 5000,
            });
          } else {
            toast.error('发送消息失败', { description: err.message });
          }
        }
        setIsStreaming(false);
      }
    },
    [notebookId, sessionId]
  );

  // ============================================================
  // 清除消息
  // ============================================================

  const clearMessages = useCallback(() => {
    setMessages([]);
    setCurrentContent('');
    setCurrentSources([]);
    lastMessageRef.current = null;
  }, []);

  // ============================================================
  // 重新生成上一条消息
  // ============================================================

  const regenerateLastMessage = useCallback(async () => {
    if (!lastMessageRef.current) return;

    // 移除最后一条 assistant 消息
    setMessages((prev) => prev.slice(0, -1));

    // 找到对应的用户消息重新发送
    const userMessage = messages[messages.length - 2];
    if (userMessage && userMessage.role === 'user') {
      await sendMessage(userMessage.content);
    }
  }, [messages, sendMessage]);

  return {
    messages,
    isStreaming,
    isLoadingHistory,
    currentContent,
    currentSources,
    sendMessage,
    clearMessages,
    regenerateLastMessage,
  };
}

// ============================================================
// 来源 Badge 点击处理 Hook
// ============================================================

export function useSourceCitation() {
  const setMainViewToPdf = useNotebookStore((state) => state.setMainViewToPdf);
  const setMainViewToGuide = useNotebookStore((state) => state.setMainViewToGuide);

  /**
   * 处理来源 Badge 点击
   */
  const handleSourceClick = useCallback(
    (source: ChatSource) => {
      // 构建高亮目标
      const highlightTarget: HighlightTarget = {
        pageNumber: source.page_number,
        boundingBox: [0, 0, 0, 0], // 后端暂未提供精确坐标，使用全区域
        sourceId: source.document_id,
        documentId: source.document_id,
        documentName: source.document_name,
        content: source.content,
      };

      setMainViewToPdf(source.document_id, highlightTarget);
    },
    [setMainViewToPdf]
  );

  /**
   * 处理来源引用文本点击（用于 Markdown 解析后的文本）
   */
  const handleCitationClick = useCallback(
    (documentName: string, pageNumber?: number) => {
      // 这里需要通过文档名查找文档 ID
      // 暂时通过全局状态或 API 获取
      // 简化处理：只切换到指南视图
      setMainViewToGuide();
    },
    [setMainViewToGuide]
  );

  return {
    handleSourceClick,
    handleCitationClick,
  };
}

// ============================================================
// Markdown 来源解析 Hook
// ============================================================

export function useSourceParsing() {
  /**
   * 解析消息内容中的来源标记
   */
  const parseMessageWithSources = useCallback((content: string, sources: ChatSource[]) => {
    if (!sources.length) {
      return [{ type: 'text' as const, content }];
    }

    // 创建文档名到来源的映射
    const sourceMap = new Map<string, ChatSource>();
    sources.forEach((source) => {
      sourceMap.set(source.document_name, source);
    });

    // 使用工具函数解析
    const { parts } = parseSourceCitations(content);

    return parts.map((part) => {
      if (part.type === 'source' && part.source) {
        const matchedSource = sourceMap.get(part.source.documentName);
        return {
          ...part,
          matchedSource,
        };
      }
      return part;
    });
  }, []);

  /**
   * 渲染带来源的消息内容（返回 React 节点数据）
   */
  const renderMessageWithSources = useCallback(
    (content: string, sources: ChatSource[]) => {
      const parts = parseMessageWithSources(content, sources);

      return parts.map((part, index) => {
        if (part.type === 'source') {
          return {
            type: 'source' as const,
            text: part.content,
            source: part.matchedSource || sources[0],
            key: `source-${index}`,
          };
        }
        return {
          type: 'text' as const,
          text: part.content,
          key: `text-${index}`,
        };
      });
    },
    [parseMessageWithSources]
  );

  return {
    parseMessageWithSources,
    renderMessageWithSources,
  };
}

// ============================================================
// 流式消息 Hook（用于逐字显示）
// ============================================================

export function useStreamingMessage(initialContent = '') {
  const [content, setContent] = useState(initialContent);
  const [displayedContent, setDisplayedContent] = useState(initialContent);

  // 增量更新
  const appendContent = useCallback((chunk: string) => {
    setContent((prev) => prev + chunk);
    setDisplayedContent((prev) => prev + chunk);
  }, []);

  // 完全替换
  const replaceContent = useCallback((newContent: string) => {
    setContent(newContent);
    setDisplayedContent(newContent);
  }, []);

  // 重置
  const reset = useCallback(() => {
    setContent('');
    setDisplayedContent('');
  }, []);

  return {
    content,
    displayedContent,
    appendContent,
    replaceContent,
    reset,
  };
}
