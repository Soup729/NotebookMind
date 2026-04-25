'use client';

import { useCallback } from 'react';
import useSWR from 'swr';
import { toast } from 'sonner';
import { API_ENDPOINTS, apiFetch, getAuthToken } from '@/lib/utils';
import type { SessionMemory } from '@/types/api';

export function useSessionMemory(notebookId: string, sessionId: string | null) {
  const token = getAuthToken();
  const key = notebookId && sessionId && token
    ? [API_ENDPOINTS.sessionMemory(notebookId, sessionId), token]
    : null;

  const { data, error, isLoading, mutate } = useSWR(
    key,
    ([url, authToken]) => apiFetch<{ memory: SessionMemory }>(url, { token: authToken }),
    { revalidateOnFocus: false }
  );

  const refreshMemory = useCallback(async () => {
    if (!sessionId || !token) {
      toast.error('请先选择一个会话');
      return null;
    }
    try {
      const result = await apiFetch<{ memory: SessionMemory }>(
        API_ENDPOINTS.sessionMemoryRefresh(notebookId, sessionId),
        { method: 'POST', token }
      );
      await mutate(result, false);
      toast.success('会话记忆已刷新');
      return result.memory;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('刷新失败');
      toast.error('刷新会话记忆失败', { description: error.message });
      return null;
    }
  }, [mutate, notebookId, sessionId, token]);

  const clearMemory = useCallback(async () => {
    if (!sessionId || !token) {
      toast.error('请先选择一个会话');
      return false;
    }
    try {
      await apiFetch(API_ENDPOINTS.sessionMemory(notebookId, sessionId), {
        method: 'DELETE',
        token,
      });
      await mutate({ memory: { summary: '' } }, false);
      toast.success('会话记忆已清空');
      return true;
    } catch (err) {
      const error = err instanceof Error ? err : new Error('清空失败');
      toast.error('清空会话记忆失败', { description: error.message });
      return false;
    }
  }, [mutate, notebookId, sessionId, token]);

  return {
    memory: data?.memory,
    isLoading,
    error,
    refreshMemory,
    clearMemory,
    mutate,
  };
}
