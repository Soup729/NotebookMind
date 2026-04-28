// ============================================================
// NotebookMind - 笔记本 & 文档 SWR Hooks
// ============================================================

'use client';

import useSWR, { mutate } from 'swr';
import { useCallback } from 'react';
import { toast } from 'sonner';
import { apiFetch, API_ENDPOINTS } from '@/lib/utils';
import type {
  Notebook,
  Document,
  DocumentGuide,
  Session,
  CreateNotebookRequest,
  CreateSessionRequest,
  Note,
  NoteListParams,
} from '@/types/api';

type RawSession = Partial<Session> & {
  ID?: string;
  UserID?: string;
  NotebookID?: string;
  Title?: string;
  LastMessageAt?: string;
  CreatedAt?: string;
  UpdatedAt?: string;
};

// ============================================================
// Token 获取（临时方案，后续可接入 Zustand Auth）
// ============================================================

const getAuthToken = (): string | null => {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem('auth_token');
};

function normalizeSession(raw: RawSession): Session {
  return {
    id: raw.id || raw.ID || '',
    user_id: raw.user_id || raw.UserID || '',
    notebook_id: raw.notebook_id || raw.NotebookID || '',
    title: raw.title || raw.Title || '新对话',
    last_message_at: raw.last_message_at || raw.LastMessageAt || raw.created_at || raw.CreatedAt || new Date().toISOString(),
    created_at: raw.created_at || raw.CreatedAt || new Date().toISOString(),
  };
}

// ============================================================
// 笔记本 Hooks
// ============================================================

/**
 * 获取笔记本详情
 */
export function useNotebook(notebookId: string | null) {
  const { data, error, isLoading, mutate: boundMutate } = useSWR(
    notebookId ? [API_ENDPOINTS.notebook(notebookId), getAuthToken()] : null,
    ([url, token]) => apiFetch<{ notebook: Notebook }>(url, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('获取笔记本失败', { description: err.message });
      },
    }
  );

  return {
    notebook: data?.notebook,
    isLoading,
    error,
    mutate: boundMutate,
  };
}

/**
 * 获取笔记本列表
 */
export function useNotebooks() {
  const { data, error, isLoading, mutate: boundMutate } = useSWR(
    [API_ENDPOINTS.notebooks, getAuthToken()],
    ([url, token]) => apiFetch<{ items: Notebook[] }>(url, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('获取笔记本列表失败', { description: err.message });
      },
    }
  );

  return {
    notebooks: data?.items || [],
    isLoading,
    error,
    mutate: boundMutate,
  };
}

/**
 * 创建笔记本
 */
export function useCreateNotebook() {
  const createNotebook = useCallback(
    async (request: CreateNotebookRequest): Promise<Notebook | null> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return null;
      }

      try {
        const response = await apiFetch<{ notebook: Notebook }>(
          API_ENDPOINTS.notebooks,
          {
            method: 'POST',
            body: JSON.stringify(request),
            token,
          }
        );

        toast.success('笔记本创建成功');
        mutate([API_ENDPOINTS.notebooks, token]);
        return response.notebook;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('创建失败');
        toast.error('创建笔记本失败', { description: error.message });
        return null;
      }
    },
    []
  );

  return { createNotebook };
}

/**
 * 更新笔记本
 */
export function useUpdateNotebook() {
  const updateNotebook = useCallback(
    async (
      notebookId: string,
      request: Partial<CreateNotebookRequest>
    ): Promise<boolean> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return false;
      }

      try {
        await apiFetch(API_ENDPOINTS.notebook(notebookId), {
          method: 'PUT',
          body: JSON.stringify(request),
          token,
        });

        toast.success('笔记本更新成功');
        mutate([API_ENDPOINTS.notebook(notebookId), token]);
        mutate([API_ENDPOINTS.notebooks, token]);
        return true;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('更新失败');
        toast.error('更新笔记本失败', { description: error.message });
        return false;
      }
    },
    []
  );

  return { updateNotebook };
}

/**
 * 删除笔记本
 */
