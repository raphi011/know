package httputil

// ListResponse is the standard JSON envelope for all list endpoints.
// Every list endpoint returns {"items": [...], "total": N}.
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// NewListResponse creates a ListResponse, ensuring Items is never nil
// so it serializes as [] instead of null.
func NewListResponse[T any](items []T, total int) ListResponse[T] {
	if items == nil {
		items = []T{}
	}
	return ListResponse[T]{Items: items, Total: total}
}
