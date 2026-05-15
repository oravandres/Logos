package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DarkbaseGenerator talks to the DarkBase image-adapter
// (`services/image-adapter/main.py`) over its FastAPI surface:
//
//   - POST /api/v1/images/generate (multipart/form-data)
//     → 200 JobInfo {id, status, …} (status starts as "queued")
//   - GET  /api/v1/images/{job_id}
//     → 200 JobInfo (status: queued | processing | completed | error)
//   - GET  /api/v1/images/outputs/{filename}
//     → 200 image/png bytes
//
// Workflow:
//
//  1. Submit the job (POST /generate). DarkBase queues the job and
//     returns immediately with the queued JobInfo.
//  2. Poll the job until it transitions to "completed" or "error".
//     Poll interval is configurable (default 1.5s — DarkBase jobs run on
//     the order of seconds-to-tens-of-seconds and a tighter loop just
//     burns CPU on both sides).
//  3. Fetch the rendered PNG from `image_url` (always relative to the
//     adapter base URL) and return it.
//
// Cancellation: every HTTP call is built with the caller-supplied context
// so client-disconnect / server-deadline propagates all the way to the
// adapter. We do NOT issue a "cancel this job" request to DarkBase on
// cancellation — its API has no such endpoint, and the worst case is a
// queued render that completes and lands in OUTPUT_DIR with no caller
// (cleaned up by the existing retention policy).
type DarkbaseGenerator struct {
	// BaseURL is the adapter root, e.g. "http://image-adapter.darkbase.svc:8081".
	// No trailing slash.
	BaseURL string

	// HTTPClient is the underlying client. nil → http.DefaultClient. The
	// per-request timeout for "generate" and "fetch image" is taken from
	// the caller's context, NOT from `Client.Timeout`, so a long DarkBase
	// render is not killed by a global client timeout. Callers must pass
	// a request context with a deadline appropriate to their UX.
	HTTPClient *http.Client

	// PollInterval controls how often the generator polls
	// `GET /api/v1/images/{job_id}`. Zero → 1500ms. Capped to 1s minimum
	// to protect the adapter.
	PollInterval time.Duration

	// AuthToken, when non-empty, is sent as `Authorization: Bearer <token>`
	// on every outbound request. DarkBase image-adapter does not enforce
	// auth today (in-cluster only), but plumbing the header now means the
	// optional follow-up DarkBase-PR-Q can flip it on without a Logos
	// release.
	AuthToken string
}

// jobStatusResponse is the subset of DarkBase's JobInfo we depend on.
// Extra fields are tolerated by encoding/json (we don't DisallowUnknownFields).
type jobStatusResponse struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Seed         int64  `json:"seed"`
	ImageURL     string `json:"image_url"`
	ErrorMessage string `json:"error_message"`
}

// errorResponse is the DarkBase {error: {code, message, details}, metadata: …}
// shape used by `make_error()`. message is what we surface back.
type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Generate submits the request, polls until done, and returns the bytes.
func (g *DarkbaseGenerator) Generate(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(g.BaseURL) == "" {
		return Result{}, fmt.Errorf("%w: BaseURL is empty", ErrUnavailable)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return Result{}, fmt.Errorf("%w: prompt is required", ErrInvalidRequest)
	}

	job, err := g.submit(ctx, req)
	if err != nil {
		return Result{}, err
	}

	final, err := g.waitForCompletion(ctx, job.ID)
	if err != nil {
		return Result{}, err
	}

	imgBytes, err := g.fetchOutput(ctx, final.ImageURL)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Bytes:       imgBytes,
		ContentType: "image/png",
		Width:       final.Width,
		Height:      final.Height,
		Seed:        final.Seed,
		Model:       req.Model,
	}, nil
}

