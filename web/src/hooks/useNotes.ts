// ============================================================
// NotebookMind - 笔记 Hook
// ============================================================

'use client';

import useSWR, { mutate } from 'swr';
import { useCallback } from 'react';
import { toast } from 'sonner';
import { apiFetch, API_ENDPOINTS, getAuthToken } from '@/lib/utils';
import type { Note, CreateNoteRequest, UpdateNoteRequest, NoteListParams } from '@/types/api';

// ============================================================
// 获取笔记列表
// ============================================================

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
    ([url, token]) => apiFetch<{ items: Note[]; total_count?: number; total?: number }>(url, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('获取笔记列表失败', { description: err.message });
      },
    }
  );

  return {
    notes: data?.items || [],
    total: data?.total_count ?? data?.total ?? 0,
    isLoading,
    error,
    mutate: boundMutate,
  };
}

// ============================================================
// 获取单个笔记
// ============================================================

export function useNote(noteId: string | null) {
  const { data, error, isLoading } = useSWR(
    noteId ? [API_ENDPOINTS.note(noteId), getAuthToken()] : null,
    ([url, token]) => apiFetch<{ note: Note }>(url, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('获取笔记详情失败', { description: err.message });
      },
    }
  );

  return {
    note: data?.note,
    isLoading,
    error,
  };
}

// ============================================================
// 创建笔记
// ============================================================

export function useCreateNote() {
  const createNote = useCallback(
    async (request: CreateNoteRequest): Promise<Note | null> => {
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
        // 刷新笔记列表 - 使用确切的 URL 模式
        const baseUrl = API_ENDPOINTS.notes;
        mutate((key) => {
          if (!Array.isArray(key)) return false;
          const [url] = key;
          return typeof url === 'string' && url.startsWith(baseUrl);
        });
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

// ============================================================
// 更新笔记
// ============================================================

export function useUpdateNote() {
  const updateNote = useCallback(
    async (noteId: string, request: UpdateNoteRequest): Promise<boolean> => {
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
        mutate((key) => {
          if (!Array.isArray(key)) return false;
          const [url] = key;
          return typeof url === 'string' && url.startsWith(API_ENDPOINTS.notes);
        });
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

// ============================================================
// 删除笔记
// ============================================================

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
      mutate((key) => {
        if (!Array.isArray(key)) return false;
        const [url] = key;
        return typeof url === 'string' && url.startsWith(API_ENDPOINTS.notes);
      });
      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('删除失败');
      toast.error('删除笔记失败', { description: error.message });
      return false;
    }
  }, []);

  return { deleteNote };
}

// ============================================================
// 钉住/取消钉住笔记
// ============================================================

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

      mutate((key) => {
        if (!Array.isArray(key)) return false;
        const [url] = key;
        return typeof url === 'string' && url.startsWith(API_ENDPOINTS.notes);
      });
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
// 添加笔记标签
// ============================================================

export function useAddNoteTag() {
  const addTag = useCallback(async (noteId: string, tag: string): Promise<boolean> => {
    const token = getAuthToken();
    if (!token) {
      toast.error('请先登录');
      return false;
    }

    try {
      await apiFetch(API_ENDPOINTS.noteTags(noteId), {
        method: 'POST',
        body: JSON.stringify({ tag }),
        token,
      });

      toast.success('标签已添加');
      mutate((key) => {
        if (!Array.isArray(key)) return false;
        const [url] = key;
        return typeof url === 'string' && url.startsWith(API_ENDPOINTS.notes);
      });
      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('添加失败');
      toast.error('添加标签失败', { description: error.message });
      return false;
    }
  }, []);

  return { addTag };
}

// ============================================================
// 移除笔记标签
// ============================================================

export function useRemoveNoteTag() {
  const removeTag = useCallback(async (noteId: string, tag: string): Promise<boolean> => {
    const token = getAuthToken();
    if (!token) {
      toast.error('请先登录');
      return false;
    }

    try {
      await apiFetch(API_ENDPOINTS.noteTags(noteId), {
        method: 'DELETE',
        body: JSON.stringify({ tag }),
        token,
      });

      toast.success('标签已移除');
      mutate((key) => {
        if (!Array.isArray(key)) return false;
        const [url] = key;
        return typeof url === 'string' && url.startsWith(API_ENDPOINTS.notes);
      });
      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('移除失败');
      toast.error('移除标签失败', { description: error.message });
      return false;
    }
  }, []);

  return { removeTag };
}

// ============================================================
// 按标签搜索笔记
// ============================================================

export function useSearchNotesByTag(tag: string) {
  const { data, error, isLoading } = useSWR(
    tag ? [API_ENDPOINTS.noteTagsSearch, getAuthToken(), tag] : null,
    ([url, token, tag]) =>
      apiFetch<{ items: Note[] }>(`${url}?tag=${encodeURIComponent(tag)}`, { token }),
    {
      revalidateOnFocus: false,
      onError: (err) => {
        toast.error('搜索笔记失败', { description: err.message });
      },
    }
  );

  return {
    notes: data?.items || [],
    isLoading,
    error,
  };
}
