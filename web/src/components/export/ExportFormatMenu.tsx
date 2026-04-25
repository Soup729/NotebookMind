'use client';

import type { ElementType } from 'react';
import { Download, FileText, GitBranch, Presentation, FileType, File } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import type { ExportFormat } from '@/types/api';

interface ExportFormatMenuProps {
  disabled?: boolean;
  onSelect: (format: ExportFormat) => void;
}

const ITEMS: Array<{ format: ExportFormat; label: string; icon: ElementType }> = [
  { format: 'markdown', label: 'Markdown', icon: FileText },
  { format: 'mindmap', label: '思维导图', icon: GitBranch },
  { format: 'docx', label: 'Word', icon: FileType },
  { format: 'pptx', label: 'PPT', icon: Presentation },
  { format: 'pdf', label: 'PDF', icon: File },
];

export function ExportFormatMenu({ disabled, onSelect }: ExportFormatMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="gap-2"
          disabled={disabled}
          title={disabled ? '没有可导出的已完成文档' : '导出'}
        >
          <Download className="w-4 h-4" />
          导出
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        {ITEMS.map((item) => {
          const Icon = item.icon;
          return (
            <DropdownMenuItem key={item.format} onClick={() => onSelect(item.format)}>
              <Icon className="w-4 h-4 mr-2" />
              {item.label}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
