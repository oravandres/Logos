package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// QuoteHandler provides HTTP handlers for CRUD operations on quotes.
type QuoteHandler struct {
	Q *dbq.Queries
}

// List returns a paginated list of quotes, optionally filtered by author, category, or title.
func (h *QuoteHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	var filterAuthorID model.OptionalUUID
	if raw := r.URL.Query().Get("author_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid author_id")
			return
		}
		filterAuthorID = &id
	}

	var filterCategoryID model.OptionalUUID
	if raw := r.URL.Query().Get("category_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid category_id")
			return
		}
		filterCategoryID = &id
	}

	searchTitle := model.StringToPgtext(r.URL.Query().Get("title"))

	countParams := dbq.CountQuotesParams{
		FilterAuthorID:   model.OptionalUUIDToPgtype(filterAuthorID),
		FilterCategoryID: model.OptionalUUIDToPgtype(filterCategoryID),
		SearchTitle:      searchTitle,
	}

	quotes, err := h.Q.ListQuotes(r.Context(), dbq.ListQuotesParams{
		Limit:            limit,
		Offset:           offset,
		FilterAuthorID:   countParams.FilterAuthorID,
		FilterCategoryID: countParams.FilterCategoryID,
		SearchTitle:      countParams.SearchTitle,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list quotes")
		return
	}

	total, err := h.Q.CountQuotes(r.Context(), countParams)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count quotes")
		return
	}

	respondJSON(w, http.StatusOK, model.PaginatedResponse[model.QuoteResponse]{
		Items:  model.QuotesFromDB(quotes),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Get returns a single quote by its UUID.
func (h *QuoteHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	quote, err := h.Q.GetQuote(r.Context(), model.UUIDToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "quote not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get quote")
		return
	}

	respondJSON(w, http.StatusOK, model.QuoteFromDB(quote))
}

// Create validates the request body and inserts a new quote.
func (h *QuoteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateQuoteRequest
	if err := decode(r, &req); err != nil {
		respondErrorDetail(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if req.Title == "" {
		respondError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Text == "" {
		respondError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.AuthorID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "author_id is required")
		return
	}

	if req.CategoryID != nil {
		if err := validateCategoryType(r.Context(), h.Q, *req.CategoryID, "quote"); err != nil {
			writeCategoryTypeError(w, "quote", err)
			return
		}
	}

	quote, err := h.Q.CreateQuote(r.Context(), dbq.CreateQuoteParams{
		Title:      req.Title,
		Text:       req.Text,
		AuthorID:   model.UUIDToPgtype(req.AuthorID),
		ImageID:    model.OptionalUUIDToPgtype(req.ImageID),
		CategoryID: model.OptionalUUIDToPgtype(req.CategoryID),
	})
	if err != nil {
		if isCheckViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, `category type must be "quote"`)
			return
		}
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced author, image, or category does not exist")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create quote")
		return
	}

	respondJSON(w, http.StatusCreated, model.QuoteFromDB(quote))
}

// Update replaces the fields of an existing quote identified by UUID.
func (h *QuoteHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req model.UpdateQuoteRequest
	if err := decode(r, &req); err != nil {
		respondErrorDetail(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if req.Title == "" {
		respondError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Text == "" {
		respondError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.AuthorID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "author_id is required")
		return
	}

	if req.CategoryID != nil {
		if err := validateCategoryType(r.Context(), h.Q, *req.CategoryID, "quote"); err != nil {
			writeCategoryTypeError(w, "quote", err)
			return
		}
	}

	quote, err := h.Q.UpdateQuote(r.Context(), dbq.UpdateQuoteParams{
		ID:         model.UUIDToPgtype(id),
		Title:      req.Title,
		Text:       req.Text,
		AuthorID:   model.UUIDToPgtype(req.AuthorID),
		ImageID:    model.OptionalUUIDToPgtype(req.ImageID),
		CategoryID: model.OptionalUUIDToPgtype(req.CategoryID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "quote not found")
			return
		}
		if isCheckViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, `category type must be "quote"`)
			return
		}
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced author, image, or category does not exist")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update quote")
		return
	}

	respondJSON(w, http.StatusOK, model.QuoteFromDB(quote))
}

// Delete removes a quote by its UUID.
func (h *QuoteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Q.DeleteQuote(r.Context(), model.UUIDToPgtype(id)); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete quote")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
