package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

// quoteResponseFromFields is the single source of truth for mapping the eight
// canonical quote columns into a QuoteResponse. Each per-query sqlc row
// (ListQuotesRow, GetQuoteRow, CreateQuoteRow, UpdateQuoteRow,
// ListQuotesByTagRow) carries the same eight fields by different names; the
// per-row wrappers below forward into this helper so the UUID / optional-UUID
// / timestamp handling lives in exactly one place.
func quoteResponseFromFields(
	id pgtype.UUID,
	title, text string,
	authorID, imageID, categoryID pgtype.UUID,
	createdAt, updatedAt pgtype.Timestamptz,
) QuoteResponse {
	resp := QuoteResponse{
		ID:        uuidFromPgtype(id),
		Title:     title,
		Text:      text,
		AuthorID:  uuidFromPgtype(authorID),
		CreatedAt: createdAt.Time,
		UpdatedAt: updatedAt.Time,
	}
	if imageID.Valid {
		v := uuidFromPgtype(imageID)
		resp.ImageID = &v
	}
	if categoryID.Valid {
		v := uuidFromPgtype(categoryID)
		resp.CategoryID = &v
	}
	return resp
}

// QuoteResponseFromListRow converts a dbq.ListQuotesRow (the :many row type
// sqlc emits for queries/quotes.sql#ListQuotes) into a QuoteResponse.
func QuoteResponseFromListRow(q dbq.ListQuotesRow) QuoteResponse {
	return quoteResponseFromFields(q.ID, q.Title, q.Text, q.AuthorID, q.ImageID, q.CategoryID, q.CreatedAt, q.UpdatedAt)
}

// QuoteResponseFromGetRow converts a dbq.GetQuoteRow into a QuoteResponse.
func QuoteResponseFromGetRow(q dbq.GetQuoteRow) QuoteResponse {
	return quoteResponseFromFields(q.ID, q.Title, q.Text, q.AuthorID, q.ImageID, q.CategoryID, q.CreatedAt, q.UpdatedAt)
}

// QuoteResponseFromCreateRow converts a dbq.CreateQuoteRow into a QuoteResponse.
func QuoteResponseFromCreateRow(q dbq.CreateQuoteRow) QuoteResponse {
	return quoteResponseFromFields(q.ID, q.Title, q.Text, q.AuthorID, q.ImageID, q.CategoryID, q.CreatedAt, q.UpdatedAt)
}

// QuoteResponseFromUpdateRow converts a dbq.UpdateQuoteRow into a QuoteResponse.
func QuoteResponseFromUpdateRow(q dbq.UpdateQuoteRow) QuoteResponse {
	return quoteResponseFromFields(q.ID, q.Title, q.Text, q.AuthorID, q.ImageID, q.CategoryID, q.CreatedAt, q.UpdatedAt)
}

// QuoteResponsesFromListRows converts a slice of dbq.ListQuotesRow values into
// QuoteResponse values for paginated list responses.
func QuoteResponsesFromListRows(rows []dbq.ListQuotesRow) []QuoteResponse {
	out := make([]QuoteResponse, len(rows))
	for i, r := range rows {
		out[i] = QuoteResponseFromListRow(r)
	}
	return out
}
