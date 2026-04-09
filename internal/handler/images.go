package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// ImageHandler provides HTTP handlers for CRUD operations on images.
type ImageHandler struct {
	Q *dbq.Queries
}

// List returns a paginated list of images, optionally filtered by category.
func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
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

	params := dbq.ListImagesParams{
		Limit:            limit,
		Offset:           offset,
		FilterCategoryID: model.OptionalUUIDToPgtype(filterCategoryID),
	}

	imgs, err := h.Q.ListImages(r.Context(), params)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list images")
		return
	}

	total, err := h.Q.CountImages(r.Context(), model.OptionalUUIDToPgtype(filterCategoryID))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count images")
		return
	}

	respondJSON(w, http.StatusOK, model.PaginatedResponse[model.ImageResponse]{
		Items:  model.ImagesFromDB(imgs),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Get returns a single image by its UUID.
func (h *ImageHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	img, err := h.Q.GetImage(r.Context(), model.UUIDToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "image not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get image")
		return
	}

	respondJSON(w, http.StatusOK, model.ImageFromDB(img))
}

// Create validates the request body and inserts a new image.
func (h *ImageHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateImageRequest
	if err := decode(r, &req); err != nil {
		respondErrorDetail(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if req.URL == "" {
		respondError(w, http.StatusBadRequest, "url is required")
		return
	}

	img, err := h.Q.CreateImage(r.Context(), dbq.CreateImageParams{
		Url:        req.URL,
		AltText:    model.OptionalStringToPgtext(req.AltText),
		CategoryID: model.OptionalUUIDToPgtype(req.CategoryID),
	})
	if err != nil {
		respondErrorDetail(w, http.StatusInternalServerError, "failed to create image", err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, model.ImageFromDB(img))
}

// Update replaces the fields of an existing image identified by UUID.
func (h *ImageHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req model.UpdateImageRequest
	if err := decode(r, &req); err != nil {
		respondErrorDetail(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if req.URL == "" {
		respondError(w, http.StatusBadRequest, "url is required")
		return
	}

	img, err := h.Q.UpdateImage(r.Context(), dbq.UpdateImageParams{
		ID:         model.UUIDToPgtype(id),
		Url:        req.URL,
		AltText:    model.OptionalStringToPgtext(req.AltText),
		CategoryID: model.OptionalUUIDToPgtype(req.CategoryID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "image not found")
			return
		}
		respondErrorDetail(w, http.StatusInternalServerError, "failed to update image", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, model.ImageFromDB(img))
}

// Delete removes an image by its UUID.
func (h *ImageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Q.DeleteImage(r.Context(), model.UUIDToPgtype(id)); err != nil {
		respondErrorDetail(w, http.StatusInternalServerError, "failed to delete image", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
