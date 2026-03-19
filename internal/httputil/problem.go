package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// ProblemDetail is an RFC 9457 (Problem Details for HTTP APIs) response body.
// All error responses (4xx, 5xx) use this format with Content-Type: application/problem+json.
type ProblemDetail struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// ProblemContentType is the IANA media type for RFC 9457 problem details.
const ProblemContentType = "application/problem+json"

// WriteProblem writes an RFC 9457 Problem Details JSON response.
// The title is derived from the HTTP status code; detail is the caller's message.
func WriteProblem(w http.ResponseWriter, status int, detail string) {
	p := ProblemDetail{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	}
	w.Header().Set("Content-Type", ProblemContentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		slog.Warn("failed to encode problem detail", "error", err)
	}
}

// WriteProblemWithRetry writes an RFC 9457 Problem Details response with a Retry-After header.
func WriteProblemWithRetry(w http.ResponseWriter, status int, detail string, retryAfterSecs int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSecs))
	WriteProblem(w, status, detail)
}
