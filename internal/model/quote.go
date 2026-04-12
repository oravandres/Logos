package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/database/dbq"
)

// QuoteResponse is the JSON-serialisable representation of a quote.
type QuoteResponse struct {
	ID         uuid.UUID  `json:"id"`
	Title      string     `json:"title"`
	Text       string     `json:"text"`
	AuthorID   uuid.UUID  `json:"author_id"`
	ImageID    *uuid.UUID `json:"image_id"`
	CategoryID *uuid.UUID `json:"category_id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateQuoteRequest is the expected JSON body for creating a quote.
type CreateQuoteRequest struct {
	Title      string     `json:"title"`
	Text       string     `json:"text"`
	AuthorID   uuid.UUID  `json:"author_id"`
	ImageID    *uuid.UUID `json:"image_id"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// UpdateQuoteRequest is the expected JSON body for updating a quote.
type UpdateQuoteRequest struct {
	Title      string     `json:"title"`
	Text       string     `json:"text"`
	AuthorID   uuid.UUID  `json:"author_id"`
	ImageID    *uuid.UUID `json:"image_id"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// QuoteFromDB converts a database row into a QuoteResponse.
func QuoteFromDB(q dbq.Quote) QuoteResponse {
	resp := QuoteResponse{
		ID:        uuidFromPgtype(q.ID),
		Title:     q.Title,
		Text:      q.Text,
		AuthorID:  uuidFromPgtype(q.AuthorID),
		CreatedAt: q.CreatedAt.Time,
		UpdatedAt: q.UpdatedAt.Time,
	}
	if q.ImageID.Valid {
		id := uuidFromPgtype(q.ImageID)
		resp.ImageID = &id
	}
	if q.CategoryID.Valid {
		id := uuidFromPgtype(q.CategoryID)
		resp.CategoryID = &id
	}
	return resp
}

// QuotesFromDB converts a slice of database rows into QuoteResponse values.
func QuotesFromDB(quotes []dbq.Quote) []QuoteResponse {
	out := make([]QuoteResponse, len(quotes))
	for i, q := range quotes {
		out[i] = QuoteFromDB(q)
	}
	return out
}
