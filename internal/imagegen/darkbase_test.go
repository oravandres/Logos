package imagegen_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oravandres/Logos/internal/imagegen"
)

// fakeJob mirrors DarkBase's JobInfo subset that matters to our client.
type fakeJob struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Seed         int64  `json:"seed"`
	ImageURL     string `json:"image_url"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// newAdapter builds an httptest server that mimics DarkBase: it accepts a
// generate POST, then returns the supplied script of poll responses on
// successive GET /api/v1/images/{id} hits, and serves a small PNG at the
// outputs path.
func newAdapter(t *testing.T, script []fakeJob, png []byte) (*httptest.Server, *adapterCounters) {
	t.Helper()
	c := &adapterCounters{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/images/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			http.Error(w, "want multipart", http.StatusUnsupportedMediaType)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.submitted.Add(1)
		c.lastPrompt.Store(r.FormValue("prompt"))
		c.lastSeed.Store(r.FormValue("seed"))
		c.lastWidth.Store(r.FormValue("width"))
		c.lastAuth.Store(r.Header.Get("Authorization"))

		_ = json.NewEncoder(w).Encode(fakeJob{
			ID: "job-1", Status: "queued", Width: 1024, Height: 1024,
		})
	})
	mux.HandleFunc("/api/v1/images/outputs/job-1.png", func(w http.ResponseWriter, r *http.Request) {
		c.fetched.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	// Status endpoint must come after the static routes so Go's
	// ServeMux picks it up via the catch-all pattern.
	mux.HandleFunc("/api/v1/images/", func(w http.ResponseWriter, r *http.Request) {
		// Strip the prefix to recover the job id.
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/images/")
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		idx := int(c.polled.Add(1)) - 1
		if idx >= len(script) {
			idx = len(script) - 1
		}
		_ = json.NewEncoder(w).Encode(script[idx])
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, c
}

type adapterCounters struct {
	submitted  atomic.Int32
	polled     atomic.Int32
	fetched    atomic.Int32
	lastPrompt atomic.Value
	lastSeed   atomic.Value
	lastWidth  atomic.Value
	lastAuth   atomic.Value
}

// tinyPNG is a minimal PNG signature + IHDR + IDAT + IEND. Image-bytes
// fidelity is not tested here — Generate just round-trips the body.
var tinyPNG = []byte("\x89PNG\r\n\x1a\nfake-bytes-for-test")

func TestDarkbase_Generate_HappyPath(t *testing.T) {
	t.Parallel()
	srv, c := newAdapter(t, []fakeJob{
		{ID: "job-1", Status: "queued"},
		{ID: "job-1", Status: "processing"},
		{
			ID: "job-1", Status: "completed",
			Width: 1024, Height: 768, Seed: 42,
			ImageURL: "/api/v1/images/outputs/job-1.png",
		},
	}, tinyPNG)

	g := &imagegen.DarkbaseGenerator{
		BaseURL:      srv.URL,
		PollInterval: 1 * time.Millisecond, // floored back to 1s by impl; we override below
	}
	// Override the floor for tests by reaching through the value directly.
	// (The package's 1s floor is intentional in production; tests use a
	// scaled-down poll via the public `PollInterval` field. The floor
	// rounds 1ms up to 1s, which is too slow for tests — so we set a
	// per-call deadline that still permits ~3 polls at 1s each.)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := g.Generate(ctx, imagegen.Request{
		Prompt: "a tiny cat", Width: 1024, Height: 768, Seed: 42,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", res.ContentType)
	}
	if string(res.Bytes) != string(tinyPNG) {
		t.Errorf("Bytes mismatch (len=%d want=%d)", len(res.Bytes), len(tinyPNG))
	}
	if res.Width != 1024 || res.Height != 768 {
		t.Errorf("dims = %dx%d, want 1024x768", res.Width, res.Height)
	}
	if res.Seed != 42 {
		t.Errorf("Seed = %d, want 42", res.Seed)
	}
	if got := c.lastPrompt.Load().(string); got != "a tiny cat" {
		t.Errorf("prompt forwarded = %q, want %q", got, "a tiny cat")
	}
	if got := c.lastSeed.Load().(string); got != "42" {
		t.Errorf("seed forwarded = %q, want %q", got, "42")
	}
	if c.submitted.Load() != 1 {
		t.Errorf("submitted = %d, want 1", c.submitted.Load())
	}
	if c.fetched.Load() != 1 {
		t.Errorf("fetched = %d, want 1", c.fetched.Load())
	}
}

func TestDarkbase_Generate_RejectsMissingPrompt(t *testing.T) {
	t.Parallel()
	g := &imagegen.DarkbaseGenerator{BaseURL: "http://example.invalid"}
	_, err := g.Generate(context.Background(), imagegen.Request{Prompt: "  "})
	if !errors.Is(err, imagegen.ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestDarkbase_Generate_RejectsEmptyBaseURL(t *testing.T) {
	t.Parallel()
	g := &imagegen.DarkbaseGenerator{BaseURL: ""}
	_, err := g.Generate(context.Background(), imagegen.Request{Prompt: "x"})
	if !errors.Is(err, imagegen.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestDarkbase_Generate_Submit4xxIsInvalidRequest(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"DIMENSION_LIMIT","message":"too big"}}`))
	}))
	defer srv.Close()

	g := &imagegen.DarkbaseGenerator{BaseURL: srv.URL}
	_, err := g.Generate(context.Background(), imagegen.Request{Prompt: "x"})
	if !errors.Is(err, imagegen.ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
	if !strings.Contains(err.Error(), "too big") {
		t.Errorf("message not propagated: %v", err)
	}
}

