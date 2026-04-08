// ============================================================
// Enterprise PDF AI - PDF 类型定义
// ============================================================

export interface PDFPageMetrics {
  width: number;
  height: number;
  scale: number;
}

export interface HighlightBoundingBox {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

export interface HighlightRegion {
  id: string;
  pageNumber: number;
  boundingBox: HighlightBoundingBox;
  documentId: string;
  content: string;
  color?: string;
  opacity?: number;
}

export interface PDFAnnotation {
  id: string;
  type: 'highlight' | 'note' | 'bookmark';
  pageNumber: number;
  boundingBox: HighlightBoundingBox;
  content?: string;
  color?: string;
  createdAt: Date;
}

export interface PDFSearchResult {
  pageNumber: number;
  matches: PDFSearchMatch[];
}

export interface PDFSearchMatch {
  text: string;
  boundingBox: HighlightBoundingBox;
  index: number;
}

export interface PDFViewport {
  scale: number;
  offsetX: number;
  offsetY: number;
}

export interface PDFLoadingState {
  isLoading: boolean;
  progress: number;
  error: string | null;
}

export interface PDFPageRenderProps {
  pageNumber: number;
  scale: number;
  viewportWidth: number;
  onLoadSuccess?: (metrics: PDFPageMetrics) => void;
  onLoadError?: (error: Error) => void;
}

export interface HighlightLayerProps {
  pageNumber: number;
  pageWidth: number;
  pageHeight: number;
  scale: number;
  highlights: HighlightRegion[];
  onHighlightClick?: (highlight: HighlightRegion) => void;
}

export interface PdfViewerState {
  currentPage: number;
  totalPages: number;
  scale: number;
  isLoading: boolean;
  error: string | null;
}
