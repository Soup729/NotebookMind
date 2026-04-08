// ============================================================
// Enterprise PDF AI - 工具库
// ============================================================

import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

// ============================================================
// 样式合并
// ============================================================

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// ============================================================
// API 基础配置
// ============================================================

export const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

export const API_ENDPOINTS = {
  // 认证
  login: '/auth/login',
  register: '/auth/register',

  // 笔记本
  notebooks: '/notebooks',
  notebook: (id: string) => `/notebooks/${id}`,

  // 文档
  documents: (notebookId: string) => `/notebooks/${notebookId}/documents`,
  document: (notebookId: string, documentId: string) =>
    `/notebooks/${notebookId}/documents/${documentId}`,
  documentGuide: (notebookId: string, documentId: string) =>
    `/notebooks/${notebookId}/documents/${documentId}/guide`,

  // 会话
  sessions: (notebookId: string) => `/notebooks/${notebookId}/sessions`,
  session: (notebookId: string, sessionId: string) =>
    `/notebooks/${notebookId}/sessions/${sessionId}`,
  sessionMessages: (sessionId: string) => `/chat/sessions/${sessionId}/messages`,
  chat: (notebookId: string, sessionId: string) =>
    `/notebooks/${notebookId}/sessions/${sessionId}/chat`,

  // 搜索
  search: (notebookId: string) => `/notebooks/${notebookId}/search`,

  // 笔记
  notes: '/notes',
  note: (id: string) => `/notes/${id}`,
  notePin: (id: string) => `/notes/${id}/pin`,
  noteTags: (id: string) => `/notes/${id}/tags`,
  noteTagsSearch: '/notes/tags/search',
} as const;

// ============================================================
// API 请求选项
// ============================================================

export interface FetchOptions extends RequestInit {
  token?: string;
}

// ============================================================
// 基础 fetch 封装
// ============================================================

export async function apiFetch<T = unknown>(
  endpoint: string,
  options: FetchOptions = {}
): Promise<T> {
  const { token, ...fetchOptions } = options;

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...fetchOptions.headers,
  };

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...fetchOptions,
    headers,
  });

  // 401 处理
  if (response.status === 401) {
    if (typeof window !== 'undefined') {
      window.location.href = '/login';
    }
    throw new Error('Unauthorized');
  }

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || `HTTP ${response.status}`);
  }

  // 204 No Content
  if (response.status === 204) {
    return {} as T;
  }

  return response.json();
}

// ============================================================
// SSE 流式请求
// ============================================================

export interface SSEChunk {
  session_id: string;
  message_id: string;
  content: string;
  sources: Array<{
    document_id: string;
    document_name: string;
    page_number: number;
    chunk_index: number;
    content: string;
    score: number;
  }>;
}

export type SSEEventHandler = (chunk: SSEChunk) => void;
export type SSEDoneHandler = (finalContent: string) => void;
export type SSEErrorHandler = (error: Error) => void;

export interface SSEOptions {
  token: string;
  onChunk?: SSEEventHandler;
  onDone?: SSEDoneHandler;
  onError?: SSEErrorHandler;
}

export async function streamChat(
  endpoint: string,
  body: { question: string; document_ids?: string[] },
  options: SSEOptions
): Promise<string> {
  const { token, onChunk, onDone, onError } = options;

  let accumulatedContent = '';

  try {
    const response = await fetch(`${API_BASE_URL}${endpoint}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    });

    if (!response.ok) {
      if (response.status === 401) {
        if (typeof window !== 'undefined') {
          window.location.href = '/login';
        }
        throw new Error('Unauthorized');
      }
      // 410 SESSION_GONE：会话已被删除或不存在
      if (response.status === 410) {
        const errorData = await response.json().catch(() => ({}));
        const sessionGoneError = new Error(errorData.error || '会话已失效');
        (sessionGoneError as Error & { code: string; sessionGone: true }).code = errorData.code || 'SESSION_GONE';
        (sessionGoneError as Error & { sessionGone: boolean }).sessionGone = true;
        throw sessionGoneError;
      }
      throw new Error(`HTTP ${response.status}`);
    }

    const reader = response.body?.getReader();
    if (!reader) {
      throw new Error('No response body');
    }

    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();

      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6).trim();

          if (data === '[DONE]') {
            onDone?.(accumulatedContent);
            return accumulatedContent;
          }

          try {
            const parsed: SSEChunk = JSON.parse(data);
            accumulatedContent = parsed.content;
            onChunk?.(parsed);
          } catch {
            // 忽略解析错误
          }
        }
      }
    }

    return accumulatedContent;
  } catch (error) {
    const err = error instanceof Error ? error : new Error('Stream error');
    onError?.(err);
    throw err;
  }
}

// ============================================================
// 工具函数
// ============================================================

// 获取认证 Token
export const getAuthToken = (): string | null => {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem('auth_token');
};

export const formatDate = (dateString: string): string => {
  const date = new Date(dateString);
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
};

export const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
};

export const truncateText = (text: string, maxLength: number): string => {
  if (text.length <= maxLength) return text;
  return text.slice(0, maxLength) + '...';
};

// 来源徽章正则匹配
export const SOURCE_PATTERN = /\[Source:\s*([^,\]]+)(?:,\s*Page\s*(\d+))?\]/g;

export interface ParsedSource {
  fullMatch: string;
  documentName: string;
  pageNumber?: number;
}

// 解析来源标记
export function parseSourceCitations(text: string): {
  parts: Array<{ type: 'text' | 'source'; content: string; source?: ParsedSource }>;
  sources: ParsedSource[];
} {
  const sources: ParsedSource[] = [];
  const parts: Array<{ type: 'text' | 'source'; content: string; source?: ParsedSource }> = [];

  let lastIndex = 0;
  let match;

  const regex = new RegExp(SOURCE_PATTERN);
  while ((match = regex.exec(text)) !== null) {
    // 添加匹配前的文本
    if (match.index > lastIndex) {
      parts.push({
        type: 'text',
        content: text.slice(lastIndex, match.index),
      });
    }

    const source: ParsedSource = {
      fullMatch: match[0],
      documentName: match[1].trim(),
      pageNumber: match[2] ? parseInt(match[2], 10) : undefined,
    };
    sources.push(source);

    // 添加来源部分
    parts.push({
      type: 'source',
      content: match[0],
      source,
    });

    lastIndex = match.index + match[0].length;
  }

  // 添加剩余文本
  if (lastIndex < text.length) {
    parts.push({
      type: 'text',
      content: text.slice(lastIndex),
    });
  }

  return { parts, sources };
}

// 延时函数
export const delay = (ms: number): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, ms));

// 防抖函数
export function debounce<T extends (...args: unknown[]) => unknown>(
  fn: T,
  ms: number
): (...args: Parameters<T>) => void {
  let timeoutId: ReturnType<typeof setTimeout>;

  return (...args: Parameters<T>) => {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => fn(...args), ms);
  };
}

// 生成唯一 ID
export const generateId = (): string =>
  `${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