export function useDeleteNotebook() {
  const deleteNotebook = useCallback(async (notebookId: string): Promise<boolean> => {
    const token = getAuthToken();
    if (!token) {
      toast.error('请先登录');
      return false;
    }

    try {
      await apiFetch(API_ENDPOINTS.notebook(notebookId), {
        method: 'DELETE',
        token,
      });

      toast.success('笔记本已删除');
      mutate([API_ENDPOINTS.notebooks, token]);
      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('删除失败');
      toast.error('删除笔记本失败', { description: error.message });
      return false;
    }
  }, []);

  return { deleteNotebook };
}

// ============================================================
// 文档 Hooks
// ============================================================

/**
 * 获取文档列表（自动轮询 processing 状态）
 */
export function useDocuments(notebookId: string | null) {
  const { data, error, isLoading, mutate: boundMutate } = useSWR(
    notebookId ? [API_ENDPOINTS.documents(notebookId), getAuthToken()] : null,
    ([url, token]) => apiFetch<{ items: Document[] }>(url, { token }),
    {
      revalidateOnFocus: false,
      // processing 状态的文档需要轮询
      refreshInterval: (data) => {
        const hasProcessing = data?.items?.some((d) => d.status === 'processing');
        return hasProcessing ? 3000 : 0;
      },
      onError: (err) => {
        toast.error('获取文档列表失败', { description: err.message });
      },
    }
  );

  const documents = data?.items || [];

  // 检查是否所有文档都已处理完成
  const isAllCompleted = documents.length > 0 && documents.every((d) => d.status !== 'processing');

  return {
    documents,
    isLoading,
    error,
    mutate: boundMutate,
    isAllCompleted,
  };
}

/**
 * 获取单个文档详情
 */
export function useDocument(notebookId: string | null, documentId: string | null) {
  const { data, error, isLoading } = useSWR(
    notebookId && documentId
      ? [API_ENDPOINTS.document(notebookId, documentId), getAuthToken()]
      : null,
    ([url, token]) => apiFetch<{ document: Document }>(url, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('获取文档详情失败', { description: err.message });
      },
    }
  );

  return {
    document: data?.document,
    isLoading,
    error,
  };
}

/**
 * 上传文档到笔记本
 */
export function useUploadDocument() {
  const uploadDocument = useCallback(
    async (notebookId: string, file: File): Promise<string | null> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return null;
      }

      const formData = new FormData();
      formData.append('file', file);
      formData.append('notebook_id', notebookId);

      try {
        // 上传到 /documents 端点，包含 notebook_id
        const response = await fetch(
          `${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1'}/documents`,
          {
            method: 'POST',
            headers: {
              Authorization: `Bearer ${token}`,
            },
            body: formData,
          }
        );

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.error || `Upload failed: ${response.status}`);
        }

        const result = await response.json();
        toast.success('文档上传成功');

        // 刷新文档列表
        mutate([API_ENDPOINTS.documents(notebookId), token]);

        return result.id;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('上传失败');
        toast.error('文档上传失败', { description: error.message });
        return null;
      }
    },
    []
  );

  return { uploadDocument };
}

/**
 * 删除文档
 */
export function useDeleteDocument() {
  const deleteDocument = useCallback(
    async (notebookId: string, documentId: string): Promise<boolean> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return false;
      }

      try {
        await apiFetch(API_ENDPOINTS.documentFile(documentId), {
          method: 'DELETE',
          token,
        });

        toast.success('文档已删除');
        mutate([API_ENDPOINTS.documents(notebookId), token]);
        return true;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('删除失败');
        toast.error('删除文档失败', { description: error.message });
        return false;
      }
    },
    []
  );

  return { deleteDocument };
}

// ============================================================
// 文档指南 Hooks
// ============================================================

/**
 * 获取文档指南
 */
