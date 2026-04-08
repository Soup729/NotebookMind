// ============================================================
// Enterprise PDF AI - API 类型定义
// ============================================================

// ============================================================
// 基础类型
// ============================================================

export interface ApiResponse<T> {
  data?: T;
  error?: string;
  message?: string;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

// ============================================================
// 认证
// ============================================================

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  user: User;
}

export interface RegisterRequest {
  email: string;
  password: string;
  name: string;
}

export interface User {
  id: string;
  email: string;
  name: string;
}

// ============================================================
// 笔记本 (Notebooks)
// ============================================================

export interface Notebook {
  id: string;
  title: string;
  description: string;
  status: 'active' | 'archived';
  document_cnt: number;
  created_at: string;
  updated_at: string;
}

export interface CreateNotebookRequest {
  title: string;
  description?: string;
}

export interface UpdateNotebookRequest {
  title?: string;
  description?: string;
  status?: 'active' | 'archived';
}

export interface NotebookListResponse {
  items: Notebook[];
}

// ============================================================
// 文档 (Documents)
// ============================================================

export interface Document {
  id: string;
  notebook_id: string;
  file_name: string;
  stored_path: string;
  status: 'processing' | 'completed' | 'failed';
  file_size: number;
  chunk_count: number;
  summary?: string;
  guide_status?: 'pending' | 'completed' | 'failed';
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface DocumentListResponse {
  items: Document[];
}

export interface UploadDocumentResponse {
  id: string;
  file_name: string;
  status: string;
  error_message?: string;
  file_size: number;
  chunk_count: number;
  created_at: string;
  updated_at: string;
  task_id: string;
}

// ============================================================
// 文档指南 (Document Guide)
// ============================================================

export interface FaqItem {
  question: string;
  answer: string;
}

export interface KeyPoint {
  text: string;
}

export interface DocumentGuide {
  id: string;
  document_id: string;
  summary: string;
  faq_json: string;
  key_points: string;
  status: 'pending' | 'completed' | 'failed';
  error_msg?: string;
  generated_at: string;
  created_at: string;
}

export interface DocumentGuideResponse {
  guide: DocumentGuide;
}

// 解析后的指南数据
export interface ParsedGuide {
  summary: string;
  faq: FaqItem[];
  keyPoints: string[];
}

// ============================================================
// 会话 (Sessions)
// ============================================================

export interface Session {
  id: string;
  user_id: string;
  notebook_id: string;
  title: string;
  last_message_at: string;
  created_at: string;
}

export interface CreateSessionRequest {
  title: string;
}

export interface SessionListResponse {
  items: Session[];
}

// ============================================================
// 聊天消息 (Chat Messages)
// ============================================================

export interface ChatSource {
  document_id: string;
  document_name: string;
  page_number: number;
  chunk_index: number;
  content: string;
  score: number;
}

export interface ChatStreamChunk {
  session_id: string;
  message_id: string;
  content: string;
  sources: ChatSource[];
}

export interface ChatMessage {
  id: string;
  session_id: string;
  role: 'user' | 'assistant';
  content: string;
  sources: ChatSource[];
  created_at: string;
}

export interface ChatRequest {
  question: string;
  document_ids?: string[];
}

export interface ChatResponse {
  session_id: string;
  message_id: string;
}

// ============================================================
// 笔记 (Notes)
// ============================================================

export interface Note {
  id: string;
  notebook_id?: string;
  session_id?: string;
  title: string;
  content: string;
  type: 'ai_response' | 'original_text' | 'summary' | 'custom';
  is_pinned: boolean;
  tags: string[];
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CreateNoteRequest {
  notebook_id?: string;
  session_id?: string;
  title: string;
  content: string;
  type: 'ai_response' | 'original_text' | 'summary' | 'custom';
  is_pinned?: boolean;
  tags?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateNoteRequest {
  title?: string;
  content?: string;
  is_pinned?: boolean;
  tags?: string[];
}

export interface NoteListResponse {
  items: Note[];
  total_count: number;
  page: number;
  page_size: number;
}

export interface NoteListParams {
  notebook_id?: string;
  session_id?: string;
  type?: Note['type'];
  tag?: string;
  pinned_only?: boolean;
  page?: number;
  page_size?: number;
}

// ============================================================
// 搜索 (Search)
// ============================================================

export interface SearchResult {
  chunk_id: string;
  document_id: string;
  document_name: string;
  page_number: number;
  chunk_index: number;
  content: string;
  score: number;
  rank: number;
  metadata: {
    page_number: number;
    chunk_index: number;
  };
}

export interface SearchRequest {
  query: string;
  top_k?: number;
  use_hybrid?: boolean;
  use_rerank?: boolean;
  document_ids?: string[];
}

export interface SearchResponse {
  query: string;
  top_k: number;
  items: SearchResult[];
}

// ============================================================
// 高亮目标 (Highlight Target)
// ============================================================

export interface HighlightTarget {
  pageNumber: number;
  boundingBox: [number, number, number, number]; // [x0, y0, x1, y1]
  sourceId: string;
  documentId: string;
  documentName: string;
  content: string;
}

// ============================================================
// 笔记本状态 (Notebook State)
// ============================================================

export type MainView = 'guide' | 'pdf';

export interface NotebookState {
  // 当前笔记本与会话
  currentNotebookId: string | null;
  activeSessionId: string | null;

  // 文档选择
  selectedDocumentIds: string[];

  // 视图状态
  mainView: MainView;
  activePdfId: string | null;

  // 高亮目标
  highlightTarget: HighlightTarget | null;

  // 操作方法
  setNotebookAndSession: (nbId: string, sId?: string) => void;
  setActiveSession: (sId: string | null) => void;
  toggleDocumentSelection: (docId: string) => void;
  setSelectedDocuments: (docIds: string[]) => void;
  setMainViewToPdf: (docId: string, target?: HighlightTarget) => void;
  setMainViewToGuide: () => void;
  clearHighlightTarget: () => void;
}

// ============================================================
// SSE 事件类型
// ============================================================

export type SSEEventType = 'chunk' | 'done' | 'error';

export interface SSEEvent {
  type: SSEEventType;
  data?: ChatStreamChunk;
  error?: string;
}

// ============================================================
// 组件 Props 类型
// ============================================================

export interface ChatPanelProps {
  notebookId: string;
  sessionId: string | null;
  onSessionCreate?: (session: Session) => void;
}

export interface SourcesPanelProps {
  notebookId: string;
  documents: Document[];
  isLoading: boolean;
  selectedIds: string[];
  onSelectionChange: (ids: string[]) => void;
  onUpload?: (file: File) => void;
  onRemove?: (docId: string) => void;
}

export interface NotesPanelProps {
  isOpen: boolean;
  onToggle: () => void;
  notebookId?: string;
}

export interface PdfViewerProps {
  documentId: string;
  fileUrl?: string;
  highlightTarget?: HighlightTarget | null;
  onClose?: () => void;
}

export interface DocumentGuideProps {
  documentId: string;
  guide?: ParsedGuide;
  isLoading: boolean;
  onSuggestedQueryClick?: (query: string) => void;
}
