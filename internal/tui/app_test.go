package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"github.com/raphi011/know/internal/tools"
)

// testModel creates a minimal Model for testing state logic
// without needing a real Client or glamour renderer. Sets ready
// and termReady to bypass initialization guards.
func testModel() Model {
	return Model{
		ready:     true,
		termReady: true,
		width:     80,
		height:    24,
		input:     textinput.New(),
		ctx:       context.Background(),
		cancel:    func() {},
	}
}

func TestAppendText(t *testing.T) {
	t.Run("empty parts creates new PartText", func(t *testing.T) {
		m := testModel()
		m.appendText("hello")
		if len(m.streamParts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(m.streamParts))
		}
		if m.streamParts[0].Type != PartText || m.streamParts[0].Content != "hello" {
			t.Errorf("expected PartText with 'hello', got %+v", m.streamParts[0])
		}
	})

	t.Run("coalesces consecutive text", func(t *testing.T) {
		m := testModel()
		m.appendText("hello ")
		m.appendText("world")
		if len(m.streamParts) != 1 {
			t.Fatalf("expected 1 part after coalescing, got %d", len(m.streamParts))
		}
		if m.streamParts[0].Content != "hello world" {
			t.Errorf("expected 'hello world', got %q", m.streamParts[0].Content)
		}
	})

	t.Run("no coalescing across types", func(t *testing.T) {
		m := testModel()
		m.appendText("before")
		m.streamParts = append(m.streamParts, ContentPart{Type: PartToolCall, ToolName: "search"})
		m.appendText("after")
		if len(m.streamParts) != 3 {
			t.Fatalf("expected 3 parts, got %d", len(m.streamParts))
		}
		if m.streamParts[2].Content != "after" {
			t.Errorf("expected 'after', got %q", m.streamParts[2].Content)
		}
	})
}

func TestFindToolPart(t *testing.T) {
	t.Run("finds by callID", func(t *testing.T) {
		m := testModel()
		m.streamParts = []ContentPart{
			{Type: PartText, Content: "hello"},
			{Type: PartToolCall, CallID: "call-1", ToolName: "search"},
			{Type: PartToolCall, CallID: "call-2", ToolName: "read"},
		}
		p := m.findToolPart("call-1")
		if p == nil || p.ToolName != "search" {
			t.Errorf("expected search tool, got %v", p)
		}
	})

	t.Run("returns most recent match", func(t *testing.T) {
		m := testModel()
		m.streamParts = []ContentPart{
			{Type: PartToolCall, CallID: "call-1", ToolName: "search"},
			{Type: PartToolCall, CallID: "call-1", ToolName: "search-v2"},
		}
		p := m.findToolPart("call-1")
		if p == nil || p.ToolName != "search-v2" {
			t.Errorf("expected most recent match (search-v2), got %v", p)
		}
	})

	t.Run("empty callID returns nil", func(t *testing.T) {
		m := testModel()
		m.streamParts = []ContentPart{
			{Type: PartToolCall, CallID: "call-1", ToolName: "search"},
		}
		if p := m.findToolPart(""); p != nil {
			t.Errorf("expected nil for empty callID, got %+v", p)
		}
	})

	t.Run("missing callID returns nil", func(t *testing.T) {
		m := testModel()
		m.streamParts = []ContentPart{
			{Type: PartToolCall, CallID: "call-1", ToolName: "search"},
		}
		if p := m.findToolPart("nonexistent"); p != nil {
			t.Errorf("expected nil for missing callID, got %+v", p)
		}
	})
}

func TestUpdateToolStatus(t *testing.T) {
	t.Run("matches by callID", func(t *testing.T) {
		m := testModel()
		m.streamParts = []ContentPart{
			{Type: PartToolCall, CallID: "call-1", ToolName: "search", Status: ToolRunning},
		}
		meta := &tools.ToolResultMeta{ResultCount: new(5)}
		m.updateToolStatus("call-1", "search", ToolComplete, meta)

		if m.streamParts[0].Status != ToolComplete {
			t.Errorf("expected ToolComplete, got %v", m.streamParts[0].Status)
		}
		if m.streamParts[0].Meta != meta {
			t.Error("expected meta to be set")
		}
	})

	t.Run("falls back to toolName when callID empty", func(t *testing.T) {
		m := testModel()
		m.streamParts = []ContentPart{
			{Type: PartToolCall, CallID: "call-1", ToolName: "search", Status: ToolRunning},
		}
		m.updateToolStatus("", "search", ToolComplete, nil)

		if m.streamParts[0].Status != ToolComplete {
			t.Errorf("expected ToolComplete via name fallback, got %v", m.streamParts[0].Status)
		}
	})

	t.Run("no match leaves parts unchanged", func(t *testing.T) {
		m := testModel()
		m.streamParts = []ContentPart{
			{Type: PartToolCall, CallID: "call-1", ToolName: "search", Status: ToolRunning},
		}
		m.updateToolStatus("nonexistent", "nonexistent", ToolComplete, nil)

		if m.streamParts[0].Status != ToolRunning {
			t.Errorf("expected status unchanged, got %v", m.streamParts[0].Status)
		}
	})
}