export function useDocumentGuide(
  notebookId: string | null,
  documentId: string | null
) {
  const { data, error, isLoading } = useSWR(
    notebookId && documentId
      ? [API_ENDPOINTS.documentGuide(notebookId, documentId), getAuthToken()]
      : null,
    ([url, token]) => apiFetch<{ guide: DocumentGuide }>(url, { token }),
    {
      revalidateOnFocus: false,
      // 指南 pending 状态时轮询（每 5 秒重试）
      refreshInterval: (data) => {
        if (data?.guide?.status === 'pending') return 5000;
        return 0;
      },
      onErrorRetry: (error, _key, _config, revalidate, opts) => {
        // 指南生成中的 "record not found" 是预期行为，不应报错
        // 仅在非 404 错误或重试次数过多时才停止重试
        if (error.message.includes('not found') || String(error).includes('Not Found')) {
          // record not found = 指南正在生成中，继续轮询
          setTimeout(() => revalidate({ retryCount: opts.retryCount + 1 }), 3000);
          return;
        }
        // 其他错误：最多重试 3 次
        if (opts.retryCount >= 3) return;
        setTimeout(() => revalidate({ retryCount: opts.retryCount + 1 }), 2000);
      },
      onError: (err) => {
        // 静默处理：不弹 toast，仅在控制台输出
        console.debug('文档指南待生成:', err.message);
      },
    }
  );

  // 解析 FAQ JSON
  const parseGuide = useCallback((guide: DocumentGuide | undefined) => {
    if (!guide) return null;

    try {
      const faq = guide.faq_json ? JSON.parse(guide.faq_json) : [];
      const keyPoints = guide.key_points ? JSON.parse(guide.key_points) : [];
      return {
        summary: guide.summary,
        faq,
        keyPoints,
        status: guide.status,
      };
    } catch {
      return {
        summary: guide.summary,
        faq: [],
        keyPoints: [],
        status: guide.status,
      };
    }
  }, []);

  return {
    guide: data?.guide,
    parsedGuide: parseGuide(data?.guide),
    isLoading,
    error,
  };
}

// ============================================================
// 会话 Hooks
// ============================================================

/**
 * 获取会话列表
 */
export function useSessions(notebookId: string | null) {
  const { data, error, isLoading, mutate: boundMutate } = useSWR(
    notebookId ? [API_ENDPOINTS.sessions(notebookId), getAuthToken()] : null,
    ([url, token]) => apiFetch<{ items: Session[] }>(url, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('获取会话列表失败', { description: err.message });
      },
    }
  );

  return {
    sessions: (data?.items || []).map((item) => normalizeSession(item as RawSession)).filter((session) => session.id),
    isLoading,
    error,
    mutate: boundMutate,
  };
}

/**
 * 创建会话
 */
export function useCreateSession() {
  const createSession = useCallback(
    async (notebookId: string, request: CreateSessionRequest): Promise<Session | null> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return null;
      }

      try {
        const response = await apiFetch<{ session: Session }>(
          API_ENDPOINTS.sessions(notebookId),
          {
            method: 'POST',
            body: JSON.stringify(request),
            token,
          }
        );

        // 不在这里弹 toast — 由调用方统一处理提示
        mutate([API_ENDPOINTS.sessions(notebookId), token]);
        const session = normalizeSession(response.session as RawSession);
        if (!session.id) {
          throw new Error('后端创建了会话，但返回中缺少 session id');
        }
        return session;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('创建会话失败');
        toast.error('创建会话失败', { description: error.message });
        return null;
      }
    },
    []
  );

  return { createSession };
}

/**
 * 删除会话
 */
export function useDeleteSession() {
  const deleteSession = useCallback(
    async (notebookId: string, sessionId: string): Promise<boolean> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return false;
      }

      try {
        await apiFetch(API_ENDPOINTS.session(notebookId, sessionId), {
          method: 'DELETE',
          token,
        });

        toast.success('会话已删除');
        mutate([API_ENDPOINTS.sessions(notebookId), token]);
        return true;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('删除失败');
        toast.error('删除会话失败', { description: error.message });
        return false;
      }
    },
    []
  );

  return { deleteSession };
}

// ============================================================
// 笔记 Hooks
// ============================================================

/**
 * 获取笔记列表
 */
