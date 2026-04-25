'use client';

import { Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import type { ExportOutlineSection } from '@/types/api';

interface ExportOutlineEditorProps {
  outline: ExportOutlineSection[];
  onChange: (outline: ExportOutlineSection[]) => void;
}

export function ExportOutlineEditor({ outline, onChange }: ExportOutlineEditorProps) {
  const updateSection = (index: number, patch: Partial<ExportOutlineSection>) => {
    onChange(outline.map((section, i) => (i === index ? { ...section, ...patch } : section)));
  };

  const updateBullets = (index: number, value: string) => {
    updateSection(index, {
      bullets: value
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
    });
  };

  const addSection = () => {
    onChange([...outline, { heading: '新章节', bullets: ['补充要点'] }]);
  };

  const removeSection = (index: number) => {
    if (outline.length <= 1) return;
    onChange(outline.filter((_, i) => i !== index));
  };

  return (
    <div className="space-y-3">
      {outline.map((section, index) => (
        <div key={index} className="border rounded-md p-3 space-y-2 bg-background">
          <div className="flex items-center gap-2">
            <Input
              value={section.heading}
              onChange={(event) => updateSection(index, { heading: event.target.value })}
              placeholder="章节标题"
              className="h-8"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8 hover:text-destructive"
              onClick={() => removeSection(index)}
              disabled={outline.length <= 1}
              title="删除章节"
            >
              <Trash2 className="w-4 h-4" />
            </Button>
          </div>
          <Textarea
            value={(section.bullets || []).join('\n')}
            onChange={(event) => updateBullets(index, event.target.value)}
            placeholder="每行一个要点"
            className="min-h-[86px]"
          />
          {section.source_refs && section.source_refs.length > 0 && (
            <div className="text-xs text-muted-foreground">
              来源：{section.source_refs.map((ref) => ref.document_name || ref.document_id || '文档').join('、')}
            </div>
          )}
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" className="gap-2" onClick={addSection}>
        <Plus className="w-4 h-4" />
        添加章节
      </Button>
    </div>
  );
}
