// ============================================================
// Enterprise PDF AI - 笔记本状态管理 (Zustand)
// ============================================================

import { create } from 'zustand';
import { devtools, persist } from 'zustand/middleware';
import type { HighlightTarget, MainView, NotebookState } from '@/types/api';

// ============================================================
// Store 实现
// ============================================================

interface NotebookStore extends NotebookState {
  // 内部状态
  _hasHydrated: boolean;

  // Hydration 标记
  setHasHydrated: (state: boolean) => void;

  // 重置状态
  reset: () => void;
}

const initialState: NotebookState = {
  // 初始状态
  currentNotebookId: null,
  activeSessionId: null,
  selectedDocumentIds: [],
  mainView: 'guide',
  activePdfId: null,
  highlightTarget: null,

  // 操作方法 - 空实现，将在 store 内部绑定
  setNotebookAndSession: () => {},
  setActiveSession: () => {},
  toggleDocumentSelection: () => {},
  setSelectedDocuments: () => {},
  setMainViewToPdf: () => {},
  setMainViewToGuide: () => {},
  clearHighlightTarget: () => {},
};

export const useNotebookStore = create<NotebookStore>()(
  devtools(
    persist(
      (set, get) => ({
        ...initialState,
        _hasHydrated: false,

        setHasHydrated: (state: boolean) => {
          set({ _hasHydrated: state });
        },

        // ============================================================
        // 设置笔记本与会话
        // ============================================================
        setNotebookAndSession: (nbId: string, sId?: string) => {
          set({
            currentNotebookId: nbId,
            activeSessionId: sId || null,
            selectedDocumentIds: [],
            mainView: 'guide',
            activePdfId: null,
            highlightTarget: null,
          });
        },

        // ============================================================
        // 设置当前会话
        // ============================================================
        setActiveSession: (sId: string | null) => {
          set({ activeSessionId: sId });
        },

        // ============================================================
        // 切换文档选中状态
        // ============================================================
        toggleDocumentSelection: (docId: string) => {
          const { selectedDocumentIds } = get();
          const isSelected = selectedDocumentIds.includes(docId);

          set({
            selectedDocumentIds: isSelected
              ? selectedDocumentIds.filter((id) => id !== docId)
              : [...selectedDocumentIds, docId],
          });
        },

        // ============================================================
        // 设置选中的文档列表
        // ============================================================
        setSelectedDocuments: (docIds: string[]) => {
          set({ selectedDocumentIds: docIds });
        },

        // ============================================================
        // 切换到 PDF 视图
        // ============================================================
        setMainViewToPdf: (docId: string, target?: HighlightTarget) => {
          set({
            mainView: 'pdf',
            activePdfId: docId,
            highlightTarget: target || null,
          });
        },

        // ============================================================
        // 切换到指南视图
        // ============================================================
        setMainViewToGuide: () => {
          set({
            mainView: 'guide',
            highlightTarget: null,
          });
        },

        // ============================================================
        // 清除高亮目标
        // ============================================================
        clearHighlightTarget: () => {
          set({ highlightTarget: null });
        },

        // ============================================================
        // 重置状态
        // ============================================================
        reset: () => {
          set({
            ...initialState,
            _hasHydrated: get()._hasHydrated,
          });
        },
      }),
      {
        name: 'enterprise-pdf-notebook-store',
        partialize: (state) => ({
          currentNotebookId: state.currentNotebookId,
          activeSessionId: state.activeSessionId,
        }),
        onRehydrateStorage: () => (state) => {
          state?.setHasHydrated(true);
        },
      }
    ),
    {
      name: 'NotebookStore',
    }
  )
);

// ============================================================
// Selector Hooks
// ============================================================

export const useCurrentNotebook = () =>
  useNotebookStore((state) => ({
    notebookId: state.currentNotebookId,
    sessionId: state.activeSessionId,
  }));

export const useSelectedDocuments = () =>
  useNotebookStore((state) => state.selectedDocumentIds);

export const useMainView = () =>
  useNotebookStore((state) => ({
    view: state.mainView,
    activePdfId: state.activePdfId,
  }));

export const useHighlightTarget = () =>
  useNotebookStore((state) => state.highlightTarget);

// ============================================================
// Action Hooks
// ============================================================

export const useNotebookActions = () =>
  useNotebookStore((state) => ({
    setNotebookAndSession: state.setNotebookAndSession,
    setActiveSession: state.setActiveSession,
    toggleDocumentSelection: state.toggleDocumentSelection,
    setSelectedDocuments: state.setSelectedDocuments,
    setMainViewToPdf: state.setMainViewToPdf,
    setMainViewToGuide: state.setMainViewToGuide,
    clearHighlightTarget: state.clearHighlightTarget,
    reset: state.reset,
  }));

// ============================================================
// 工具函数
// ============================================================

export const isHighlightInViewport = (
  highlight: HighlightTarget,
  currentPage: number,
  viewportHeight: number
): boolean => {
  if (highlight.pageNumber !== currentPage) return false;

  const [, y0, , y1] = highlight.boundingBox;
  return y0 >= 0 && y1 <= viewportHeight;
};

export const calculateHighlightStyle = (
  highlight: HighlightTarget,
  pageWidth: number,
  pageHeight: number,
  scale: number
): React.CSSProperties => {
  const [x0, y0, x1, y1] = highlight.boundingBox;

  return {
    position: 'absolute',
    left: `${(x0 / pageWidth) * 100}%`,
    top: `${(y0 / pageHeight) * 100}%`,
    width: `${((x1 - x0) / pageWidth) * 100}%`,
    height: `${((y1 - y0) / pageHeight) * 100}%`,
    backgroundColor: 'rgba(254, 240, 138, 0.5)',
    backdropFilter: 'blur(2px)',
    borderRadius: '2px',
    pointerEvents: 'auto',
    cursor: 'pointer',
    transition: 'all 0.2s ease-out',
    boxShadow: '0 0 0 2px rgba(250, 204, 21, 0.3)',
  };
};
