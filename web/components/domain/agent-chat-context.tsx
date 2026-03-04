"use client";

import { createContext, useContext, useReducer, useRef } from "react";

// --- Types ---

export type StreamEvent = {
  type:
    | "token"
    | "tool_call"
    | "tool_result"
    | "done"
    | "error"
    | "message_id"
    | "conversation_id";
  content: string;
  tool?: string;
};

export type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "tool_call" | "tool_result";
  content: string;
  docRefs: string[];
  toolName?: string;
  toolInput?: string;
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

type ToolEvent = {
  tool: string;
  type: "call" | "result";
  content: string;
};

type State = {
  conversations: Conversation[];
  activeConversationId: string | null;
  isStreaming: boolean;
  streamingContent: string;
  toolEvents: ToolEvent[];
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
  | { type: "STREAM_START" }
  | { type: "STREAM_TOKEN"; content: string }
  | { type: "STREAM_TOOL"; event: ToolEvent }
  | { type: "STREAM_END" }
  | { type: "SET_ERROR"; error: string | null };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "SET_CONVERSATIONS":
      return { ...state, conversations: action.conversations };
    case "SET_ACTIVE":
      return {
        ...state,
        activeConversationId: action.id,
        streamingContent: "",
        toolEvents: [],
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
    case "STREAM_START":
      return {
        ...state,
        isStreaming: true,
        streamingContent: "",
        toolEvents: [],
        error: null,
      };
    case "STREAM_TOKEN":
      return {
        ...state,
        streamingContent: state.streamingContent + action.content,
      };
    case "STREAM_TOOL":
      return {
        ...state,
        toolEvents: [...state.toolEvents, action.event],
      };
    case "STREAM_END":
      return { ...state, isStreaming: false };
    case "SET_ERROR":
      return { ...state, error: action.error, isStreaming: false };
  }
}

// --- GraphQL helpers ---

async function graphqlQuery<T>(
  query: string,
  variables: Record<string, unknown>,
): Promise<T> {
  const response = await fetch("/api/graphql", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, variables }),
  });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const json = await response.json();
  if (json.errors?.length) throw new Error(json.errors[0].message);
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
  streamingContent: "",
  toolEvents: [],
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

  // Load conversations when vault changes
  const loadConversations = async (vid: string) => {
    if (loadedVaultRef.current === vid) return;
    loadedVaultRef.current = vid;
    try {
      const data = await graphqlQuery<{
        conversations: Conversation[];
      }>(
        `query ($vaultId: ID!) {
          conversations(vaultId: $vaultId) {
            id vaultId title createdAt updatedAt
            messages { id role content docRefs toolName toolInput createdAt }
          }
        }`,
        { vaultId: vid },
      );
      dispatch({ type: "SET_CONVERSATIONS", conversations: data.conversations });
    } catch (err) {
      console.error("Failed to load conversations:", err);
    }
  };

  const newConversation = async (vid: string) => {
    try {
      const data = await graphqlMutate<{
        createConversation: Conversation;
      }>(
        `mutation ($vaultId: ID!) {
          createConversation(vaultId: $vaultId) {
            id vaultId title createdAt updatedAt
            messages { id role content docRefs toolName toolInput createdAt }
          }
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
    // Load messages if not already loaded
    const conv = state.conversations.find((c) => c.id === id);
    if (conv && conv.messages.length === 0) {
      try {
        const data = await graphqlQuery<{
          conversation: Conversation | null;
        }>(
          `query ($id: ID!) {
            conversation(id: $id) {
              id vaultId title createdAt updatedAt
              messages { id role content docRefs toolName toolInput createdAt }
            }
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
    }
  };

  const sendMessage = (content: string, docRefs: string[] = []) => {
    if (state.isStreaming || !vaultId) return;

    // Abort any previous stream
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    dispatch({ type: "STREAM_START" });

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
            try {
              const event: StreamEvent = JSON.parse(jsonStr);

              switch (event.type) {
                case "token":
                  dispatch({ type: "STREAM_TOKEN", content: event.content });
                  break;
                case "tool_call":
                  dispatch({
                    type: "STREAM_TOOL",
                    event: {
                      tool: event.tool ?? "",
                      type: "call",
                      content: event.content,
                    },
                  });
                  break;
                case "tool_result":
                  dispatch({
                    type: "STREAM_TOOL",
                    event: {
                      tool: event.tool ?? "",
                      type: "result",
                      content: event.content,
                    },
                  });
                  break;
                case "conversation_id":
                  newConvId = event.content;
                  // Reload conversations to get the new one
                  if (vaultId) {
                    loadedVaultRef.current = null;
                    await loadConversations(vaultId);
                  }
                  dispatch({ type: "SET_ACTIVE", id: newConvId });
                  break;
                case "error":
                  dispatch({ type: "SET_ERROR", error: event.content });
                  break;
                case "done": {
                  // Reload the conversation to get the final messages
                  if (newConvId) {
                    try {
                      const data = await graphqlQuery<{
                        conversation: Conversation | null;
                      }>(
                        `query ($id: ID!) {
                          conversation(id: $id) {
                            id vaultId title createdAt updatedAt
                            messages { id role content docRefs toolName toolInput createdAt }
                          }
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
                    }
                  }
                  break;
                }
              }
            } catch {
              // Skip malformed SSE lines
            }
          }
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        dispatch({
          type: "SET_ERROR",
          error: err instanceof Error ? err.message : "Stream failed",
        });
      } finally {
        dispatch({ type: "STREAM_END" });
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
