'use client';

import useSWR from 'swr';
import { API_ENDPOINTS, apiFetch, getAuthToken } from '@/lib/utils';
import type { KnowledgeGraphResponse } from '@/types/api';

export function useNotebookGraph(notebookId: string) {
  const token = getAuthToken();
  const key = notebookId && token ? [API_ENDPOINTS.knowledgeGraph(notebookId), token] : null;

  const { data, error, isLoading, mutate } = useSWR(
    key,
    ([url, authToken]) => apiFetch<KnowledgeGraphResponse>(url, { token: authToken }),
    {
      revalidateOnFocus: false,
      refreshInterval: (graph) => (graph?.status === 'building' ? 4000 : 0),
    }
  );

  return {
    graph: data,
    isLoading,
    error,
    mutate,
  };
}
