import type { ExportFormat } from '@/types/api';

export interface ExportIntent {
  format: ExportFormat;
  requirements: string;
}

const RULES: Array<{ format: ExportFormat; patterns: RegExp[] }> = [
  { format: 'mindmap', patterns: [/思维导图/i, /脑图/i, /mind\s*map/i, /mindmap/i] },
  { format: 'pptx', patterns: [/ppt/i, /pptx/i, /幻灯片/i, /演示文稿/i] },
  { format: 'docx', patterns: [/word/i, /docx/i, /文档报告/i, /报告文档/i] },
  { format: 'pdf', patterns: [/pdf/i] },
  { format: 'markdown', patterns: [/markdown/i, /\bmd\b/i, /导出.*文档/i, /生成.*文档/i] },
];

const EXPORT_VERBS = /(生成|导出|制作|创建|做一份|整理成|输出)/i;

export function detectExportIntent(input: string): ExportIntent | null {
  const text = input.trim();
  if (!text || !EXPORT_VERBS.test(text)) {
    return null;
  }

  for (const rule of RULES) {
    if (rule.patterns.some((pattern) => pattern.test(text))) {
      return { format: rule.format, requirements: text };
    }
  }

  return null;
}
