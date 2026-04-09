package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/database/dbq"
)

// CategoryResponse is the JSON-serialisable representation of a category.
type CategoryResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateCategoryRequest is the expected JSON body for creating a category.
type CreateCategoryRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// UpdateCategoryRequest is the expected JSON body for updating a category.
type UpdateCategoryRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CategoryFromDB converts a database row into a CategoryResponse.
func CategoryFromDB(c dbq.Category) CategoryResponse {
	return CategoryResponse{
		ID:        uuidFromPgtype(c.ID),
		Name:      c.Name,
		Type:      c.Type,
		CreatedAt: c.CreatedAt.Time,
	}
}

// CategoriesFromDB converts a slice of database rows into CategoryResponse values.
func CategoriesFromDB(cats []dbq.Category) []CategoryResponse {
	out := make([]CategoryResponse, len(cats))
	for i, c := range cats {
		out[i] = CategoryFromDB(c)
	}
	return out
}
