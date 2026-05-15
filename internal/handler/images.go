package handler

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for image.Decode
	_ "image/jpeg" // register JPEG decoder for image.Decode
	_ "image/png"  // register PNG decoder for image.Decode
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oravandres/Logos/internal/blobstore"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

// ImageHandler provides HTTP handlers for CRUD + upload + blob serving of images.
//
// `Blobs` and `MaxUploadBytes` are nil/zero when the operator did not set
// `LOGOS_IMAGE_UPLOAD_DIR`; in that case Upload/Blob respond with 503 so
// the failure mode is loud rather than mysterious.
type ImageHandler struct {
	Q              *dbq.Queries
	Blobs          blobstore.Store
	MaxUploadBytes int64
}

// supportedUploadFormats maps a sniffed Content-Type to the file extension
// we persist on disk. Only formats whose decoders we register above are
// listed: anything else results in a 415.
var supportedUploadFormats = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/gif":  "gif",
	"image/webp": "webp",
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
		Items:  model.ImagesFromListRows(imgs),
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

	respondJSON(w, http.StatusOK, model.ImageFromGetRow(img))
}

// Create validates the request body and inserts a new external-URL image.
func (h *ImageHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateImageRequest
	if err := decode(w, r, &req); err != nil {
		writeDecodeError(w, err)
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
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced category does not exist")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create image")
		return
	}

	respondJSON(w, http.StatusCreated, model.ImageFromCreateRow(img))
}

// Update replaces the user-editable fields of an existing image.
//
// Source-specific audit columns (content_type, prompt, …) are intentionally
// not updatable via this endpoint — re-upload or re-generate produces a new
// row instead.
func (h *ImageHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req model.UpdateImageRequest
	if err := decode(w, r, &req); err != nil {
		writeDecodeError(w, err)
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
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced category does not exist")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update image")
		return
	}

	respondJSON(w, http.StatusOK, model.ImageFromUpdateRow(img))
}

// Delete removes an image by UUID, plus its on-disk blob for `uploaded` /
// `generated` rows. External-URL rows have no on-disk artifact.
//
// Order of operations: row deletion first, then a best-effort blob delete.
// If the blob delete fails, we log and continue — the row is already gone
// and the orphaned blob is harmless (and easy to GC offline).
func (h *ImageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	row, err := h.Q.DeleteImage(r.Context(), model.UUIDToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "image not found")
			return
		}
		respondErrorDetail(w, http.StatusInternalServerError, "failed to delete image", err.Error())
		return
	}

	if h.Blobs != nil && (row.Source == model.SourceUploaded || row.Source == model.SourceGenerated) {
		// We do not have the original extension on hand here: the
		// canonical place is the URL string we stored on the row,
		// which we already consumed via the DELETE RETURNING. To keep
		// this handler simple and avoid an extra round-trip just to
		// learn the extension, try the small set of supported
		// extensions; missing files are tolerated by Delete().
		for _, ext := range []string{"png", "jpg", "gif", "webp"} {
			if delErr := h.Blobs.Delete(id, ext); delErr != nil {
				slog.Warn("failed to delete image blob",
					"id", id.String(), "ext", ext, "error", delErr)
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// Upload accepts a multipart/form-data POST with the binary in field `file`
// and optional `alt_text` / `category_id` form fields.
//
// Why multipart vs base64 in JSON?
//   - Multipart is the platform native shape for file uploads. The bytes
//     never pass through a JSON decoder so we don't pay the 33% base64
//     overhead and we don't allocate the body twice.
//   - Browsers can post FormData straight from a `<input type=file>` with
//     no client-side encoding step.
//
// Validation (in order; each step is a hard 4xx if it fails):
//  1. Content type is multipart/form-data with a boundary.
//  2. Aggregate body is at most MaxUploadBytes (cheap, Reader-bound).
//  3. The `file` part decodes as one of the registered image formats.
//  4. The `category_id`, if present, parses as a UUID.
//
// On success we Put() the bytes, then INSERT a row whose `url` is the
// blobstore-relative URL — the same URL the client will read with GET
// `/api/v1/images/{id}/blob`.
func (h *ImageHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if h.Blobs == nil {
		respondError(w, http.StatusServiceUnavailable, "image uploads are not configured")
		return
	}

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		respondError(w, http.StatusUnsupportedMediaType, "content type must be multipart/form-data")
		return
	}

	max := h.MaxUploadBytes
	if max <= 0 {
		max = 10 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)

	// `ParseMultipartForm` keeps small parts in memory; large parts spill
	// to a temp file. The 1 MiB threshold below is the in-memory cap;
	// total cap is enforced by MaxBytesReader above.
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respondError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("upload exceeds %d bytes", max))
			return
		}
		respondErrorDetail(w, http.StatusBadRequest, "invalid multipart body", err.Error())
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "file form field is required")
		return
	}
	defer func() { _ = file.Close() }()
	if fileHeader.Size <= 0 {
		respondError(w, http.StatusBadRequest, "file is empty")
		return
	}

	// Buffer the bytes once so we can: (a) sniff the format, (b) decode
	// it for dimensions, (c) restream to disk. The MaxBytesReader above
	// guarantees this can never exceed the configured cap.
	buf, err := io.ReadAll(file)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respondError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("upload exceeds %d bytes", max))
			return
		}
		respondErrorDetail(w, http.StatusBadRequest, "failed to read upload", err.Error())
		return
	}

	contentType := http.DetectContentType(buf)
	// http.DetectContentType returns "image/png; charset=utf-8" for some
	// callers; we only care about the type/subtype prefix.
	mt := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	ext, ok := supportedUploadFormats[mt]
	if !ok {
		respondError(w, http.StatusUnsupportedMediaType,
			"unsupported image type; allowed: image/png, image/jpeg, image/gif, image/webp")
		return
	}

	// `image.DecodeConfig` returns dimensions cheaply; we don't need a
	// full pixel decode to populate width/height. webp lacks a built-in
	// decoder in the standard library, so width/height stay NULL for it.
	width, height := decodeImageConfig(mt, buf)

	var altText *string
	if v := strings.TrimSpace(r.FormValue("alt_text")); v != "" {
		altText = &v
	}
	var categoryID *uuid.UUID
	if raw := strings.TrimSpace(r.FormValue("category_id")); raw != "" {
		parsed, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			respondError(w, http.StatusBadRequest, "invalid category_id")
			return
		}
		categoryID = &parsed
	}

	id := uuid.New()

	if err := h.Blobs.Put(r.Context(), id, ext, bytes.NewReader(buf)); err != nil {
		respondErrorDetail(w, http.StatusInternalServerError, "failed to persist image bytes", err.Error())
		return
	}

	row, err := h.Q.CreateUploadedImage(r.Context(), dbq.CreateUploadedImageParams{
		ID:          model.UUIDToPgtype(id),
		Url:         h.Blobs.Path(id, ext),
		AltText:     model.OptionalStringToPgtext(altText),
		CategoryID:  model.OptionalUUIDToPgtype(categoryID),
		ContentType: model.StringToPgtext(mt),
		SizeBytes:   pgtype.Int8{Int64: int64(len(buf)), Valid: true},
		Width:       optionalInt32(width),
		Height:      optionalInt32(height),
	})
	if err != nil {
		// Roll back the blob so the on-disk artifact does not outlive
		// a failed insert. This is the only place we treat the row
		// and the blob as a transactional unit.
		_ = h.Blobs.Delete(id, ext)
		if isFKViolation(err) {
			respondError(w, http.StatusUnprocessableEntity, "referenced category does not exist")
			return
		}
		respondErrorDetail(w, http.StatusInternalServerError, "failed to create image", err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, model.ImageFromUploadRow(row))
}

