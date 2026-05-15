package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/blobstore"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/handler"
)

// imageRouter is the same shape as the production /images sub-router so
// these tests exercise the handlers via real chi URL params (otherwise
// `chi.URLParam(r, "id")` returns "" and parseUUID rejects every request).
func imageRouter(h *handler.ImageHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/images", h.List)
	r.Post("/images", h.Create)
	r.Get("/images/{id}", h.Get)
	r.Put("/images/{id}", h.Update)
	r.Delete("/images/{id}", h.Delete)
	r.Post("/images/uploads", h.Upload)
	r.Get("/images/{id}/blob", h.Blob)
	return r
}

func TestImageCreate_Validation(t *testing.T) {
	t.Parallel()
	router := imageRouter(&handler.ImageHandler{Q: nilQ()})
	rec := postJSON(t, router, "/images", map[string]any{})
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "url is required")
}

func TestImageUpload_RequiresBlobstore(t *testing.T) {
	t.Parallel()
	// No `Blobs` configured: handler must short-circuit to 503 rather
	// than crashing on a nil-pointer deref or silently accepting the
	// upload and dropping the bytes on the floor.
	router := imageRouter(&handler.ImageHandler{Q: nilQ()})

	body, ct := multipartUpload(t, "image/png", smallPNG(t), "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/images/uploads", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorMsg(t, rec, "image uploads are not configured")
}

func TestImageUpload_RejectsWrongContentType(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, MaxUploadBytes: 1 << 20,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/images/uploads", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusUnsupportedMediaType)
	assertErrorMsg(t, rec, "content type must be multipart/form-data")
}

func TestImageUpload_RejectsMissingFileField(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, MaxUploadBytes: 1 << 20,
	})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	if err := mw.WriteField("alt_text", "no file please"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/images/uploads", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "file form field is required")
}

func TestImageUpload_RejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, MaxUploadBytes: 1 << 20,
	})

	// A blob that is decidedly not an image.
	body, ct := multipartUpload(t, "application/octet-stream", []byte("not an image at all"), "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/images/uploads", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusUnsupportedMediaType)
}

func TestImageUpload_RejectsOversizedBody(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, MaxUploadBytes: 1024, // tiny cap
	})

	// File part itself is 4 KiB — well past the 1 KiB MaxBytesReader cap
	// so the read fails before we even get to format detection.
	huge := bytes.Repeat([]byte("X"), 4096)
	body, ct := multipartUpload(t, "image/png", huge, "")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/images/uploads", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusRequestEntityTooLarge)
}

func TestImageUpload_RejectsInvalidCategoryID(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, MaxUploadBytes: 1 << 20,
	})

	body, ct := multipartUpload(t, "image/png", smallPNG(t), "not-a-uuid")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/images/uploads", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid category_id")
}

func TestImageBlob_RequiresBlobstore(t *testing.T) {
	t.Parallel()
	router := imageRouter(&handler.ImageHandler{Q: nilQ()})
	rec := getRequest(t, router, "/images/00000000-0000-0000-0000-000000000001/blob")
	assertStatus(t, rec, http.StatusServiceUnavailable)
}

func TestImageBlob_InvalidUUID(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{Q: nilQ(), Blobs: store})
	rec := getRequest(t, router, "/images/not-a-uuid/blob")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

// multipartUpload builds a multipart body with one file part (`file`), the
// given declared part Content-Type, the given body bytes, and an optional
// `category_id` form field. Returns the body and the matching outer
// Content-Type header value (boundary included).
func multipartUpload(t *testing.T, partType string, body []byte, categoryID string) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	hdr := make(map[string][]string)
	hdr["Content-Disposition"] = []string{`form-data; name="file"; filename="upload"`}
	hdr["Content-Type"] = []string{partType}
	w, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatalf("Write part: %v", err)
	}
	if categoryID != "" {
		if err := mw.WriteField("category_id", categoryID); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf, mw.FormDataContentType()
}

// smallPNG renders a 2x2 PNG image; the bytes pass http.DetectContentType
// as image/png and image.DecodeConfig returns valid dimensions.
func smallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func newStoreT(t *testing.T) blobstore.Store {
	t.Helper()
	s, err := blobstore.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	return s
}

// Compile-time guards: catch silent import / signature changes that
// would make the helpers below stop covering what they claim to cover.
var (
	_ = uuid.UUID{}
	_ = (*dbq.Queries)(nil)
	_ = (io.Reader)(nil)
	_ = json.Marshal
)
