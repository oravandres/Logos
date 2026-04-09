package model

// Pagination defaults.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// PaginatedResponse wraps a page of results with total count and offset metadata.
type PaginatedResponse[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}
