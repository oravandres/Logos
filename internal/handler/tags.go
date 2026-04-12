package handler

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// TagHandler provides HTTP handlers for CRUD operations on tags.
type TagHandler struct {
	Q *dbq.Queries
}

// List returns a paginated list of tags sorted alphabetically.
func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	tags, err := h.Q.ListTags(r.Context(), dbq.ListTagsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}

	total, err := h.Q.CountTags(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count tags")
		return
	}

	respondJSON(w, http.StatusOK, model.PaginatedResponse[model.TagResponse]{
		Items:  model.TagsFromDB(tags),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Get returns a single tag by its UUID.
func (h *TagHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.Q.GetTag(r.Context(), model.UUIDToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "tag not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get tag")
		return
	}

	respondJSON(w, http.StatusOK, model.TagFromDB(tag))
}

// Create validates the request body and inserts a new tag.
func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTagRequest
	if err := decode(r, &req); err != nil {
		respondErrorDetail(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 100 {
		respondError(w, http.StatusBadRequest, "name must be 100 characters or fewer")
		return
	}

	tag, err := h.Q.CreateTag(r.Context(), req.Name)
	if err != nil {
		if isUniqueViolation(err) {
			respondError(w, http.StatusConflict, "tag with this name already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create tag")
		return
	}

	respondJSON(w, http.StatusCreated, model.TagFromDB(tag))
}

// Delete removes a tag by its UUID. Associated quote_tags rows cascade.
func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Q.DeleteTag(r.Context(), model.UUIDToPgtype(id)); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete tag")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
