"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import {
  PlusIcon,
  PaperAirplaneIcon,
  TrashIcon,
  MagnifyingGlassIcon,
  DocumentTextIcon,
} from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";
import { useAgentChat } from "@/components/domain/agent-chat-context";
import { MarkdownRenderer } from "@/components/domain/markdown-renderer";
import { DocRefAutocomplete } from "@/components/domain/doc-ref-autocomplete";

type AgentChatPanelProps = {
  vaultId: string | null;
};

function AgentChatPanel({ vaultId }: AgentChatPanelProps) {
  const t = useTranslations("docs");
  const chat = useAgentChat();
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const [input, setInput] = useState("");
  const [docRefs, setDocRefs] = useState<string[]>([]);
  const [showAutocomplete, setShowAutocomplete] = useState(false);

  const activeConv = chat.conversations.find(
    (c) => c.id === chat.activeConversationId,
  );
  const activeMessages = activeConv?.messages ?? [];

  // Load conversations when vault changes
  useEffect(() => {
    if (vaultId) {
      chat.loadConversations(vaultId);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vaultId]);

  // Auto-scroll to bottom
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [chat.streamingContent, activeMessages.length]);

  function handleSend() {
    const trimmed = input.trim();
    if (!trimmed || chat.isStreaming) return;
    chat.sendMessage(trimmed, docRefs);
    setInput("");
    setDocRefs([]);
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
    if (e.key === "@") {
      setShowAutocomplete(true);
    }
  }

  function handleDocRefSelect(path: string) {
    if (!docRefs.includes(path)) {
      setDocRefs([...docRefs, path]);
    }
    setShowAutocomplete(false);
  }

  function removeDocRef(path: string) {
    setDocRefs(docRefs.filter((r) => r !== path));
  }

  if (!vaultId) {
    return (
      <div className="flex flex-col items-center justify-center p-4 text-center text-sm text-slate-500 dark:text-slate-400">
        {t("noVaultTitle")}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Conversation header */}
      <div className="flex items-center gap-1 border-b border-slate-200 px-2 py-1.5 dark:border-slate-700">
        <button
          type="button"
          onClick={() => vaultId && chat.newConversation(vaultId)}
          className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
          title={t("agentNewChat")}
        >
          <PlusIcon className="size-4" />
        </button>
        {chat.conversations.length > 0 && (
          <select
            value={chat.activeConversationId ?? ""}
            onChange={(e) => {
              if (e.target.value) chat.switchConversation(e.target.value);
            }}
            className="min-w-0 flex-1 truncate rounded bg-transparent px-1 py-0.5 text-xs text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
          >
            {chat.conversations.map((conv) => (
              <option key={conv.id} value={conv.id}>
                {conv.title}
              </option>
            ))}
          </select>
        )}
        {activeConv && (
          <button
            type="button"
            onClick={() => chat.deleteConversation(activeConv.id)}
            className="rounded p-1 text-slate-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950 dark:hover:text-red-400"
            title={t("agentDeleteConversation")}
          >
            <TrashIcon className="size-3.5" />
          </button>
        )}
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-2 py-2">
        {activeMessages.length === 0 && !chat.isStreaming && (
          <div className="mt-8 text-center text-xs text-slate-400 dark:text-slate-500">
            {t("agentPlaceholder")}
          </div>
        )}
        {activeMessages
          .filter((m) => m.role === "user" || m.role === "assistant")
          .map((msg) => (
            <ChatBubble key={msg.id} message={msg} />
          ))}

        {/* Tool indicators */}
        {chat.toolEvents.map((event, i) => (
          <ToolIndicator key={i} event={event} />
        ))}

        {/* Streaming content */}
        {chat.isStreaming && chat.streamingContent && (
          <div className="mb-2">
            <div className="rounded-lg bg-slate-50 px-2.5 py-2 text-xs dark:bg-slate-800">
              <MarkdownRenderer
                content={chat.streamingContent}
                className="prose-xs"
              />
            </div>
          </div>
        )}

        {/* Typing indicator */}
        {chat.isStreaming && !chat.streamingContent && chat.toolEvents.length === 0 && (
          <div className="mb-2">
            <div className="flex gap-1 rounded-lg bg-slate-50 px-3 py-2 dark:bg-slate-800">
              <span className="size-1.5 animate-bounce rounded-full bg-slate-400 [animation-delay:0ms]" />
              <span className="size-1.5 animate-bounce rounded-full bg-slate-400 [animation-delay:150ms]" />
              <span className="size-1.5 animate-bounce rounded-full bg-slate-400 [animation-delay:300ms]" />
            </div>
          </div>
        )}

        {/* Error */}
        {chat.error && (
          <div className="mb-2 rounded-lg bg-red-50 px-2.5 py-2 text-xs text-red-600 dark:bg-red-950 dark:text-red-400">
            {chat.error}
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input area */}
      <div className="border-t border-slate-200 p-2 dark:border-slate-700">
        {/* Doc ref badges */}
        {docRefs.length > 0 && (
          <div className="mb-1.5 flex flex-wrap gap-1">
            {docRefs.map((ref) => (
              <span
                key={ref}
                className="inline-flex items-center gap-0.5 rounded-full bg-primary-50 px-2 py-0.5 text-[10px] text-primary-700 dark:bg-primary-950 dark:text-primary-300"
              >
                @{ref.split("/").pop()}
                <button
                  type="button"
                  onClick={() => removeDocRef(ref)}
                  className="ml-0.5 hover:text-red-500"
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        )}

        {/* Autocomplete */}
        {showAutocomplete && (
          <DocRefAutocomplete
            onSelect={handleDocRefSelect}
            onClose={() => setShowAutocomplete(false)}
          />
        )}

        <div className="flex items-end gap-1.5">
          <button
            type="button"
            onClick={() => setShowAutocomplete(!showAutocomplete)}
            className="mb-0.5 rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
            title={t("agentReferenceDoc")}
          >
            <DocumentTextIcon className="size-4" />
          </button>
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t("agentPlaceholder")}
            disabled={chat.isStreaming}
            rows={1}
            className={cn(
              "min-h-[32px] max-h-[100px] flex-1 resize-none rounded-lg bg-white px-2.5 py-1.5 text-xs",
              "text-slate-900 placeholder:text-slate-400",
              "ring-1 ring-slate-300 transition-colors",
              "focus:outline-none focus:ring-2 focus:ring-primary-500",
              "disabled:opacity-50",
              "dark:bg-slate-900 dark:text-white dark:ring-slate-700",
            )}
          />
          <button
            type="button"
            onClick={handleSend}
            disabled={chat.isStreaming || !input.trim()}
            className={cn(
              "mb-0.5 rounded-lg p-1.5 transition-colors",
              "bg-primary-600 text-white hover:bg-primary-700",
              "disabled:bg-slate-200 disabled:text-slate-400",
              "dark:disabled:bg-slate-700 dark:disabled:text-slate-500",
            )}
            title={t("agentSend")}
          >
            <PaperAirplaneIcon className="size-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}

// --- Sub-components ---

function ChatBubble({
  message,
}: {
  message: { role: string; content: string };
}) {
  const isUser = message.role === "user";

  return (
    <div className={cn("mb-2 flex", isUser ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "max-w-[90%] rounded-lg px-2.5 py-2 text-xs",
          isUser
            ? "bg-primary-600 text-white"
            : "bg-slate-50 text-slate-900 dark:bg-slate-800 dark:text-slate-100",
        )}
      >
        {isUser ? (
          <p className="whitespace-pre-wrap">{message.content}</p>
        ) : (
          <MarkdownRenderer content={message.content} className="prose-xs" />
        )}
      </div>
    </div>
  );
}

function ToolIndicator({
  event,
}: {
  event: { tool: string; type: string; content: string };
}) {
  const t = useTranslations("docs");
  const icon =
    event.tool === "kb_search" ? (
      <MagnifyingGlassIcon className="size-3" />
    ) : (
      <DocumentTextIcon className="size-3" />
    );

  const label =
    event.type === "call"
      ? event.tool === "kb_search"
        ? t("agentToolSearching", { query: event.content })
        : t("agentToolReading", { path: event.content })
      : event.tool === "kb_search"
        ? t("agentToolFound", { count: event.content })
        : t("agentToolRead", { path: event.content });

  return (
    <div className="mb-1.5 flex items-center gap-1.5 rounded bg-amber-50 px-2 py-1 text-[10px] text-amber-700 dark:bg-amber-950 dark:text-amber-300">
      {icon}
      <span className="truncate">{label}</span>
    </div>
  );
}

export { AgentChatPanel };
