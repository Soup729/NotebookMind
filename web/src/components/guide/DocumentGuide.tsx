// ============================================================
// NotebookMind - 文档指南组件
// ============================================================

'use client';

import React, { memo } from 'react';
import { FileText, HelpCircle, Lightbulb, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { useDocumentGuide } from '@/hooks/useNotebook';
import type { FaqItem } from '@/types/api';

interface DocumentGuideProps {
  notebookId: string;
  documentId: string;
  onSuggestedQueryClick?: (query: string) => void;
}

// ============================================================
// 摘要卡片
// ============================================================

interface SummaryCardProps {
  summary: string;
}

function SummaryCard({ summary }: SummaryCardProps) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <FileText className="w-4 h-4 text-primary" />
          文档摘要
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground leading-relaxed">
          {summary}
        </p>
      </CardContent>
    </Card>
  );
}

// ============================================================
// FAQ 卡片
// ============================================================

interface FaqCardProps {
  faq: FaqItem[];
  onItemClick?: (question: string) => void;
}

function FaqCard({ faq, onItemClick }: FaqCardProps) {
  if (!faq || faq.length === 0) return null;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <HelpCircle className="w-4 h-4 text-primary" />
          常见问题
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {faq.map((item, index) => (
          <div
            key={index}
            className="p-3 rounded-lg bg-muted/50 hover:bg-muted transition-colors"
          >
            <p className="text-sm font-medium mb-1">{item.question}</p>
            <p className="text-sm text-muted-foreground">{item.answer}</p>
            {onItemClick && (
              <Button
                variant="ghost"
                size="sm"
                className="mt-2 h-7 text-xs"
                onClick={() => onItemClick(item.question)}
              >
                询问更多
              </Button>
            )}
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

// ============================================================
// 关键要点卡片
// ============================================================

interface KeyPointsCardProps {
  keyPoints: string[];
  onPointClick?: (point: string) => void;
}

function KeyPointsCard({ keyPoints, onPointClick }: KeyPointsCardProps) {
  if (!keyPoints || keyPoints.length === 0) return null;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <Lightbulb className="w-4 h-4 text-primary" />
          关键要点
        </CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-2">
          {keyPoints.map((point, index) => (
            <li
              key={index}
              className="flex items-start gap-2 text-sm"
            >
              <span className="w-1.5 h-1.5 rounded-full bg-primary mt-1.5 flex-shrink-0" />
              <span className="text-muted-foreground">{point}</span>
            </li>
          ))}
        </ul>
        {onPointClick && (
          <Button
            variant="outline"
            size="sm"
            className="mt-4 w-full"
            onClick={() => onPointClick(keyPoints[0])}
          >
            基于要点提问
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

// ============================================================
// 建议问题组件
// ============================================================

interface SuggestedQueriesProps {
  documentId: string;
  faqQuestions?: string[];
  onQueryClick?: (query: string) => void;
}

function SuggestedQueries({ documentId, faqQuestions, onQueryClick }: SuggestedQueriesProps) {
  // 优先使用文档 FAQ 提取的问题，否则使用默认问题
  const queries = faqQuestions && faqQuestions.length > 0
    ? faqQuestions
    : [
        '这篇文档的主要内容是什么？',
        '有哪些关键信息需要关注？',
        '文档中讨论了哪些重要概念？',
      ];

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">建议问题</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap gap-2">
          {queries.map((query, index) => (
            <Button
              key={index}
              variant="secondary"
              size="sm"
              onClick={() => onQueryClick?.(query)}
              className="text-xs"
            >
              {query}
            </Button>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

// ============================================================
// 加载骨架
// ============================================================

function GuideSkeleton() {
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-2">
          <Skeleton className="h-5 w-24" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-4 w-full mb-2" />
          <Skeleton className="h-4 w-3/4" />
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <Skeleton className="h-5 w-16" />
        </CardHeader>
        <CardContent className="space-y-3">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </CardContent>
      </Card>
    </div>
  );
}

// ============================================================
// 文档指南主组件
// ============================================================

export const DocumentGuide = memo(function DocumentGuide({
  notebookId,
  documentId,
  onSuggestedQueryClick,
}: DocumentGuideProps) {
  const { parsedGuide, isLoading, error } = useDocumentGuide(notebookId, documentId);

  // 判断是否为"生成中"的预期状态
  const isPending = parsedGuide?.status === 'pending';
  const isNotFound = error && (
    String(error).toLowerCase().includes('not found') ||
    error.message?.includes('Not Found') ||
    error.message?.includes('not found')
  );

  if (isLoading || isPending || isNotFound) {
    return (
      <Card className="p-8 text-center">
        <Loader2 className="w-6 h-6 animate-spin mx-auto mb-3 text-primary" />
        <p className="text-sm font-medium text-muted-foreground">正在分析文档并生成指南</p>
        <p className="text-xs text-muted-foreground/60 mt-1">首次处理需要 10-30 秒，请稍候...</p>
      </Card>
    );
  }

  if (!parsedGuide) {
    return (
      <Card className="p-4 text-center">
        <p className="text-sm text-muted-foreground">无法加载文档指南</p>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {/* 摘要 */}
      {parsedGuide.summary && (
        <SummaryCard summary={parsedGuide.summary} />
      )}

      {/* FAQ */}
      {parsedGuide.faq && parsedGuide.faq.length > 0 && (
        <FaqCard faq={parsedGuide.faq} onItemClick={onSuggestedQueryClick} />
      )}

      {/* 关键要点 */}
      {parsedGuide.keyPoints && parsedGuide.keyPoints.length > 0 && (
        <KeyPointsCard
          keyPoints={parsedGuide.keyPoints}
          onPointClick={onSuggestedQueryClick}
        />
      )}

      {/* 建议问题 — 基于文档 FAQ 动态生成 */}
      <SuggestedQueries
        documentId={documentId}
        faqQuestions={parsedGuide.faq.map((item) => item.question)}
        onSuggestedQueryClick={onSuggestedQueryClick}
      />
    </div>
  );
});