func TestDarkbase_Generate_Submit5xxIsUnavailable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := &imagegen.DarkbaseGenerator{BaseURL: srv.URL}
	_, err := g.Generate(context.Background(), imagegen.Request{Prompt: "x"})
	if !errors.Is(err, imagegen.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestDarkbase_Generate_JobErrorIsJobFailed(t *testing.T) {
	t.Parallel()
	srv, _ := newAdapter(t, []fakeJob{
		{ID: "job-1", Status: "error", ErrorMessage: "OOM on transformer"},
	}, tinyPNG)

	g := &imagegen.DarkbaseGenerator{BaseURL: srv.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := g.Generate(ctx, imagegen.Request{Prompt: "x"})
	if !errors.Is(err, imagegen.ErrJobFailed) {
		t.Fatalf("err = %v, want ErrJobFailed", err)
	}
	if !strings.Contains(err.Error(), "OOM on transformer") {
		t.Errorf("message not propagated: %v", err)
	}
}

func TestDarkbase_Generate_HonorsContextCancellation(t *testing.T) {
	t.Parallel()
	// Adapter that always returns "processing" — Generate would loop
	// forever. We expect ctx.Done to break us out.
	srv, _ := newAdapter(t, []fakeJob{
		{ID: "job-1", Status: "processing"},
	}, tinyPNG)

	g := &imagegen.DarkbaseGenerator{BaseURL: srv.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	_, err := g.Generate(ctx, imagegen.Request{Prompt: "x"})
	if err == nil {
		t.Fatal("Generate did not return an error on cancelled context")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		// The first poll might race against the deadline and return
		// ErrUnavailable from the in-flight client. Both are acceptable
		// outcomes — the bug we're guarding against is "Generate hangs
		// past the deadline", so just assert _some_ failure within the
		// timeout window. Calling t.Logf to record what we observed.
		t.Logf("non-deadline error (acceptable): %v", err)
	}
}

func TestDarkbase_Generate_SendsBearerWhenConfigured(t *testing.T) {
	t.Parallel()
	srv, c := newAdapter(t, []fakeJob{
		{
			ID: "job-1", Status: "completed",
			ImageURL: "/api/v1/images/outputs/job-1.png",
		},
	}, tinyPNG)

	g := &imagegen.DarkbaseGenerator{BaseURL: srv.URL, AuthToken: "secret-shh"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := g.Generate(ctx, imagegen.Request{Prompt: "x"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got, _ := c.lastAuth.Load().(string); got != "Bearer secret-shh" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer secret-shh")
	}
}

func TestDarkbase_Generate_EmptyOutputBodyIsUnavailable(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/images/generate", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fakeJob{ID: "job-1", Status: "queued"})
	})
	mux.HandleFunc("/api/v1/images/outputs/job-1.png", func(w http.ResponseWriter, _ *http.Request) {
		// Empty body — generator should treat this as unavailable rather
		// than returning a zero-length Result that would break downstream
		// content-type detection.
	})
	mux.HandleFunc("/api/v1/images/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fakeJob{
			ID: "job-1", Status: "completed",
			ImageURL: "/api/v1/images/outputs/job-1.png",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	g := &imagegen.DarkbaseGenerator{BaseURL: srv.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := g.Generate(ctx, imagegen.Request{Prompt: "x"})
	if !errors.Is(err, imagegen.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestDarkbase_Generate_AbsoluteImageURLIsHonored(t *testing.T) {
	t.Parallel()
	// Some adapter deployments may begin returning an absolute image_url.
	// The client should accept that without re-prefixing BaseURL.
	bytesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/job-1.png") {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		_, _ = w.Write(tinyPNG)
	}))
	defer bytesSrv.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/images/generate", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fakeJob{ID: "job-1", Status: "queued"})
	})
	mux.HandleFunc("/api/v1/images/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fakeJob{
			ID: "job-1", Status: "completed",
			ImageURL: bytesSrv.URL + "/job-1.png",
		})
	})
	mainSrv := httptest.NewServer(mux)
	defer mainSrv.Close()

	g := &imagegen.DarkbaseGenerator{BaseURL: mainSrv.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := g.Generate(ctx, imagegen.Request{Prompt: "x"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(res.Bytes) != string(tinyPNG) {
		t.Errorf("Bytes diverge — absolute image_url not followed?")
	}
}

// Compile-time fence: keep the compiler honest about exported symbols.
var (
	_ = fmt.Sprintf
	_ = io.EOF
)
