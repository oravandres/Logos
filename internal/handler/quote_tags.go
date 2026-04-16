package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// TxBeginner abstracts transaction creation so handlers can be tested
// without a live pgxpool.Pool.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// QuoteTagHandler provides HTTP handlers for managing tag associations on quotes.
type QuoteTagHandler struct {
	Q    *dbq.Queries
	Pool TxBeginner
}

// verifyQuoteInTx locks the quote row (FOR KEY SHARE) within the given
// transaction, returning false and writing an HTTP error when the quote
// does not exist or the lookup fails.
func verifyQuoteInTx(ctx context.Context, qtx *dbq.Queries, w http.ResponseWriter, quoteID uuid.UUID) bool {
	_, err := qtx.GetQuoteForKeyShare(ctx, model.UUIDToPgtype(quoteID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "quote not found")
			return false
		}
		respondError(w, http.StatusInternalServerError, "failed to verify quote")
		return false
	}
	return true
}

// ListTags returns all tags associated with a given quote.
func (h *QuoteTagHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	quoteID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()

	if _, err := h.Q.GetQuote(ctx, model.UUIDToPgtype(quoteID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "quote not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to verify quote")
		return
	}

	tags, err := h.Q.ListTagsByQuote(ctx, model.UUIDToPgtype(quoteID))
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
	quoteID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
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

	ctx := r.Context()

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := h.Q.WithTx(tx)

	if !verifyQuoteInTx(ctx, qtx, w, quoteID) {
		return
	}

	if err := qtx.AddTagToQuote(ctx, dbq.AddTagToQuoteParams{
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

	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit transaction")
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
