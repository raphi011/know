"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import {
  PlusIcon,
  PaperAirplaneIcon,
  TrashIcon,
  DocumentTextIcon,
} from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";
import {
  useAgentChat,
  type ChatMessage,
} from "@/components/domain/agent-chat-context";
import { MarkdownRenderer } from "@/components/domain/markdown-renderer";
import { DocRefAutocomplete } from "@/components/domain/doc-ref-autocomplete";
import { ToolCard } from "@/components/domain/tool-card";

type AgentChatPanelProps = {
  vaultId: string | null;
};

// Pair adjacent tool_call + tool_result messages into ToolCard-compatible entries
type PairedEntry =
  | { type: "message"; message: ChatMessage }
  | {
      type: "tool_card";
      tool: string;
      callContent: string;
      result?: {
        content: string;
        meta?: ChatMessage["toolMeta"];
      };
    };

/** Extract display-friendly content from the stored JSON tool input. */
function extractCallContent(toolInput: string | undefined, tool: string): string {
  if (!toolInput) return "";
  try {
    const parsed = JSON.parse(toolInput) as Record<string, unknown>;
    if (tool === "kb_search" || tool === "web_search") return String(parsed.query ?? toolInput);
    if (tool === "read_document") return String(parsed.path ?? toolInput);
  } catch { /* not JSON, use as-is */ }
  return toolInput;
}

function pairMessages(messages: ChatMessage[]): PairedEntry[] {
  const entries: PairedEntry[] = [];
  let i = 0;

  while (i < messages.length) {
    const msg = messages[i]!;

    if (msg.role === "tool_call") {
      const tool = msg.toolName ?? "kb_search";
      const callContent = extractCallContent(msg.toolInput, tool);

      // Look ahead for a matching tool_result
      const next = messages[i + 1];
      if (next?.role === "tool_result" && next.toolName === msg.toolName) {
        entries.push({
          type: "tool_card",
          tool,
          callContent,
          result: { content: next.content, meta: next.toolMeta },
        });
        i += 2;
        continue;
      }
      // Orphaned tool_call (no matching result)
      entries.push({ type: "tool_card", tool, callContent });
      i++;
      continue;
    }

    if (msg.role === "tool_result") {
      // Orphaned tool_result (no preceding call) — skip it
      i++;
      continue;
    }

    entries.push({ type: "message", message: msg });
    i++;
  }

  return entries;
}

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
  const pairedEntries = pairMessages(activeMessages);

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
  }, [chat.streamingContent, activeMessages.length, chat.toolExecutions.length]);

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

        {/* Persisted messages — paired as ToolCards or ChatBubbles */}
        {pairedEntries.map((entry, i) =>
          entry.type === "tool_card" ? (
            <ToolCard
              key={`tool-${i}`}
              tool={entry.tool}
              callContent={entry.callContent}
              result={entry.result}
            />
          ) : (
            <ChatBubble key={entry.message.id} message={entry.message} />
          ),
        )}

        {/* Streaming tool executions */}
        {chat.toolExecutions.map((exec, i) => (
          <ToolCard
            key={`stream-tool-${i}`}
            tool={exec.tool}
            callContent={exec.callContent}
            result={exec.result}
          />
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
        {chat.isStreaming &&
          !chat.streamingContent &&
          chat.toolExecutions.length === 0 && (
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
                  x
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

export { AgentChatPanel };
