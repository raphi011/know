"use client";

import { createContext, useContext, useEffect, useReducer, useRef } from "react";

// --- Types ---

export type ToolResultMeta = {
  durationMs: number;
  resultCount?: number;
  chunkCount?: number;
  matchedDocs?: { title: string; path: string; score: number }[];
  documentPath?: string;
  documentTitle?: string;
  contentLength?: number;
  webResultCount?: number;
  webSources?: { title: string; url: string }[];
};

export type StreamEvent = {
  type: "text" | "tool_start" | "tool_end" | "msg_start" | "msg_end" | "conv_id" | "error";
  content?: string;
  convId?: string;
  msgId?: string;
  callId?: string;
  tool?: string;
  input?: Record<string, unknown>;
  meta?: ToolResultMeta;
};

export type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "tool_result";
  content: string;
  docRefs: string[];
  toolName?: string;
  toolInput?: string;
  toolMeta?: ToolResultMeta;
  toolCallId?: string;
  toolCalls?: string;
  createdAt: string;
};

export type Conversation = {
  id: string;
  vaultId: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  messages: ChatMessage[];
};

export type StreamSegment =
  | { type: "text"; content: string }
  | {
      type: "tool";
      callId: string;
      tool: string;
      input: Record<string, unknown>;
      result?: { meta?: ToolResultMeta };
    };

type State = {
  conversations: Conversation[];
  activeConversationId: string | null;
  isStreaming: boolean;
  streamSegments: StreamSegment[];
  error: string | null;
};

type Action =
  | { type: "SET_CONVERSATIONS"; conversations: Conversation[] }
  | { type: "SET_ACTIVE"; id: string | null }
  | { type: "ADD_CONVERSATION"; conversation: Conversation }
  | { type: "REMOVE_CONVERSATION"; id: string }
  | { type: "UPDATE_TITLE"; id: string; title: string }
  | { type: "SET_MESSAGES"; id: string; messages: ChatMessage[] }
  | { type: "ADD_MESSAGE"; id: string; message: ChatMessage }
  | { type: "MSG_START" }
  | { type: "STREAM_TEXT"; content: string }
  | { type: "TOOL_START"; callId: string; tool: string; input: Record<string, unknown> }
  | { type: "TOOL_END"; callId: string; meta?: ToolResultMeta }
  | { type: "MSG_END" }
  | { type: "SET_ERROR"; error: string | null };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "SET_CONVERSATIONS":
      return { ...state, conversations: action.conversations };
    case "SET_ACTIVE":
      return {
        ...state,
        activeConversationId: action.id,
        streamSegments: [],
        error: null,
      };
    case "ADD_CONVERSATION":
      return {
        ...state,
        conversations: [action.conversation, ...state.conversations],
        activeConversationId: action.conversation.id,
      };
    case "REMOVE_CONVERSATION": {
      const filtered = state.conversations.filter(
        (c) => c.id !== action.id,
      );
      return {
        ...state,
        conversations: filtered,
        activeConversationId:
          state.activeConversationId === action.id
            ? (filtered[0]?.id ?? null)
            : state.activeConversationId,
      };
    }
    case "UPDATE_TITLE":
      return {
        ...state,
        conversations: state.conversations.map((c) =>
          c.id === action.id ? { ...c, title: action.title } : c,
        ),
      };
    case "SET_MESSAGES":
      return {
        ...state,
        conversations: state.conversations.map((c) =>
          c.id === action.id ? { ...c, messages: action.messages } : c,
        ),
      };
    case "ADD_MESSAGE":
      return {
        ...state,
        conversations: state.conversations.map((c) =>
          c.id === action.id
            ? { ...c, messages: [...c.messages, action.message] }
            : c,
        ),
      };
    case "MSG_START":
      return {
        ...state,
        isStreaming: true,
        streamSegments: [],
        error: null,
      };
    case "STREAM_TEXT": {
      const segments = [...state.streamSegments];
      const last = segments[segments.length - 1];
      if (last && last.type === "text") {
        segments[segments.length - 1] = { type: "text", content: last.content + action.content };
      } else {
        segments.push({ type: "text", content: action.content });
      }
      return { ...state, streamSegments: segments };
    }
    case "TOOL_START":
      return {
        ...state,
        streamSegments: [
          ...state.streamSegments,
          { type: "tool", callId: action.callId, tool: action.tool, input: action.input },
        ],
      };
    case "TOOL_END": {
      const segments = state.streamSegments.map((seg) =>
        seg.type === "tool" && seg.callId === action.callId
          ? { ...seg, result: { meta: action.meta } }
          : seg,
      );
      return { ...state, streamSegments: segments };
    }
    case "MSG_END":
      return { ...state, isStreaming: false, streamSegments: [] };
    case "SET_ERROR":
      return { ...state, error: action.error, isStreaming: false };
  }
}

// --- GraphQL helpers ---

const TOOL_META_FIELDS = `toolMeta {
  durationMs resultCount chunkCount
  matchedDocs { title path score }
  documentPath documentTitle contentLength
  webResultCount
  webSources { title url }
}`;

