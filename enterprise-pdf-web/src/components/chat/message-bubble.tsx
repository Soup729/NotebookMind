"use client";

import type { ChatMessage } from "@/types/api";
import { formatDate } from "@/lib/utils";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function MessageBubble({ message }: { message: ChatMessage }) {
  const isAssistant = message.role === "assistant";

  return (
    <article
      className={`rounded-[1.5rem] border p-4 ${
        isAssistant ? "border-line bg-white/80" : "border-ink bg-ink text-white"
      }`}
    >
      <div className="mb-3 flex items-center justify-between gap-4 text-xs uppercase tracking-[0.2em]">
        <span>{isAssistant ? "Assistant" : "You"}</span>
        <span className={isAssistant ? "text-ink/45" : "text-white/60"}>{formatDate(message.created_at)}</span>
      </div>

      {isAssistant ? (
        <div className="prose prose-sm max-w-none prose-p:leading-7">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
        </div>
      ) : (
        <p className="whitespace-pre-wrap text-sm leading-7">{message.content}</p>
      )}

      {isAssistant && message.sources.length > 0 ? (
        <div className="mt-4 space-y-2">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-ink/45">Sources</p>
          {message.sources.map((source) => (
            <div key={`${source.document_id}-${source.chunk_index}`} className="rounded-2xl border border-line bg-panel p-3">
              <div className="mb-2 flex items-center justify-between gap-3 text-xs text-ink/55">
                <span>{source.file_name || "Unknown file"}</span>
                <span>Score {source.score.toFixed(3)}</span>
              </div>
              <p className="line-clamp-4 text-sm leading-6 text-ink/75">{source.content}</p>
            </div>
          ))}
        </div>
      ) : null}
    </article>
  );
}
