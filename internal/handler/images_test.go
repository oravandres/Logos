package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/blobstore"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/handler"
	"github.com/oravandres/Logos/internal/imagegen"
)

// fakeGenerator is a controllable Generator for handler tests.
type fakeGenerator struct {
	res  imagegen.Result
	err  error
	last imagegen.Request
}

func (g *fakeGenerator) Generate(_ context.Context, req imagegen.Request) (imagegen.Result, error) {
	g.last = req
	return g.res, g.err
}

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
	r.Post("/images:generate", h.Generate)
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

// ----------------------------------------------------------------------
// Generate handler tests
// ----------------------------------------------------------------------

func TestImageGenerate_RequiresGenerator(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{Q: nilQ(), Blobs: store})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "x"})
	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorMsg(t, rec, "image generation is not configured")
}

func TestImageGenerate_RequiresBlobstore(t *testing.T) {
	t.Parallel()
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), ImageGen: &fakeGenerator{},
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "x"})
	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorMsg(t, rec, "image storage is not configured")
}

func TestImageGenerate_RejectsMissingPrompt(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: &fakeGenerator{},
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "  "})
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "prompt is required")
}

func TestImageGenerate_RejectsExcessiveDimensions(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: &fakeGenerator{},
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{
		"prompt": "x", "width": 9999, "height": 9999,
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "width/height must be in [0, 4096]")
}

func TestImageGenerate_BackendInvalidRequestMapsTo400(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	gen := &fakeGenerator{err: imagegen.ErrInvalidRequest}
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: gen,
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "x"})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestImageGenerate_BackendUnavailableMapsTo503(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	gen := &fakeGenerator{err: imagegen.ErrUnavailable}
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: gen,
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "x"})
	assertStatus(t, rec, http.StatusServiceUnavailable)
}

func TestImageGenerate_BackendJobFailedMapsTo502(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	gen := &fakeGenerator{err: imagegen.ErrJobFailed}
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: gen,
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "x"})
	assertStatus(t, rec, http.StatusBadGateway)
}

func TestImageGenerate_DeadlineExceededMapsTo504(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	gen := &fakeGenerator{err: context.DeadlineExceeded}
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: gen, GenTimeout: 1 * time.Millisecond,
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "x"})
	assertStatus(t, rec, http.StatusGatewayTimeout)
}

func TestImageGenerate_RejectsUnsupportedContentType(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	gen := &fakeGenerator{
		res: imagegen.Result{
			Bytes:       []byte("anything"),
			ContentType: "image/tiff",
		},
	}
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: gen,
	})

	rec := postJSON(t, router, "/images:generate", map[string]any{"prompt": "x"})
	assertStatus(t, rec, http.StatusBadGateway)
}

func TestImageGenerate_ForwardsRequestFields(t *testing.T) {
	t.Parallel()
	store := newStoreT(t)
	// Returning ErrUnavailable lets the test stop before the DB layer
	// (which the nil stub can't satisfy) but still exercises the field
	// forwarding path that runs *before* the Generator returns.
	gen := &fakeGenerator{err: imagegen.ErrUnavailable}
	router := imageRouter(&handler.ImageHandler{
		Q: nilQ(), Blobs: store, ImageGen: gen,
	})

	body := map[string]any{
		"prompt":    "an autumn forest",
		"model":     "flux2",
		"width":     1024,
		"height":    768,
		"seed":      9999,
		"steps":     28,
		"cfg_scale": 4.0,
	}
	rec := postJSON(t, router, "/images:generate", body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d want 503; body=%s", rec.Code, rec.Body.String())
	}

	if gen.last.Prompt != "an autumn forest" {
		t.Errorf("Prompt = %q", gen.last.Prompt)
	}
	if gen.last.Model != "flux2" {
		t.Errorf("Model = %q", gen.last.Model)
	}
	if gen.last.Width != 1024 || gen.last.Height != 768 {
		t.Errorf("dims = %dx%d", gen.last.Width, gen.last.Height)
	}
	if gen.last.Seed != 9999 {
		t.Errorf("Seed = %d", gen.last.Seed)
	}
	if gen.last.Steps != 28 {
		t.Errorf("Steps = %d", gen.last.Steps)
	}
	if gen.last.CFGScale != 4.0 {
		t.Errorf("CFGScale = %v", gen.last.CFGScale)
	}
}

// Compile-time guards: catch silent import / signature changes that
// would make the helpers below stop covering what they claim to cover.
var (
	_ = uuid.UUID{}
	_ = (*dbq.Queries)(nil)
	_ = (io.Reader)(nil)
	_ = json.Marshal
	_ = errors.Is
)
