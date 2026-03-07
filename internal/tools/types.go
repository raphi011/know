// Package tools provides a shared tool executor used by both the agent chat
// and the embedded MCP server.
package tools

// ToolResultMeta contains structured metadata about a tool execution result.
type ToolResultMeta struct {
	DurationMs     int64        `json:"durationMs"`
	ResultCount    *int         `json:"resultCount,omitempty"`
	ChunkCount     *int         `json:"chunkCount,omitempty"`
	MatchedDocs    []ToolDocRef `json:"matchedDocs,omitempty"`
	DocumentPath   *string      `json:"documentPath,omitempty"`
	DocumentTitle  *string      `json:"documentTitle,omitempty"`
	ContentLength  *int         `json:"contentLength,omitempty"`
	WebResultCount *int         `json:"webResultCount,omitempty"`
	WebSources     []ToolWebRef `json:"webSources,omitempty"`
}

// ToolDocRef references a matched KB document.
type ToolDocRef struct {
	Title string  `json:"title"`
	Path  string  `json:"path"`
	Score float64 `json:"score"`
}

// ToolWebRef references a web search result.
type ToolWebRef struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// IntPtr returns a pointer to the given int value.
func IntPtr(v int) *int { return &v }
