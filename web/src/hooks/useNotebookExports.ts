'use client';

import { useCallback } from 'react';
import useSWR from 'swr';
import { toast } from 'sonner';
import { API_BASE_URL, API_ENDPOINTS, apiFetch, downloadBlob } from '@/lib/utils';
import type {
  ConfirmExportRequest,
  ExportOutlineRequest,
  NotebookArtifact,
} from '@/types/api';

const getAuthToken = (): string | null => {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem('auth_token');
};

function fileNameFromDisposition(disposition: string | null): string | null {
  if (!disposition) return null;
  const utf8Match = disposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8Match?.[1]) return decodeURIComponent(utf8Match[1]);
  const plainMatch = disposition.match(/filename="?([^";]+)"?/i);
  return plainMatch?.[1] || null;
}

export function useNotebookExports(notebookId: string) {
  const createOutline = useCallback(
    async (request: ExportOutlineRequest): Promise<NotebookArtifact | null> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return null;
      }

      try {
        const data = await apiFetch<{ artifact: NotebookArtifact }>(
          API_ENDPOINTS.exportOutline(notebookId),
          {
            method: 'POST',
            body: JSON.stringify(request),
            token,
          }
        );
        return data.artifact;
      } catch (error) {
        const err = error instanceof Error ? error : new Error('生成大纲失败');
        toast.error('生成大纲失败', { description: err.message });
        return null;
      }
    },
    [notebookId]
  );

  const confirmExport = useCallback(
    async (artifactId: string, request: ConfirmExportRequest): Promise<NotebookArtifact | null> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return null;
      }

      try {
        const data = await apiFetch<{ artifact: NotebookArtifact }>(
          API_ENDPOINTS.exportConfirm(notebookId, artifactId),
          {
            method: 'POST',
            body: JSON.stringify(request),
            token,
          }
        );
        toast.success('导出任务已开始');
        return data.artifact;
      } catch (error) {
        const err = error instanceof Error ? error : new Error('提交导出失败');
        toast.error('提交导出失败', { description: err.message });
        return null;
      }
    },
    [notebookId]
  );

  const downloadExport = useCallback(
    async (artifact: NotebookArtifact): Promise<boolean> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return false;
      }

      try {
        const response = await fetch(`${API_BASE_URL}${API_ENDPOINTS.exportDownload(notebookId, artifact.id)}`, {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        const blob = await response.blob();
        const fileName =
          fileNameFromDisposition(response.headers.get('Content-Disposition')) ||
          artifact.file_name ||
          'notebook-export';
        downloadBlob(blob, fileName);
        return true;
      } catch (error) {
        const err = error instanceof Error ? error : new Error('下载失败');
        toast.error('下载失败', { description: err.message });
        return false;
      }
    },
    [notebookId]
  );

  return { createOutline, confirmExport, downloadExport };
}

export function useExportArtifact(notebookId: string, artifactId: string | null) {
  const token = getAuthToken();
  const { data, error, isLoading, mutate } = useSWR(
    notebookId && artifactId && token
      ? [API_ENDPOINTS.artifact(notebookId, artifactId), token]
      : null,
    ([url, authToken]) => apiFetch<{ artifact: NotebookArtifact }>(url, { token: authToken }),
    {
      refreshInterval: (data) => (data?.artifact?.status === 'generating' ? 2000 : 0),
      revalidateOnFocus: false,
    }
  );

  return {
    artifact: data?.artifact,
    isLoading,
    error,
    mutate,
  };
}

export function useNotebookArtifacts(notebookId: string) {
  const token = getAuthToken();
  const { data, error, isLoading, mutate } = useSWR(
    notebookId && token ? [API_ENDPOINTS.artifacts(notebookId), token] : null,
    ([url, authToken]) => apiFetch<{ items: NotebookArtifact[] }>(url, { token: authToken }),
    { revalidateOnFocus: false }
  );

  return {
    artifacts: data?.items || [],
    isLoading,
    error,
    mutate,
  };
}

export function useGenerateNotebookArtifact(notebookId: string) {
  const generateArtifact = useCallback(
    async (type: string): Promise<NotebookArtifact | null> => {
      const token = getAuthToken();
      if (!token) {
        toast.error('请先登录');
        return null;
      }

      try {
        const data = await apiFetch<{ artifact: NotebookArtifact }>(
          API_ENDPOINTS.artifactGenerate(notebookId),
          {
            method: 'POST',
            body: JSON.stringify({ type }),
            token,
          }
        );
        toast.success('工作台产物已生成');
        return data.artifact;
      } catch (error) {
        const err = error instanceof Error ? error : new Error('生成失败');
        toast.error('生成工作台产物失败', { description: err.message });
        return null;
      }
    },
    [notebookId]
  );

  return { generateArtifact };
}