// submit posts the form-data and returns the queued job.
func (g *DarkbaseGenerator) submit(ctx context.Context, req Request) (jobStatusResponse, error) {
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	fields := map[string]string{
		"prompt": req.Prompt,
	}
	if req.Width > 0 {
		fields["width"] = strconv.Itoa(req.Width)
	}
	if req.Height > 0 {
		fields["height"] = strconv.Itoa(req.Height)
	}
	if req.Steps > 0 {
		fields["steps"] = strconv.Itoa(req.Steps)
	}
	if req.Seed != 0 {
		fields["seed"] = strconv.FormatInt(req.Seed, 10)
	}
	if req.CFGScale != 0 {
		fields["cfg_scale"] = strconv.FormatFloat(req.CFGScale, 'f', -1, 64)
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return jobStatusResponse{}, fmt.Errorf("imagegen: encode form field %s: %w", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		return jobStatusResponse{}, fmt.Errorf("imagegen: close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.BaseURL+"/api/v1/images/generate", buf)
	if err != nil {
		return jobStatusResponse{}, fmt.Errorf("imagegen: build submit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	g.applyAuth(httpReq)

	resp, err := g.client().Do(httpReq)
	if err != nil {
		return jobStatusResponse{}, fmt.Errorf("%w: submit: %s", ErrUnavailable, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		return jobStatusResponse{}, fmt.Errorf("%w: submit returned %s",
			ErrUnavailable, resp.Status)
	}
	if resp.StatusCode >= 400 {
		return jobStatusResponse{}, fmt.Errorf("%w: %s",
			ErrInvalidRequest, decodeErrorMessage(resp))
	}

	var job jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return jobStatusResponse{}, fmt.Errorf("imagegen: decode submit response: %w", err)
	}
	if job.ID == "" {
		return jobStatusResponse{}, fmt.Errorf("%w: submit response missing id", ErrUnavailable)
	}
	return job, nil
}

// waitForCompletion polls until status ∈ {completed, error} or ctx ends.
func (g *DarkbaseGenerator) waitForCompletion(ctx context.Context, jobID string) (jobStatusResponse, error) {
	interval := g.PollInterval
	if interval <= 0 {
		interval = 1500 * time.Millisecond
	}
	if interval < 1*time.Second {
		// Floor: avoid hammering the adapter with sub-second polls. The
		// FLUX.2 worker takes seconds at minimum.
		interval = 1 * time.Second
	}

	statusURL := g.BaseURL + "/api/v1/images/" + url.PathEscape(jobID)

	t := time.NewTimer(0)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return jobStatusResponse{}, ctx.Err()
		case <-t.C:
		}

		st, err := g.fetchStatus(ctx, statusURL)
		if err != nil {
			return jobStatusResponse{}, err
		}
		switch st.Status {
		case "completed":
			if st.ImageURL == "" {
				return jobStatusResponse{}, fmt.Errorf(
					"%w: completed job has no image_url", ErrUnavailable)
			}
			return st, nil
		case "error":
			msg := st.ErrorMessage
			if msg == "" {
				msg = "unknown error"
			}
			return jobStatusResponse{}, fmt.Errorf("%w: %s", ErrJobFailed, msg)
		case "queued", "processing":
			// loop
		default:
			return jobStatusResponse{}, fmt.Errorf(
				"%w: unexpected job status %q", ErrUnavailable, st.Status)
		}
		t.Reset(interval)
	}
}

func (g *DarkbaseGenerator) fetchStatus(ctx context.Context, statusURL string) (jobStatusResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return jobStatusResponse{}, fmt.Errorf("imagegen: build status request: %w", err)
	}
	g.applyAuth(httpReq)
	resp, err := g.client().Do(httpReq)
	if err != nil {
		return jobStatusResponse{}, fmt.Errorf("%w: poll: %s", ErrUnavailable, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return jobStatusResponse{}, fmt.Errorf("%w: job not found", ErrUnavailable)
	}
	if resp.StatusCode >= 500 {
		return jobStatusResponse{}, fmt.Errorf("%w: status returned %s",
			ErrUnavailable, resp.Status)
	}
	if resp.StatusCode >= 400 {
		return jobStatusResponse{}, fmt.Errorf("%w: %s",
			ErrInvalidRequest, decodeErrorMessage(resp))
	}

	var st jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return jobStatusResponse{}, fmt.Errorf("imagegen: decode status: %w", err)
	}
	return st, nil
}

// fetchOutput downloads the PNG from `image_url`. The adapter returns a
// path relative to the API root; we resolve it against BaseURL.
func (g *DarkbaseGenerator) fetchOutput(ctx context.Context, imageURL string) ([]byte, error) {
	full := imageURL
	if !strings.HasPrefix(strings.ToLower(full), "http") {
		// `image_url` is server-relative (e.g. "/api/v1/images/outputs/<id>.png").
		full = g.BaseURL + full
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("imagegen: build fetch request: %w", err)
	}
	g.applyAuth(httpReq)
	resp, err := g.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch output: %s", ErrUnavailable, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: output file missing", ErrUnavailable)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: fetch returned %s", ErrUnavailable, resp.Status)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRequest, decodeErrorMessage(resp))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imagegen: read output body: %w", err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: output body empty", ErrUnavailable)
	}
	return body, nil
}

func (g *DarkbaseGenerator) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return http.DefaultClient
}

func (g *DarkbaseGenerator) applyAuth(r *http.Request) {
	if tok := strings.TrimSpace(g.AuthToken); tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
}

// decodeErrorMessage best-effort extracts the human message from a
// DarkBase error envelope; falls back to the status text.
func decodeErrorMessage(resp *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil || len(body) == 0 {
		return resp.Status
	}
	var env errorResponse
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Message != "" {
		return env.Error.Message
	}
	return strings.TrimSpace(string(body))
}

// Compile-time check.
var _ Generator = (*DarkbaseGenerator)(nil)
