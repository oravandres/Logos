package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// QuoteTagHandler provides HTTP handlers for managing tag associations on quotes.
type QuoteTagHandler struct {
	Q *dbq.Queries
}

// requireQuote verifies the quote exists and returns its UUID, writing a
// 400/404/500 response and returning false when the caller should stop.
func (h *QuoteTagHandler) requireQuote(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	quoteID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return uuid.Nil, false
	}

	_, err = h.Q.GetQuote(r.Context(), model.UUIDToPgtype(quoteID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "quote not found")
			return uuid.Nil, false
		}
		respondError(w, http.StatusInternalServerError, "failed to verify quote")
		return uuid.Nil, false
	}

	return quoteID, true
}

// ListTags returns all tags associated with a given quote.
func (h *QuoteTagHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	quoteID, ok := h.requireQuote(w, r)
	if !ok {
		return
	}

	tags, err := h.Q.ListTagsByQuote(r.Context(), model.UUIDToPgtype(quoteID))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list tags for quote")
		return
	}

	respondJSON(w, http.StatusOK, model.TagsFromDB(tags))
}

// addTagBody is the expected JSON body for adding a tag to a quote.
type addTagBody struct {
	TagID uuid.UUID `json:"tag_id"`
}

// AddTag associates a tag with a quote. Idempotent via ON CONFLICT DO NOTHING.
func (h *QuoteTagHandler) AddTag(w http.ResponseWriter, r *http.Request) {
	quoteID, ok := h.requireQuote(w, r)
	if !ok {
		return
	}

	var body addTagBody
	if err := decode(r, &body); err != nil {
		respondErrorDetail(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if body.TagID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "tag_id is required")
		return
	}

	if err := h.Q.AddTagToQuote(r.Context(), dbq.AddTagToQuoteParams{
		QuoteID: model.UUIDToPgtype(quoteID),
		TagID:   model.UUIDToPgtype(body.TagID),
	}); err != nil {
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced tag does not exist")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to add tag to quote")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveTag removes a tag association from a quote.
func (h *QuoteTagHandler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	quoteID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tagID, err := parseUUID(r, "tagID")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Q.RemoveTagFromQuote(r.Context(), dbq.RemoveTagFromQuoteParams{
		QuoteID: model.UUIDToPgtype(quoteID),
		TagID:   model.UUIDToPgtype(tagID),
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to remove tag from quote")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