func TestHandleStreamEvent_Text(t *testing.T) {
	m := testModel()
	m.streaming = true

	msg := streamEventMsg{
		event: StreamEvent{Type: "text", Content: "Hello from the agent"},
	}
	result, cmd := m.handleStreamEvent(msg)
	rm := result.(Model)

	if cmd != nil {
		t.Error("expected nil cmd when msg.ch is nil")
	}
	if len(rm.streamParts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(rm.streamParts))
	}
	if rm.streamParts[0].Content != "Hello from the agent" {
		t.Errorf("expected text content, got %q", rm.streamParts[0].Content)
	}
}

func TestHandleStreamEvent_ToolLifecycle(t *testing.T) {
	m := testModel()
	m.streaming = true

	// tool_start — creates new part
	startMsg := streamEventMsg{
		event: StreamEvent{
			Type:   "tool_start",
			Tool:   "search_documents",
			CallID: "call-42",
			Input:  map[string]any{"query": "test"},
		},
	}
	result, cmd := m.handleStreamEvent(startMsg)
	if cmd != nil {
		t.Error("expected nil cmd when msg.ch is nil")
	}
	m = result.(Model)

	if len(m.streamParts) != 1 {
		t.Fatalf("expected 1 part after tool_start, got %d", len(m.streamParts))
	}
	if m.streamParts[0].Status != ToolRunning {
		t.Errorf("expected ToolRunning, got %v", m.streamParts[0].Status)
	}

	// tool_end
	meta := &tools.ToolResultMeta{ResultCount: new(3)}
	endMsg := streamEventMsg{
		event: StreamEvent{
			Type:   "tool_end",
			Tool:   "search_documents",
			CallID: "call-42",
			Meta:   meta,
		},
	}
	result, cmd = m.handleStreamEvent(endMsg)
	if cmd != nil {
		t.Error("expected nil cmd when msg.ch is nil")
	}
	m = result.(Model)

	if m.streamParts[0].Status != ToolComplete {
		t.Errorf("expected ToolComplete after tool_end, got %v", m.streamParts[0].Status)
	}
	if m.streamParts[0].Meta != meta {
		t.Error("expected meta to be set after tool_end")
	}
}

func TestHandleStreamEvent_ToolStartReuse(t *testing.T) {
	m := testModel()
	m.streaming = true

	// First tool_start creates the part.
	msg1 := streamEventMsg{
		event: StreamEvent{
			Type:   "tool_start",
			Tool:   "edit_document",
			CallID: "call-99",
			Input:  map[string]any{"path": "/doc.md"},
		},
	}
	result, _ := m.handleStreamEvent(msg1)
	m = result.(Model)

	// Simulate tool completing (e.g. before approval).
	m.streamParts[0].Status = ToolComplete
	m.streamParts[0].Meta = &tools.ToolResultMeta{ContentLength: new(100)}

	// Second tool_start with same CallID reuses the existing part.
	msg2 := streamEventMsg{
		event: StreamEvent{
			Type:   "tool_start",
			Tool:   "edit_document",
			CallID: "call-99",
			Input:  map[string]any{"path": "/doc.md"},
		},
	}
	result, _ = m.handleStreamEvent(msg2)
	m = result.(Model)

	if len(m.streamParts) != 1 {
		t.Fatalf("expected 1 part (reused), got %d", len(m.streamParts))
	}
	if m.streamParts[0].Status != ToolRunning {
		t.Errorf("expected status reset to ToolRunning, got %v", m.streamParts[0].Status)
	}
	if m.streamParts[0].Meta != nil {
		t.Error("expected meta cleared on reuse")
	}
}

func TestHandleStreamEvent_Error(t *testing.T) {
	m := testModel()
	m.streaming = true
	m.streamParts = []ContentPart{{Type: PartText, Content: "partial"}}

	msg := streamEventMsg{
		event: StreamEvent{Type: "error", Content: "agent crashed"},
	}
	result, cmd := m.handleStreamEvent(msg)
	rm := result.(Model)

	// Error event finalizes the stream.
	if rm.streaming {
		t.Error("expected streaming=false after error")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Batch with finalize + next)")
	}

	// Should have appended a PartError before finalization cleared parts.
	// Since finalizeStream sets streamParts=nil, we check that it was called
	// by verifying streaming is false. The PartError was rendered into
	// scrollback via tea.Println.
}