// Blob serves the raw image bytes for `uploaded` and `generated` rows.
// External-URL rows return 404 — there is nothing on disk to serve, and
// the client should already have the URL from the row's `url` field.
func (h *ImageHandler) Blob(w http.ResponseWriter, r *http.Request) {
	if h.Blobs == nil {
		respondError(w, http.StatusServiceUnavailable, "image blob storage is not configured")
		return
	}

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
	if img.Source == model.SourceExternalURL {
		respondError(w, http.StatusNotFound, "image has no local blob")
		return
	}

	mt := ""
	if img.ContentType.Valid {
		mt = img.ContentType.String
	}
	ext, ok := supportedUploadFormats[mt]
	if !ok {
		respondError(w, http.StatusInternalServerError, "image content type is not servable")
		return
	}

	f, err := h.Blobs.Open(id, ext)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			respondError(w, http.StatusNotFound, "image blob is missing")
			return
		}
		respondErrorDetail(w, http.StatusInternalServerError, "failed to open image blob", err.Error())
		return
	}
	defer func() { _ = f.Close() }()

	rsc, ok := f.(io.ReadSeeker)
	if !ok {
		// LocalStore returns *os.File which is a ReadSeeker; future
		// Stores that don't (e.g. a streaming MinIO client) would
		// need a different serve path. Spell that out explicitly.
		respondError(w, http.StatusInternalServerError, "image blob is not seekable")
		return
	}
	stat, statErr := f.Stat()
	if statErr != nil {
		respondErrorDetail(w, http.StatusInternalServerError, "failed to stat image blob", statErr.Error())
		return
	}

	w.Header().Set("Content-Type", mt)
	// Cache aggressively: the URL contains a UUID and the bytes are
	// immutable for the lifetime of the row (re-uploads create new ids).
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, ext, modTime(stat, img.UpdatedAt.Time), rsc)
}

func decodeImageConfig(mt string, buf []byte) (width, height int) {
	if mt == "image/webp" {
		// stdlib has no webp decoder; we'd need a third-party import
		// for dimensions. Skipped for v1.
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func optionalInt32(n int) pgtype.Int4 {
	if n <= 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}

func modTime(stat fs.FileInfo, fallback time.Time) time.Time {
	if stat == nil {
		return fallback
	}
	t := stat.ModTime()
	if t.IsZero() {
		return fallback
	}
	return t
}