const MESSAGE_FIELDS = `id role content docRefs toolName toolInput ${TOOL_META_FIELDS} toolCallId toolCalls createdAt`;
const CONVERSATION_FIELDS = `id vaultId title createdAt updatedAt messages { ${MESSAGE_FIELDS} }`;

async function graphqlQuery<T>(
  query: string,
  variables: Record<string, unknown>,
): Promise<T> {
  const response = await fetch("/api/graphql", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, variables }),
  });
  if (!response.ok) throw new Error(`GraphQL request failed: HTTP ${response.status}`);
  let json: { data?: T; errors?: Array<{ message?: string }> };
  try {
    json = await response.json();
  } catch {
    throw new Error("GraphQL response was not valid JSON");
  }
  if (json.errors?.length)
    throw new Error(json.errors[0]?.message ?? "Unknown GraphQL error");
  if (!json.data)
    throw new Error("GraphQL response missing 'data' field");
  return json.data;
}

async function graphqlMutate<T>(
  query: string,
  variables: Record<string, unknown>,
): Promise<T> {
  return graphqlQuery<T>(query, variables);
}

// --- Context ---

type AgentChatContextValue = State & {
  sendMessage: (content: string, docRefs?: string[]) => void;
  newConversation: (vaultId: string) => Promise<void>;
  switchConversation: (id: string) => Promise<void>;
  deleteConversation: (id: string) => Promise<void>;
  loadConversations: (vaultId: string) => Promise<void>;
};

const AgentChatContext = createContext<AgentChatContextValue | null>(null);

const initialState: State = {
  conversations: [],
  activeConversationId: null,
  isStreaming: false,
  streamSegments: [],
  error: null,
};

