package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/database/dbq"
)

// TagResponse is the JSON-serialisable representation of a tag.
type TagResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateTagRequest is the expected JSON body for creating a tag.
type CreateTagRequest struct {
	Name string `json:"name"`
}

// TagFromDB converts a database row into a TagResponse.
func TagFromDB(t dbq.Tag) TagResponse {
	return TagResponse{
		ID:        uuidFromPgtype(t.ID),
		Name:      t.Name,
		CreatedAt: t.CreatedAt.Time,
	}
}

// TagsFromDB converts a slice of database rows into TagResponse values.
func TagsFromDB(tags []dbq.Tag) []TagResponse {
	out := make([]TagResponse, len(tags))
	for i, t := range tags {
		out[i] = TagFromDB(t)
	}
	return out
}
