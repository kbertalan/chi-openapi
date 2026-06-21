// Package pagination provides a generic response envelope for list endpoints.
package pagination

// PaginatedResponse is a generic page of results. The type parameter T is the
// element type, so each instantiation (e.g. PaginatedResponse[users.User])
// monomorphizes into its own component schema with the element's $ref inlined.
type PaginatedResponse[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
}

// New builds a one-page response covering all of items.
func New[T any](items []T) PaginatedResponse[T] {
	return PaginatedResponse[T]{
		Items:      items,
		Page:       1,
		PageSize:   len(items),
		TotalItems: len(items),
	}
}
