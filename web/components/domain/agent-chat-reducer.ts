import type { ToolResultMeta, ChatMessage, Conversation } from "./agent-chat-context";

export type StreamSegment =
  | { type: "text"; content: string }
  | {
      type: "tool";
      callId: string;
      tool: string;
      input: Record<string, unknown>;
      result?: { meta?: ToolResultMeta };
    };

export type State = {
  conversations: Conversation[];
  activeConversationId: string | null;
  isStreaming: boolean;
  streamSegments: StreamSegment[];
  error: string | null;
};

export type Action =
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

export const initialState: State = {
  conversations: [],
  activeConversationId: null,
  isStreaming: false,
  streamSegments: [],
  error: null,
};

export function reducer(state: State, action: Action): State {
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