export function useNotes(params?: NoteListParams) {
  const token = getAuthToken();

  const buildUrl = useCallback(() => {
    const searchParams = new URLSearchParams();
    if (params?.notebook_id) searchParams.set('notebook_id', params.notebook_id);
    if (params?.session_id) searchParams.set('session_id', params.session_id);
    if (params?.type) searchParams.set('type', params.type);
    if (params?.tag) searchParams.set('tag', params.tag);
    if (params?.pinned_only) searchParams.set('pinned_only', 'true');
    if (params?.page) searchParams.set('page', String(params.page));
    if (params?.page_size) searchParams.set('page_size', String(params.page_size));

    const query = searchParams.toString();
    return query ? `${API_ENDPOINTS.notes}?${query}` : API_ENDPOINTS.notes;
  }, [params]);

  const { data, error, isLoading, mutate: boundMutate } = useSWR(
    token ? [buildUrl(), token] : null,
    ([url, token]) => apiFetch<{ items: Note[]; total_count: number }>(url, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('获取笔记列表失败', { description: err.message });
      },
    }
  );

  return {
    notes: data?.items || [],
    total: data?.total_count || 0,
    isLoading,
    error,
    mutate: boundMutate,
  };
}

/**
 * 创建笔记
 */
export function useCreateNote() {
  const createNote = useCallback(
    async (
      request: Omit<import('@/types/api').CreateNoteRequest, never>
    ): Promise<Note | null> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return null;
      }

      try {
        const response = await apiFetch<{ note: Note }>(API_ENDPOINTS.notes, {
          method: 'POST',
          body: JSON.stringify(request),
          token,
        });

        toast.success('笔记已保存');
        mutate((key) => typeof key === 'string' && key.startsWith(API_ENDPOINTS.notes));
        return response.note;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('保存失败');
        toast.error('保存笔记失败', { description: error.message });
        return null;
      }
    },
    []
  );

  return { createNote };
}

/**
 * 更新笔记
 */
export function useUpdateNote() {
  const updateNote = useCallback(
    async (noteId: string, request: Partial<import('@/types/api').UpdateNoteRequest>): Promise<boolean> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return false;
      }

      try {
        await apiFetch(API_ENDPOINTS.note(noteId), {
          method: 'PUT',
          body: JSON.stringify(request),
          token,
        });

        toast.success('笔记已更新');
        mutate((key) => typeof key === 'string' && key.startsWith(API_ENDPOINTS.notes));
        return true;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('更新失败');
        toast.error('更新笔记失败', { description: error.message });
        return false;
      }
    },
    []
  );

  return { updateNote };
}

/**
 * 删除笔记
 */
export function useDeleteNote() {
  const deleteNote = useCallback(async (noteId: string): Promise<boolean> => {
    const token = getAuthToken();
    if (!token) {
      toast.error('请先登录');
      return false;
    }

    try {
      await apiFetch(API_ENDPOINTS.note(noteId), {
        method: 'DELETE',
        token,
      });

      toast.success('笔记已删除');
      mutate((key) => typeof key === 'string' && key.startsWith(API_ENDPOINTS.notes));
      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('删除失败');
      toast.error('删除笔记失败', { description: error.message });
      return false;
    }
  }, []);

  return { deleteNote };
}

/**
 * 钉住/取消钉住笔记
 */
export function useTogglePinNote() {
  const togglePin = useCallback(async (noteId: string): Promise<boolean> => {
    const token = getAuthToken();
    if (!token) {
      toast.error('请先登录');
      return false;
    }

    try {
      await apiFetch<{ note: Note }>(API_ENDPOINTS.notePin(noteId), {
        method: 'POST',
        token,
      });

      mutate((key) => typeof key === 'string' && key.startsWith(API_ENDPOINTS.notes));
      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('操作失败');
      toast.error('钉住笔记失败', { description: error.message });
      return false;
    }
  }, []);

  return { togglePin };
}

// ============================================================
// 模型列表 Hook
// ============================================================

/**
 * 获取可用 LLM 模型列表
 */
export function useAvailableModels() {
  const { data, error, isLoading } = useSWR(
    [API_ENDPOINTS.models, getAuthToken()],
    ([url, token]) => apiFetch<import('@/types/api').ModelsResponse>(url, { token }),
    {
      revalidateOnFocus: false,
      revalidateIfStale: false,
      onError: (err) => {
        console.debug('获取模型列表失败:', err.message);
      },
    }
  );

  return {
    models: data?.models || [],
    defaultProvider: data?.default_provider || 'openai',
    isLoading,
    error,
  };
}


