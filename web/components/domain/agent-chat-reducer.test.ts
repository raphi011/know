import { describe, it, expect } from "vitest";
import { reducer, initialState, type State } from "./agent-chat-reducer";

describe("agent-chat-reducer", () => {
  it("MSG_START clears segments and sets streaming", () => {
    const before: State = {
      ...initialState,
      streamSegments: [{ type: "text", content: "old" }],
      error: "old error",
    };
    const after = reducer(before, { type: "MSG_START" });
    expect(after.isStreaming).toBe(true);
    expect(after.streamSegments).toEqual([]);
    expect(after.error).toBeNull();
  });

  it("STREAM_TEXT appends to existing text segment", () => {
    const before: State = {
      ...initialState,
      isStreaming: true,
      streamSegments: [{ type: "text", content: "Hello" }],
    };
    const after = reducer(before, { type: "STREAM_TEXT", content: " world" });
    expect(after.streamSegments).toEqual([{ type: "text", content: "Hello world" }]);
  });

  it("STREAM_TEXT after tool creates new text segment", () => {
    const before: State = {
      ...initialState,
      isStreaming: true,
      streamSegments: [
        { type: "text", content: "Before" },
        { type: "tool", callId: "c1", tool: "kb_search", input: { query: "test" } },
      ],
    };
    const after = reducer(before, { type: "STREAM_TEXT", content: "After" });
    expect(after.streamSegments).toHaveLength(3);
    expect(after.streamSegments[2]).toEqual({ type: "text", content: "After" });
  });

  it("STREAM_TEXT on empty segments creates first text segment", () => {
    const before: State = { ...initialState, isStreaming: true };
    const after = reducer(before, { type: "STREAM_TEXT", content: "First" });
    expect(after.streamSegments).toEqual([{ type: "text", content: "First" }]);
  });

  it("TOOL_START creates tool segment", () => {
    const before: State = { ...initialState, isStreaming: true };
    const after = reducer(before, {
      type: "TOOL_START",
      callId: "c1",
      tool: "kb_search",
      input: { query: "kubernetes" },
    });
    expect(after.streamSegments).toEqual([
      { type: "tool", callId: "c1", tool: "kb_search", input: { query: "kubernetes" } },
    ]);
  });

  it("TOOL_END attaches result by callId", () => {
    const meta = { durationMs: 42, resultCount: 3 };
    const before: State = {
      ...initialState,
      isStreaming: true,
      streamSegments: [
        { type: "tool", callId: "c1", tool: "kb_search", input: { query: "test" } },
        { type: "tool", callId: "c2", tool: "read_document", input: { path: "/doc" } },
      ],
    };
    const after = reducer(before, { type: "TOOL_END", callId: "c1", meta });
    const seg = after.streamSegments[0];
    expect(seg).toMatchObject({ type: "tool", callId: "c1", result: { meta } });
    // c2 should be unchanged
    const seg2 = after.streamSegments[1]!;
    expect(seg2).toMatchObject({ type: "tool", callId: "c2" });
    expect(seg2.type === "tool" ? seg2.result : undefined).toBeUndefined();
  });

  it("MSG_END clears segments and stops streaming", () => {
    const before: State = {
      ...initialState,
      isStreaming: true,
      streamSegments: [
        { type: "text", content: "hello" },
        { type: "tool", callId: "c1", tool: "kb_search", input: {} },
      ],
    };
    const after = reducer(before, { type: "MSG_END" });
    expect(after.isStreaming).toBe(false);
    expect(after.streamSegments).toEqual([]);
  });

  it("SET_ERROR stops streaming", () => {
    const before: State = { ...initialState, isStreaming: true };
    const after = reducer(before, { type: "SET_ERROR", error: "boom" });
    expect(after.isStreaming).toBe(false);
    expect(after.error).toBe("boom");
  });

  it("full interleaved sequence: text -> tool_start -> tool_end -> text = 3 segments", () => {
    let state: State = { ...initialState };

    state = reducer(state, { type: "MSG_START" });
    state = reducer(state, { type: "STREAM_TEXT", content: "Let me search." });
    state = reducer(state, {
      type: "TOOL_START",
      callId: "c1",
      tool: "kb_search",
      input: { query: "kubernetes" },
    });
    state = reducer(state, {
      type: "TOOL_END",
      callId: "c1",
      meta: { durationMs: 100, resultCount: 2 },
    });
    state = reducer(state, { type: "STREAM_TEXT", content: "Based on the results..." });

    expect(state.streamSegments).toHaveLength(3);
    expect(state.streamSegments[0]).toEqual({ type: "text", content: "Let me search." });
    expect(state.streamSegments[1]).toMatchObject({
      type: "tool",
      callId: "c1",
      tool: "kb_search",
      result: { meta: { durationMs: 100, resultCount: 2 } },
    });
    expect(state.streamSegments[2]).toEqual({ type: "text", content: "Based on the results..." });
  });
});
