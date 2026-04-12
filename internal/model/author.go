package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/database/dbq"
)

// AuthorResponse is the JSON-serialisable representation of an author.
type AuthorResponse struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Bio        *string    `json:"bio"`
	BornDate   *string    `json:"born_date"`
	DiedDate   *string    `json:"died_date"`
	ImageID    *uuid.UUID `json:"image_id"`
	CategoryID *uuid.UUID `json:"category_id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateAuthorRequest is the expected JSON body for creating an author.
type CreateAuthorRequest struct {
	Name       string     `json:"name"`
	Bio        *string    `json:"bio"`
	BornDate   *string    `json:"born_date"`
	DiedDate   *string    `json:"died_date"`
	ImageID    *uuid.UUID `json:"image_id"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// UpdateAuthorRequest is the expected JSON body for updating an author.
type UpdateAuthorRequest struct {
	Name       string     `json:"name"`
	Bio        *string    `json:"bio"`
	BornDate   *string    `json:"born_date"`
	DiedDate   *string    `json:"died_date"`
	ImageID    *uuid.UUID `json:"image_id"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// AuthorFromDB converts a database row into an AuthorResponse.
func AuthorFromDB(a dbq.Author) AuthorResponse {
	resp := AuthorResponse{
		ID:        uuidFromPgtype(a.ID),
		Name:      a.Name,
		BornDate:  DateFromPgtype(a.BornDate),
		DiedDate:  DateFromPgtype(a.DiedDate),
		CreatedAt: a.CreatedAt.Time,
		UpdatedAt: a.UpdatedAt.Time,
	}
	if a.Bio.Valid {
		resp.Bio = &a.Bio.String
	}
	if a.ImageID.Valid {
		id := uuidFromPgtype(a.ImageID)
		resp.ImageID = &id
	}
	if a.CategoryID.Valid {
		id := uuidFromPgtype(a.CategoryID)
		resp.CategoryID = &id
	}
	return resp
}

// AuthorsFromDB converts a slice of database rows into AuthorResponse values.
func AuthorsFromDB(authors []dbq.Author) []AuthorResponse {
	out := make([]AuthorResponse, len(authors))
	for i, a := range authors {
		out[i] = AuthorFromDB(a)
	}
	return out
}
