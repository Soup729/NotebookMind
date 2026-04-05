"use client";

import { MessageBubble } from "@/components/chat/message-bubble";
import { Topbar } from "@/components/layout/topbar";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { apiRequest } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import type { ChatMessage, ChatSession } from "@/types/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useEffect, useState } from "react";

export default function ChatPage() {
  const { token } = useAuth();
  const queryClient = useQueryClient();
  const [selectedSessionId, setSelectedSessionId] = useState("");
  const [question, setQuestion] = useState("");
  const [error, setError] = useState("");

  const sessionsQuery = useQuery({
    queryKey: ["chat-sessions"],
    queryFn: () => apiRequest<{ items: ChatSession[] }>("/chat/sessions", {}, token),
    enabled: !!token
  });

  useEffect(() => {
    if (!selectedSessionId && sessionsQuery.data?.items?.length) {
      setSelectedSessionId(sessionsQuery.data.items[0].id);
    }
  }, [selectedSessionId, sessionsQuery.data]);

  const messagesQuery = useQuery({
    queryKey: ["chat-messages", selectedSessionId],
    queryFn: () => apiRequest<{ items: ChatMessage[] }>(`/chat/sessions/${selectedSessionId}/messages`, {}, token),
    enabled: !!token && !!selectedSessionId
  });

  const createSessionMutation = useMutation({
    mutationFn: () => apiRequest<{ session: ChatSession }>("/chat/sessions", { method: "POST", body: "{}" }, token),
    onSuccess: (response) => {
      setSelectedSessionId(response.session.id);
      queryClient.invalidateQueries({ queryKey: ["chat-sessions"] });
    }
  });

  const sendMessageMutation = useMutation({
    mutationFn: async (payload: { sessionId: string; question: string }) =>
      apiRequest<{ session: ChatSession; message: ChatMessage }>(
        `/chat/sessions/${payload.sessionId}/messages`,
        {
          method: "POST",
          body: JSON.stringify({ question: payload.question })
        },
        token
      ),
    onSuccess: (response) => {
      setQuestion("");
      setError("");
      setSelectedSessionId(response.session.id);
      queryClient.invalidateQueries({ queryKey: ["chat-sessions"] });
      queryClient.invalidateQueries({ queryKey: ["chat-messages", response.session.id] });
      queryClient.invalidateQueries({ queryKey: ["dashboard-overview"] });
      queryClient.invalidateQueries({ queryKey: ["usage-summary"] });
    },
    onError: (err) => setError(err instanceof Error ? err.message : "发送失败")
  });

  async function ensureSession() {
    if (selectedSessionId) {
      return selectedSessionId;
    }
    const response = await createSessionMutation.mutateAsync();
    return response.session.id;
  }

  async function handleSend() {
    const trimmed = question.trim();
    if (!trimmed) {
      return;
    }
    const sessionId = await ensureSession();
    await sendMessageMutation.mutateAsync({ sessionId, question: trimmed });
  }

  const sessions = sessionsQuery.data?.items || [];
  const messages = messagesQuery.data?.items || [];

  return (
    <div className="grid gap-6 xl:grid-cols-[320px_1fr]">
      <div className="space-y-4">
        <Topbar title="Chat" subtitle="选择已有会话，或新建一个问答线程并查看带来源的回复。" />
        <Button className="w-full bg-mint" onClick={() => createSessionMutation.mutate()}>
          <Plus className="mr-2 h-4 w-4" />
          新建会话
        </Button>

        <div className="space-y-3">
          {sessions.map((session) => (
            <button
              key={session.id}
              className={`w-full rounded-[1.5rem] border p-4 text-left transition ${
                selectedSessionId === session.id ? "border-ink bg-ink text-white" : "border-line bg-white/75"
              }`}
              onClick={() => setSelectedSessionId(session.id)}
            >
              <p className="font-semibold">{session.title}</p>
              <p className={`mt-2 text-xs ${selectedSessionId === session.id ? "text-white/65" : "text-ink/50"}`}>
                {formatDate(session.last_message_at)}
              </p>
            </button>
          ))}
        </div>
      </div>

      <div className="flex min-h-[70vh] flex-col rounded-[1.75rem] border border-line bg-white/70 p-4 shadow-panel">
        <div className="flex-1 space-y-4 overflow-y-auto pr-2">
          {messages.map((message) => (
            <MessageBubble key={message.id} message={message} />
          ))}
          {!messages.length ? (
            <div className="rounded-[1.5rem] border border-dashed border-line bg-panel p-8 text-center text-sm text-ink/60">
              还没有消息。输入问题后，系统会检索你的文档并生成带来源的回答。
            </div>
          ) : null}
        </div>

        <div className="mt-4 space-y-3 border-t border-line pt-4">
          <Textarea
            rows={5}
            placeholder="例如：请总结本周项目 PDF 中关于架构演进的重点"
            value={question}
            onChange={(event) => setQuestion(event.target.value)}
          />
          {error ? <p className="text-sm text-red-600">{error}</p> : null}
          <div className="flex justify-end">
            <Button onClick={handleSend} disabled={sendMessageMutation.isPending}>
              {sendMessageMutation.isPending ? "生成中..." : "发送问题"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
