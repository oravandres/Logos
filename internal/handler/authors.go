package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// AuthorHandler provides HTTP handlers for CRUD operations on authors.
type AuthorHandler struct {
	Q *dbq.Queries
}

// List returns a paginated list of authors, optionally filtered by category or name.
func (h *AuthorHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	var filterCategoryID model.OptionalUUID
	if raw := r.URL.Query().Get("category_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid category_id")
			return
		}
		filterCategoryID = &id
	}

	searchName := model.StringToPgtext(r.URL.Query().Get("name"))

	filterParams := dbq.CountAuthorsParams{
		FilterCategoryID: model.OptionalUUIDToPgtype(filterCategoryID),
		SearchName:       searchName,
	}

	authors, err := h.Q.ListAuthors(r.Context(), dbq.ListAuthorsParams{
		Limit:            limit,
		Offset:           offset,
		FilterCategoryID: filterParams.FilterCategoryID,
		SearchName:       filterParams.SearchName,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list authors")
		return
	}

	total, err := h.Q.CountAuthors(r.Context(), filterParams)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count authors")
		return
	}

	respondJSON(w, http.StatusOK, model.PaginatedResponse[model.AuthorResponse]{
		Items:  model.AuthorsFromDB(authors),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Get returns a single author by its UUID.
func (h *AuthorHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	author, err := h.Q.GetAuthor(r.Context(), model.UUIDToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "author not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get author")
		return
	}

	respondJSON(w, http.StatusOK, model.AuthorFromDB(author))
}

// Create validates the request body and inserts a new author.
func (h *AuthorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAuthorRequest
	if err := decode(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	bornDate, err := model.OptionalStringToPgdate(req.BornDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "born_date must be YYYY-MM-DD")
		return
	}

	diedDate, err := model.OptionalStringToPgdate(req.DiedDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "died_date must be YYYY-MM-DD")
		return
	}

	if err := validateDateChronology(bornDate, diedDate); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.CategoryID != nil {
		if err := validateCategoryType(r.Context(), h.Q, *req.CategoryID, "author"); err != nil {
			writeCategoryTypeError(w, "author", err)
			return
		}
	}

	author, err := h.Q.CreateAuthor(r.Context(), dbq.CreateAuthorParams{
		Name:       req.Name,
		Bio:        model.OptionalStringToPgtext(req.Bio),
		BornDate:   bornDate,
		DiedDate:   diedDate,
		ImageID:    model.OptionalUUIDToPgtype(req.ImageID),
		CategoryID: model.OptionalUUIDToPgtype(req.CategoryID),
	})
	if err != nil {
		if isCheckViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, `category type must be "author"`)
			return
		}
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced image or category does not exist")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create author")
		return
	}

	respondJSON(w, http.StatusCreated, model.AuthorFromDB(author))
}

// Update replaces the fields of an existing author identified by UUID.
func (h *AuthorHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req model.UpdateAuthorRequest
	if err := decode(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	bornDate, err := model.OptionalStringToPgdate(req.BornDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "born_date must be YYYY-MM-DD")
		return
	}

	diedDate, err := model.OptionalStringToPgdate(req.DiedDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "died_date must be YYYY-MM-DD")
		return
	}

	if err := validateDateChronology(bornDate, diedDate); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.CategoryID != nil {
		if err := validateCategoryType(r.Context(), h.Q, *req.CategoryID, "author"); err != nil {
			writeCategoryTypeError(w, "author", err)
			return
		}
	}

	author, err := h.Q.UpdateAuthor(r.Context(), dbq.UpdateAuthorParams{
		ID:         model.UUIDToPgtype(id),
		Name:       req.Name,
		Bio:        model.OptionalStringToPgtext(req.Bio),
		BornDate:   bornDate,
		DiedDate:   diedDate,
		ImageID:    model.OptionalUUIDToPgtype(req.ImageID),
		CategoryID: model.OptionalUUIDToPgtype(req.CategoryID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "author not found")
			return
		}
		if isCheckViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, `category type must be "author"`)
			return
		}
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced image or category does not exist")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update author")
		return
	}

	respondJSON(w, http.StatusOK, model.AuthorFromDB(author))
}

// Delete removes an author by its UUID. Fails if quotes reference this author.
func (h *AuthorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := h.Q.DeleteAuthor(r.Context(), model.UUIDToPgtype(id)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "author not found")
			return
		}
		if isFKViolation(err) {
			respondError(w, http.StatusConflict, "author has associated quotes and cannot be deleted")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete author")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// validateDateChronology returns an error if both dates are present and died is before born.
func validateDateChronology(born, died pgtype.Date) error {
	if born.Valid && died.Valid && died.Time.Before(born.Time) {
		return errors.New("died_date must not be earlier than born_date")
	}
	return nil
}
