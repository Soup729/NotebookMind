"use client";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1";

export type ApiError = Error & { status?: number };

export function normalizeToken(token: string | null | undefined) {
  if (!token) {
    return "";
  }
  const trimmed = token.trim();
  if (trimmed.toLowerCase().startsWith("bearer ")) {
    return `Bearer ${trimmed.slice(7).trim()}`;
  }
  return `Bearer ${trimmed}`;
}

export async function apiRequest<T>(path: string, init: RequestInit = {}, token?: string): Promise<T> {
  const headers = new Headers(init.headers || {});
  headers.set("Accept", "application/json");

  const isFormData = init.body instanceof FormData;
  if (!isFormData && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const normalizedToken = normalizeToken(token);
  if (normalizedToken) {
    headers.set("Authorization", normalizedToken);
  }

  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    headers,
    cache: "no-store"
  });

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  const data = text ? JSON.parse(text) : {};
  if (!response.ok) {
    const error = new Error(data?.error || "Request failed") as ApiError;
    error.status = response.status;
    throw error;
  }

  return data as T;
}

// SSE Event Types
export interface SourceEvent {
  session_id: string;
  message_id: string;
  sources: ChatSource[];
  prompt_tokens: number;
}

export interface TokenEvent {
  session_id: string;
  message_id: string;
  token: string;
}

export interface DoneEvent {
  session_id: string;
  message_id: string;
  content: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface ErrorEvent {
  session_id: string;
  message_id: string;
  error: string;
}

export interface ChatSource {
  document_id: string;
  file_name: string;
  content: string;
  score: number;
  chunk_index: number;
}

export type SSEEventType = "source" | "token" | "done" | "error";

export interface SSEEventCallback {
  onSource?: (event: SourceEvent) => void;
  onToken?: (event: TokenEvent) => void;
  onDone?: (event: DoneEvent) => void;
  onError?: (event: ErrorEvent) => void;
}

export class SSEClient {
  private controller: AbortController | null = null;
  private token: string;

  constructor(token: string) {
    this.token = token;
  }

  async streamChat(
    sessionId: string,
    question: string,
    callbacks: SSEEventCallback,
    documentIds?: string[]
  ): Promise<void> {
    this.controller = new AbortController();

    const headers = new Headers();
    headers.set("Content-Type", "application/json");
    headers.set("Authorization", normalizeToken(this.token));
    headers.set("Accept", "text/event-stream");

    const body: Record<string, unknown> = { question };
    if (documentIds && documentIds.length > 0) {
      body.document_ids = documentIds;
    }

    try {
      const response = await fetch(`${API_URL}/chat/sessions/${sessionId}/stream`, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        signal: this.controller.signal,
      });

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || "Stream request failed");
      }

      const reader = response.body?.getReader();
      if (!reader) {
        throw new Error("Response body is not readable");
      }

      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          if (line.startsWith("event: ")) {
            const eventType = line.slice(7).trim() as SSEEventType;
            continue;
          }
          if (line.startsWith("data: ")) {
            const data = line.slice(6).trim();
            if (data === "[DONE]") {
              return;
            }
            try {
              const parsed = JSON.parse(data);
              // Infer event type from payload structure
              if ("sources" in parsed && "prompt_tokens" in parsed) {
                callbacks.onSource?.(parsed as SourceEvent);
              } else if ("token" in parsed && "session_id" in parsed) {
                callbacks.onToken?.(parsed as TokenEvent);
              } else if ("content" in parsed && "total_tokens" in parsed) {
                callbacks.onDone?.(parsed as DoneEvent);
              } else if ("error" in parsed) {
                callbacks.onError?.(parsed as ErrorEvent);
              }
            } catch {
              // Skip malformed JSON
            }
          }
        }
      }
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") {
        return;
      }
      throw err;
    }
  }

  abort(): void {
    this.controller?.abort();
  }
}

// VQA Client for Visual Question Answering

export interface VQAResponse {
  answer: string;
  image_answer?: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  context_enhanced?: boolean;
}

export class VQAClient {
  private token: string;

  constructor(token: string) {
    this.token = token;
  }

  async askImage(question: string, imageFile: File): Promise<VQAResponse> {
    const formData = new FormData();
    formData.append("question", question);
    formData.append("image", imageFile);

    const response = await fetch(`${API_URL}/vqa/image`, {
      method: "POST",
      headers: {
        "Authorization": normalizeToken(this.token),
      },
      body: formData,
    });

    if (!response.ok) {
      const errorData = await response.json();
      throw new Error(errorData.error || "VQA request failed");
    }

    return response.json();
  }

  async askImageURL(question: string, imageURL: string): Promise<VQAResponse> {
    const response = await fetch(`${API_URL}/vqa/image-url`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": normalizeToken(this.token),
      },
      body: JSON.stringify({ question, image_url: imageURL }),
    });

    if (!response.ok) {
      const errorData = await response.json();
      throw new Error(errorData.error || "VQA request failed");
    }

    return response.json();
  }

  async askImageWithContext(
    question: string,
    imageFile: File,
    documentIds?: string[]
  ): Promise<VQAResponse> {
    const formData = new FormData();
    formData.append("question", question);
    formData.append("image", imageFile);
    if (documentIds && documentIds.length > 0) {
      formData.append("document_ids", JSON.stringify(documentIds));
    }

    const response = await fetch(`${API_URL}/vqa/image-context`, {
      method: "POST",
      headers: {
        "Authorization": normalizeToken(this.token),
      },
      body: formData,
    });

    if (!response.ok) {
      const errorData = await response.json();
      throw new Error(errorData.error || "VQA request failed");
    }

    return response.json();
  }
}

// Reflection types
export interface ReflectionResult {
  accuracy_score: number;
  completeness_score: number;
  source_coverage: string[];
  missing_aspects: string[];
  suggested_improvements: string[];
  confidence_level: "high" | "medium" | "low";
}

// API functions for advanced features
export async function getRecommendations(
  sessionId: string,
  token: string
): Promise<{ session_id: string; questions: string[] }> {
  const response = await fetch(
    `${API_URL}/chat/sessions/${sessionId}/recommendations`,
    {
      method: "POST",
      headers: {
        "Authorization": normalizeToken(token),
      },
    }
  );

  if (!response.ok) {
    const errorData = await response.json();
    throw new Error(errorData.error || "Failed to get recommendations");
  }

  return response.json();
}

export async function getReflection(
  sessionId: string,
  messageId: string,
  token: string
): Promise<{ session_id: string; message_id: string; reflection: ReflectionResult }> {
  const response = await fetch(
    `${API_URL}/chat/sessions/${sessionId}/messages/${messageId}/reflection`,
    {
      method: "POST",
      headers: {
        "Authorization": normalizeToken(token),
      },
    }
  );

  if (!response.ok) {
    const errorData = await response.json();
    throw new Error(errorData.error || "Failed to get reflection");
  }

  return response.json();
}