func TestHandleStreamEvent_ConvID(t *testing.T) {
	m := testModel()
	m.streaming = true
	m.conversationID = "old-id"

	msg := streamEventMsg{
		event: StreamEvent{Type: "conv_id", ConvID: "new-conv-123"},
	}
	result, cmd := m.handleStreamEvent(msg)
	rm := result.(Model)

	if cmd != nil {
		t.Error("expected nil cmd when msg.ch is nil")
	}
	if rm.conversationID != "new-conv-123" {
		t.Errorf("expected conversationID='new-conv-123', got %q", rm.conversationID)
	}
}

func TestHandleStreamEvent_Interrupted(t *testing.T) {
	t.Run("first interruption creates dialog", func(t *testing.T) {
		m := testModel()
		m.streaming = true

		msg := streamEventMsg{
			event: StreamEvent{
				Type:        "interrupted",
				Tool:        "edit_document",
				InterruptID: "int-1",
			},
		}
		result, cmd := m.handleStreamEvent(msg)
		if cmd != nil {
			t.Error("expected nil cmd when msg.ch is nil")
		}
		rm := result.(Model)

		if rm.dialog == nil {
			t.Fatal("expected dialog to be created")
		}
		if len(rm.dialog.approvals) != 1 {
			t.Fatalf("expected 1 pending approval, got %d", len(rm.dialog.approvals))
		}
		if rm.dialog.approvals[0].decision != decisionPending {
			t.Error("expected decisionPending")
		}
		if rm.dialog.approvals[0].event.Tool != "edit_document" {
			t.Errorf("expected tool 'edit_document', got %q", rm.dialog.approvals[0].event.Tool)
		}
	})

	t.Run("subsequent interruption appends to dialog", func(t *testing.T) {
		m := testModel()
		m.streaming = true

		// First interruption.
		msg1 := streamEventMsg{
			event: StreamEvent{Type: "interrupted", Tool: "edit_document", InterruptID: "int-1"},
		}
		result, _ := m.handleStreamEvent(msg1)
		m = result.(Model)

		// Second interruption.
		msg2 := streamEventMsg{
			event: StreamEvent{Type: "interrupted", Tool: "create_document", InterruptID: "int-2"},
		}
		result, _ = m.handleStreamEvent(msg2)
		m = result.(Model)

		if len(m.dialog.approvals) != 2 {
			t.Fatalf("expected 2 pending approvals, got %d", len(m.dialog.approvals))
		}
		if m.dialog.approvals[1].event.Tool != "create_document" {
			t.Errorf("expected second approval for 'create_document', got %q", m.dialog.approvals[1].event.Tool)
		}
	})
}

func TestHandleStreamEvent_MsgEnd(t *testing.T) {
	m := testModel()
	m.streaming = true
	m.streamParts = []ContentPart{{Type: PartText, Content: "response"}}

	msg := streamEventMsg{
		event: StreamEvent{
			Type:              "msg_end",
			InputTokens:       1500,
			OutputTokens:      300,
			ContextWindowMax:  200_000,
			ContextWindowUsed: 50_000,
		},
	}
	result, cmd := m.handleStreamEvent(msg)
	rm := result.(Model)

	if rm.streaming {
		t.Error("expected streaming=false after msg_end")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Batch with finalize)")
	}
	if rm.tokenInput != 1500 {
		t.Errorf("expected tokenInput=1500, got %d", rm.tokenInput)
	}
	if rm.tokenOutput != 300 {
		t.Errorf("expected tokenOutput=300, got %d", rm.tokenOutput)
	}
	if rm.contextWindowMax != 200_000 {
		t.Errorf("expected contextWindowMax=200000, got %d", rm.contextWindowMax)
	}
	if rm.contextWindowUsed != 50_000 {
		t.Errorf("expected contextWindowUsed=50000, got %d", rm.contextWindowUsed)
	}
}

func TestHandleStreamEvent_Done(t *testing.T) {
	m := testModel()
	m.streaming = true
	m.streamParts = []ContentPart{{Type: PartText, Content: "final"}}

	msg := streamEventMsg{done: true}
	result, _ := m.handleStreamEvent(msg)
	rm := result.(Model)

	if rm.streaming {
		t.Error("expected streaming=false after done")
	}
	if rm.streamParts != nil {
		t.Error("expected streamParts=nil after done")
	}
}

