'use client';

import { useMemo, useState, type ReactNode } from 'react';
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
  type NodeMouseHandler,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { AlertCircle, GitBranch, Loader2, Maximize2, Network } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useNotebookGraph } from '@/hooks/useNotebookGraph';
import type { KnowledgeGraphNode } from '@/types/api';

interface KnowledgeGraphPanelProps {
  notebookId: string;
}

const typeStyles: Record<string, { bg: string; border: string; text: string }> = {
  concept: { bg: '#ecfeff', border: '#0891b2', text: '#164e63' },
  method: { bg: '#f0fdf4', border: '#16a34a', text: '#14532d' },
  metric: { bg: '#fff7ed', border: '#ea580c', text: '#7c2d12' },
  dataset: { bg: '#f5f3ff', border: '#7c3aed', text: '#4c1d95' },
  org: { bg: '#fef2f2', border: '#dc2626', text: '#7f1d1d' },
};

export function KnowledgeGraphPanel({ notebookId }: KnowledgeGraphPanelProps) {
  const { graph, isLoading, error, mutate } = useNotebookGraph(notebookId);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  const selectedNode = useMemo(
    () => graph?.nodes.find((node) => node.id === selectedNodeId) || null,
    [graph?.nodes, selectedNodeId]
  );

  const { nodes, edges } = useMemo(() => {
    const graphNodes = graph?.nodes || [];
    const graphEdges = graph?.edges || [];
    const radius = Math.max(180, Math.min(520, graphNodes.length * 26));
    const center = radius + 80;
    const nodes: Node[] = graphNodes.map((node, index) => {
      const angle = (index / Math.max(1, graphNodes.length)) * Math.PI * 2;
      const style = typeStyles[node.type] || typeStyles.concept;
      const importance = Math.min(20, Math.max(0, node.size * 2));
      return {
        id: node.id,
        data: {
          label: `${node.label}\n${node.type}`,
        },
        position: {
          x: center + Math.cos(angle) * radius,
          y: center + Math.sin(angle) * radius,
        },
        style: {
          width: 128 + importance,
          minHeight: 48,
          border: `1px solid ${style.border}`,
          borderRadius: 8,
          background: style.bg,
          color: style.text,
          fontSize: 12,
          whiteSpace: 'pre-line',
          textAlign: 'center',
          padding: 10,
          boxShadow: selectedNodeId === node.id ? `0 0 0 3px ${style.border}33` : 'none',
        },
      };
    });
    const edges: Edge[] = graphEdges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      label: edge.label,
      animated: edge.weight > 1,
      style: { strokeWidth: Math.min(4, Math.max(1, edge.weight)), stroke: '#64748b' },
      labelStyle: { fontSize: 11, fill: '#334155' },
    }));
    return { nodes, edges };
  }, [graph?.edges, graph?.nodes, selectedNodeId]);

  const handleNodeClick: NodeMouseHandler = (_, node) => {
    setSelectedNodeId(node.id);
  };

  return (
    <section className="rounded-lg border bg-background p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Network className="h-4 w-4 text-primary" />
          <div>
            <h3 className="font-medium">知识图谱</h3>
            <p className="text-xs text-muted-foreground">
              {graph?.stats.entities || 0} 个实体 · {graph?.stats.relations || 0} 条关系 · {graph?.stats.documents || 0} 个文档
              {graph?.semantic_index_status ? ` · 语义索引 ${graph.semantic_index_status}` : ''}
            </p>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={() => mutate()} className="gap-2">
          <Maximize2 className="h-4 w-4" />
          刷新
        </Button>
      </div>

      {isLoading ? (
        <GraphLoading />
      ) : error ? (
        <GraphState
          icon={<AlertCircle className="h-5 w-5 text-destructive" />}
          title="知识图谱加载失败"
          description={error instanceof Error ? error.message : '请稍后重试'}
        />
      ) : graph?.status === 'building' ? (
        <GraphState
          icon={<Loader2 className="h-5 w-5 animate-spin text-primary" />}
          title="正在生成知识图谱"
          description="文档解析完成后会自动抽取实体和关系。"
        />
      ) : !graph || graph.nodes.length === 0 ? (
        <GraphState
          icon={<GitBranch className="h-5 w-5 text-muted-foreground" />}
          title="还没有可展示的知识图谱"
          description="上传并解析文档后，系统会自动生成 notebook 级实体关系图。"
        />
      ) : (
        <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
          <div className="h-[420px] overflow-hidden rounded-md border bg-muted/20">
            <ReactFlow
              nodes={nodes}
              edges={edges}
              fitView
              minZoom={0.2}
              maxZoom={1.8}
              onNodeClick={handleNodeClick}
              proOptions={{ hideAttribution: true }}
            >
              <Background gap={18} size={1} />
              <Controls />
              <MiniMap pannable zoomable nodeStrokeWidth={3} />
            </ReactFlow>
          </div>
          <NodeDetails node={selectedNode || graph.nodes[0]} selected={Boolean(selectedNode)} />
        </div>
      )}
    </section>
  );
}

function NodeDetails({ node, selected }: { node: KnowledgeGraphNode; selected: boolean }) {
  return (
    <aside className="rounded-md border bg-muted/20 p-3">
      <p className="text-xs text-muted-foreground">{selected ? '当前实体' : '高频实体'}</p>
      <h4 className="mt-1 text-sm font-semibold">{node.label}</h4>
      <div className="mt-2 flex flex-wrap gap-2 text-xs">
        <span className={cn('rounded border px-2 py-1', 'bg-background')}>{node.type}</span>
        <span className="rounded border bg-background px-2 py-1">权重 {node.size}</span>
        <span className="rounded border bg-background px-2 py-1">置信 {node.confidence.toFixed(2)}</span>
      </div>
      {node.documents.length > 0 && (
        <div className="mt-3">
          <p className="mb-1 text-xs font-medium text-muted-foreground">来源文档</p>
          <div className="space-y-1">
            {node.documents.slice(0, 4).map((doc) => (
              <p key={doc.id} className="truncate text-xs">{doc.name || doc.id}</p>
            ))}
          </div>
        </div>
      )}
      {node.evidence.length > 0 && (
        <div className="mt-3">
          <p className="mb-1 text-xs font-medium text-muted-foreground">证据片段</p>
          <div className="space-y-2">
            {node.evidence.slice(0, 3).map((evidence) => (
              <div key={`${evidence.document_id}-${evidence.chunk_id}-${evidence.page}`} className="rounded bg-background p-2">
                <p className="line-clamp-3 text-xs text-muted-foreground">{evidence.text}</p>
                <p className="mt-1 text-[11px] text-muted-foreground">
                  {evidence.document_name || evidence.document_id}
                  {evidence.page ? ` · 第 ${evidence.page} 页` : ''}
                </p>
              </div>
            ))}
          </div>
        </div>
      )}
    </aside>
  );
}

function GraphLoading() {
  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
      <Skeleton className="h-[420px] w-full rounded-md" />
      <div className="space-y-2 rounded-md border p-3">
        <Skeleton className="h-4 w-1/3" />
        <Skeleton className="h-6 w-2/3" />
        <Skeleton className="h-16 w-full" />
      </div>
    </div>
  );
}

function GraphState({ icon, title, description }: { icon: ReactNode; title: string; description: string }) {
  return (
    <div className="flex min-h-[260px] items-center justify-center rounded-md border border-dashed bg-muted/20 p-6 text-center">
      <div className="max-w-sm space-y-2">
        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-full bg-background">{icon}</div>
        <p className="font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
    </div>
  );
}