export function AgentChatProvider({
  vaultId,
  children,
}: {
  vaultId: string | null;
  children: React.ReactNode;
}) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const abortRef = useRef<AbortController | null>(null);
  const loadedVaultRef = useRef<string | null>(null);
  const stateRef = useRef(state);

  // Keep stateRef in sync for use in async callbacks
  useEffect(() => {
    stateRef.current = state;
  });

  // Abort any in-flight stream on unmount
  useEffect(() => () => abortRef.current?.abort(), []);

  // Load conversations when vault changes
  const loadConversations = async (vid: string) => {
    if (loadedVaultRef.current === vid) return;
    loadedVaultRef.current = vid;
    try {
      const data = await graphqlQuery<{
        conversations: Conversation[];
      }>(
        `query ($vaultId: ID!) {
          conversations(vaultId: $vaultId) { ${CONVERSATION_FIELDS} }
        }`,
        { vaultId: vid },
      );
      dispatch({ type: "SET_CONVERSATIONS", conversations: data.conversations });
      // Auto-select the most recent conversation and load its messages
      const first = data.conversations[0];
      if (first) {
        dispatch({ type: "SET_ACTIVE", id: first.id });
        try {
          const msgData = await graphqlQuery<{
            conversation: Conversation | null;
          }>(
            `query ($id: ID!) {
              conversation(id: $id) { ${CONVERSATION_FIELDS} }
            }`,
            { id: first.id },
          );
          if (msgData.conversation) {
            dispatch({
              type: "SET_MESSAGES",
              id: first.id,
              messages: msgData.conversation.messages,
            });
          }
        } catch (err) {
          console.error("Failed to load messages for active conversation:", err);
          dispatch({
            type: "SET_ERROR",
            error: err instanceof Error ? err.message : "Failed to load messages",
          });
        }
      }
    } catch (err) {
      console.error("Failed to load conversations:", err);
      dispatch({
        type: "SET_ERROR",
        error: err instanceof Error ? err.message : "Failed to load conversations",
      });
    }
  };

  const newConversation = async (vid: string) => {
    try {
      const data = await graphqlMutate<{
        createConversation: Conversation;
      }>(
        `mutation ($vaultId: ID!) {
          createConversation(vaultId: $vaultId) { ${CONVERSATION_FIELDS} }
        }`,
        { vaultId: vid },
      );
      dispatch({ type: "ADD_CONVERSATION", conversation: data.createConversation });
    } catch (err) {
      console.error("Failed to create conversation:", err);
      dispatch({
        type: "SET_ERROR",
        error: err instanceof Error ? err.message : "Failed to create conversation",
      });
    }
  };

  const switchConversation = async (id: string) => {
    dispatch({ type: "SET_ACTIVE", id });
    // Load messages if not already loaded — read from ref to avoid stale closure
    const conv = stateRef.current.conversations.find((c) => c.id === id);
    if (conv && conv.messages.length === 0) {
      try {
        const data = await graphqlQuery<{
          conversation: Conversation | null;
        }>(
          `query ($id: ID!) {
            conversation(id: $id) { ${CONVERSATION_FIELDS} }
          }`,
          { id },
        );
        if (data.conversation) {
          dispatch({
            type: "SET_MESSAGES",
            id,
            messages: data.conversation.messages,
          });
        }
      } catch (err) {
        console.error("Failed to load messages:", err);
        dispatch({
          type: "SET_ERROR",
          error: err instanceof Error ? err.message : "Failed to load messages",
        });
      }
    }
  };

  const deleteConversation = async (id: string) => {
    try {
      await graphqlMutate(
        `mutation ($id: ID!) { deleteConversation(id: $id) }`,
        { id },
      );
      dispatch({ type: "REMOVE_CONVERSATION", id });
    } catch (err) {
      console.error("Failed to delete conversation:", err);
      dispatch({
        type: "SET_ERROR",
        error: err instanceof Error ? err.message : "Failed to delete conversation",
      });
    }
  };

  const sendMessage = (content: string, docRefs: string[] = []) => {
    if (state.isStreaming) return;
    if (!vaultId) {
      dispatch({ type: "SET_ERROR", error: "No vault selected" });
      return;
    }

    // Abort any previous stream
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    dispatch({ type: "MSG_START" });

    // Add optimistic user message
    const convId = state.activeConversationId;
    if (convId) {
      const optimisticMsg: ChatMessage = {
        id: `temp-${Date.now()}`,
        role: "user",
        content,
        docRefs,
        createdAt: new Date().toISOString(),
      };
      dispatch({ type: "ADD_MESSAGE", id: convId, message: optimisticMsg });
    }

    const doStream = async () => {
      try {
        const response = await fetch("/api/agent/chat", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          signal: controller.signal,
          body: JSON.stringify({
            conversationId: convId || undefined,
            vaultId,
            content,
            docRefs,
          }),
        });

        if (!response.ok) {
          const text = await response.text().catch(() => "Unknown error");
          dispatch({ type: "SET_ERROR", error: text });
          return;
        }

        const reader = response.body?.getReader();
        if (!reader) {
          dispatch({ type: "SET_ERROR", error: "No response body" });
          return;
        }

        const decoder = new TextDecoder();
        let buffer = "";
        let newConvId = convId;

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() ?? "";

          for (const line of lines) {
            if (!line.startsWith("data: ")) continue;
            const jsonStr = line.slice(6);

            let event: StreamEvent;
            try {
              event = JSON.parse(jsonStr);
            } catch (parseErr) {
              console.warn("Skipping malformed SSE event", jsonStr, parseErr);
              continue;
            }

            switch (event.type) {
              case "text":
                dispatch({ type: "STREAM_TEXT", content: event.content ?? "" });
                break;
              case "tool_start":
                dispatch({
                  type: "TOOL_START",
                  callId: event.callId ?? "",
                  tool: event.tool ?? "",
                  input: event.input ?? {},
                });
                break;
              case "tool_end":
                dispatch({
                  type: "TOOL_END",
                  callId: event.callId ?? "",
                  meta: event.meta,
                });
                break;
              case "conv_id":
                newConvId = event.convId ?? event.content ?? "";
                // Reload conversations to get the new one
                if (vaultId) {
                  loadedVaultRef.current = null;
                  await loadConversations(vaultId);
                }
                dispatch({ type: "SET_ACTIVE", id: newConvId });
                break;
              case "error":
                dispatch({ type: "SET_ERROR", error: event.content ?? "Unknown error" });
                break;
              case "msg_end": {
                // Reload the conversation to get the final messages
                if (newConvId) {
                  try {
                    const data = await graphqlQuery<{
                      conversation: Conversation | null;
                    }>(
                      `query ($id: ID!) {
                        conversation(id: $id) { ${CONVERSATION_FIELDS} }
                      }`,
                      { id: newConvId },
                    );
                    if (data.conversation) {
                      dispatch({
                        type: "SET_MESSAGES",
                        id: newConvId,
                        messages: data.conversation.messages,
                      });
                      // Update title in case it was auto-generated
                      dispatch({
                        type: "UPDATE_TITLE",
                        id: newConvId,
                        title: data.conversation.title,
                      });
                    }
                  } catch (err) {
                    console.error("Failed to reload conversation:", err);
                    dispatch({
                      type: "SET_ERROR",
                      error:
                        err instanceof Error
                          ? err.message
                          : "Failed to reload conversation",
                    });
                  }
                }
                dispatch({ type: "MSG_END" });
                break;
              }
            }
          }
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        dispatch({
          type: "SET_ERROR",
          error: err instanceof Error ? err.message : "Stream failed",
        });
      }
    };

    doStream();
  };

  const value: AgentChatContextValue = {
    ...state,
    sendMessage,
    newConversation,
    switchConversation,
    deleteConversation,
    loadConversations,
  };

  return <AgentChatContext value={value}>{children}</AgentChatContext>;
}

export function useAgentChat(): AgentChatContextValue {
  const ctx = useContext(AgentChatContext);
  if (ctx === null) {
    throw new Error("useAgentChat must be used within an AgentChatProvider");
  }
  return ctx;
}
