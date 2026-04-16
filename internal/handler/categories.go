package handler

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// CategoryHandler provides HTTP handlers for CRUD operations on categories.
type CategoryHandler struct {
	Q *dbq.Queries
}

// List returns a paginated list of categories, optionally filtered by type.
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	filterType := model.StringToPgtext(r.URL.Query().Get("type"))

	cats, err := h.Q.ListCategories(r.Context(), dbq.ListCategoriesParams{
		Limit:      limit,
		Offset:     offset,
		FilterType: filterType,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list categories")
		return
	}

	total, err := h.Q.CountCategories(r.Context(), filterType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count categories")
		return
	}

	respondJSON(w, http.StatusOK, model.PaginatedResponse[model.CategoryResponse]{
		Items:  model.CategoriesFromDB(cats),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Get returns a single category by its UUID.
func (h *CategoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	cat, err := h.Q.GetCategory(r.Context(), model.UUIDToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "category not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get category")
		return
	}

	respondJSON(w, http.StatusOK, model.CategoryFromDB(cat))
}

// Create validates the request body and inserts a new category.
func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateCategoryRequest
	if err := decode(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	if req.Name == "" || req.Type == "" {
		respondError(w, http.StatusBadRequest, "name and type are required")
		return
	}

	validTypes := map[string]bool{"image": true, "quote": true, "author": true}
	if !validTypes[req.Type] {
		respondError(w, http.StatusBadRequest, "type must be one of: image, quote, author")
		return
	}

	cat, err := h.Q.CreateCategory(r.Context(), dbq.CreateCategoryParams{
		Name: req.Name,
		Type: req.Type,
	})
	if err != nil {
		if isUniqueViolation(err) {
			respondError(w, http.StatusConflict, "category with this name and type already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create category")
		return
	}

	respondJSON(w, http.StatusCreated, model.CategoryFromDB(cat))
}

// Update replaces the fields of an existing category identified by UUID.
func (h *CategoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req model.UpdateCategoryRequest
	if err := decode(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	if req.Name == "" || req.Type == "" {
		respondError(w, http.StatusBadRequest, "name and type are required")
		return
	}

	validTypes := map[string]bool{"image": true, "quote": true, "author": true}
	if !validTypes[req.Type] {
		respondError(w, http.StatusBadRequest, "type must be one of: image, quote, author")
		return
	}

	cat, err := h.Q.UpdateCategory(r.Context(), dbq.UpdateCategoryParams{
		ID:   model.UUIDToPgtype(id),
		Name: req.Name,
		Type: req.Type,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "category not found")
			return
		}
		if isUniqueViolation(err) {
			respondError(w, http.StatusConflict, "category with this name and type already exists")
			return
		}
		if isCheckViolation(err) {
			respondError(w, http.StatusConflict, "cannot change category type while it is referenced by other entities")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update category")
		return
	}

	respondJSON(w, http.StatusOK, model.CategoryFromDB(cat))
}

// Delete removes a category by its UUID.
func (h *CategoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Q.DeleteCategory(r.Context(), model.UUIDToPgtype(id)); err != nil {
		respondErrorDetail(w, http.StatusInternalServerError, "failed to delete category", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
