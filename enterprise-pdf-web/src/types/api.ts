export interface User {
  id: string;
  name: string;
  email: string;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export interface DocumentItem {
  id: string;
  file_name: string;
  status: "processing" | "completed" | "failed";
  error_message: string;
  file_size: number;
  chunk_count: number;
  task_id?: string;
  created_at: string;
  updated_at: string;
  processed_at?: string | null;
}

export interface ChatSession {
  id: string;
  user_id: string;
  title: string;
  last_message_at: string;
  created_at: string;
  updated_at: string;
}

export interface ChatSource {
  document_id: string;
  file_name: string;
  content: string;
  score: number;
  chunk_index: number;
}

export interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  created_at: string;
  sources: ChatSource[];
}

export interface DashboardOverview {
  total_documents: number;
  completed_documents: number;
  total_sessions: number;
  total_messages: number;
  total_tokens: number;
  daily_tokens: Array<{ date: string; tokens: number }>;
}

export interface UsageSummary {
  total_tokens: number;
  daily_tokens: Array<{ date: string; tokens: number }>;
}