func TestFinalizeStream(t *testing.T) {
	t.Run("clears streaming state and returns cmd", func(t *testing.T) {
		m := testModel()
		m.streaming = true
		m.streamParts = []ContentPart{{Type: PartText, Content: "done"}}
		cmd := m.finalizeStream()

		if m.streaming {
			t.Error("expected streaming=false")
		}
		if m.streamParts != nil {
			t.Error("expected streamParts=nil")
		}
		if cmd == nil {
			t.Error("expected non-nil cmd (tea.Println)")
		}
	})

	t.Run("dialog blocks finalization", func(t *testing.T) {
		m := testModel()
		m.streaming = true
		m.streamParts = []ContentPart{{Type: PartText, Content: "pending"}}
		m.dialog = &approvalDialog{
			approvals: []pendingApproval{{event: StreamEvent{Tool: "edit"}}},
		}
		cmd := m.finalizeStream()

		if !m.streaming {
			t.Error("expected streaming to remain true while dialog active")
		}
		if m.streamParts == nil {
			t.Error("expected streamParts to remain while dialog active")
		}
		if cmd != nil {
			t.Error("expected nil cmd when dialog blocks finalization")
		}
	})

	t.Run("not streaming is noop", func(t *testing.T) {
		m := testModel()
		m.streaming = false
		cmd := m.finalizeStream()
		if cmd != nil {
			t.Error("expected nil cmd when not streaming")
		}
	})
}

func TestToolKeyArg(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  string
	}{
		{"path tool", "read_document", map[string]any{"path": "/notes/test.md"}, "/notes/test.md"},
		{"query tool", "search_documents", map[string]any{"query": "golang testing"}, "golang testing"},
		{"folder tool", "list_folder_contents", map[string]any{"folder": "/docs"}, "/docs"},
		{"title tool", "create_memory", map[string]any{"title": "TDD patterns"}, "TDD patterns"},
		{"unknown tool", "unknown_tool", map[string]any{"path": "/x"}, ""},
		{"nil input", "read_document", nil, ""},
		{"missing field", "read_document", map[string]any{"other": "field"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolKeyArg(tt.tool, tt.input)
			if got != tt.want {
				t.Errorf("toolKeyArg(%q, %v) = %q, want %q", tt.tool, tt.input, got, tt.want)
			}
		})
	}
}

func TestToolDetail(t *testing.T) {
	t.Run("nil meta returns empty", func(t *testing.T) {
		if got := toolDetail("read_document", nil); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("read_document with content length", func(t *testing.T) {
		meta := &tools.ToolResultMeta{ContentLength: new(1234)}
		got := toolDetail("read_document", meta)
		if got != "1234 chars" {
			t.Errorf("expected '1234 chars', got %q", got)
		}
	})

	t.Run("search_documents with counts", func(t *testing.T) {
		meta := &tools.ToolResultMeta{ResultCount: new(5), ChunkCount: new(12)}
		got := toolDetail("search_documents", meta)
		if !strings.Contains(got, "5 docs") || !strings.Contains(got, "12 chunks") {
			t.Errorf("expected '5 docs, 12 chunks', got %q", got)
		}
	})

	t.Run("search_documents with only result count", func(t *testing.T) {
		meta := &tools.ToolResultMeta{ResultCount: new(5)}
		got := toolDetail("search_documents", meta)
		if got != "5 docs" {
			t.Errorf("expected '5 docs', got %q", got)
		}
	})

	t.Run("web_search with results", func(t *testing.T) {
		meta := &tools.ToolResultMeta{WebResultCount: new(10)}
		got := toolDetail("web_search", meta)
		if got != "10 results" {
			t.Errorf("expected '10 results', got %q", got)
		}
	})
}

func TestRenderToolStatus(t *testing.T) {
	t.Run("running shows tool name and arg", func(t *testing.T) {
		p := ContentPart{
			Type:     PartToolCall,
			ToolName: "search_documents",
			Input:    map[string]any{"query": "test"},
			Status:   ToolRunning,
		}
		got := renderToolStatus(p)
		if !strings.Contains(got, "search_documents") {
			t.Errorf("expected tool name in output, got %q", got)
		}
		if !strings.Contains(got, "test") {
			t.Errorf("expected key arg in output, got %q", got)
		}
	})

	t.Run("complete with detail", func(t *testing.T) {
		p := ContentPart{
			Type:     PartToolCall,
			ToolName: "read_document",
			Input:    map[string]any{"path": "/doc.md"},
			Status:   ToolComplete,
			Meta:     &tools.ToolResultMeta{ContentLength: new(500)},
		}
		got := renderToolStatus(p)
		if !strings.Contains(got, "500 chars") {
			t.Errorf("expected detail in output, got %q", got)
		}
	})

	t.Run("failed shows failed suffix", func(t *testing.T) {
		p := ContentPart{
			Type:     PartToolCall,
			ToolName: "edit_document",
			Status:   ToolFailed,
		}
		got := renderToolStatus(p)
		if !strings.Contains(got, "failed") {
			t.Errorf("expected 'failed' in output, got %q", got)
		}
	})
}
