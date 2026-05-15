// Package imagegen is a small abstraction over an image-generation backend.
//
// The interface is shaped around the question Logos actually wants to ask:
// "produce a single image from this prompt, return the bytes". The two
// implementations we know about live behind very different protocols
// (DarkBase exposes a multipart submit + a polling endpoint; Sparky a JSON
// gateway with bearer auth and a worker that has not yet been wired to
// produce an `output_uri`). Both fit through a `Generator.Generate` call
// that blocks until the image is ready or the context is cancelled.
//
// Why blocking? The HTTP handler that consumes this package
// (`POST /api/v1/images:generate`) is itself synchronous in v1 — Logos has
// no Redis / queue, and the alternative would be to leak job-id polling
// into LogosUI for the first cut. The user-side experience is "click
// Generate, wait, see the image"; the request-deadline (set by the
// caller's context) is the cancellation knob.
package imagegen

import (
	"context"
	"errors"
)

// Request is the inbound generation request as the handler sees it.
//
// Defaults are NOT applied here — the calling handler validates and fills
// in zero values from configuration. Generators may further coerce values
// to backend-specific multiples (e.g. DarkBase rounds to /16) and surface
// the resolved values back on Result.
type Request struct {
	// Prompt is the natural-language description of the image.
	Prompt string

	// Model is an optional model identifier passed through to the backend.
	// Empty = the backend's default.
	Model string

	// Width and Height are the requested pixel dimensions. Zero =
	// generator-default.
	Width  int
	Height int

	// Seed is the optional generator seed. Zero = generator-chosen
	// (random) seed; the chosen seed is echoed on Result so callers can
	// persist it for reproducibility.
	Seed int64

	// Steps is the optional denoising step count. Zero = generator-default.
	Steps int

	// CFGScale (classifier-free guidance) controls how strictly the model
	// follows the prompt. Zero = generator-default.
	CFGScale float64
}

// Result carries the generator's output back to the caller.
//
// `Bytes` is held in memory because the corpus we generate is tightly
// bounded (FLUX.2 outputs are <10 MiB at the configured 2048×2048 cap)
// and it lets the handler hash, decode, and persist with no temp file.
type Result struct {
	Bytes       []byte
	ContentType string
	Width       int
	Height      int
	Seed        int64
	Model       string
}

// Generator is the operational interface for image-generation backends.
//
// The contract is:
//   - Synchronous: returns when the image is ready, the context is
//     cancelled / deadline-exceeded, or an unrecoverable error occurs.
//   - Single-image: every call yields exactly one image (or an error).
//   - Cancellable: implementations MUST honor ctx.Done() during any
//     internal polling / streaming so callers can free request goroutines
//     on client disconnect.
type Generator interface {
	Generate(ctx context.Context, req Request) (Result, error)
}

// Errors returned by Generators. Wrapping is fine; callers compare with
// errors.Is.
var (
	// ErrInvalidRequest signals a 4xx-shaped failure that LogosUI can
	// surface to the user (validation, queue-full, etc.). The wrapped
	// error message is the human-readable reason.
	ErrInvalidRequest = errors.New("imagegen: invalid request")

	// ErrUnavailable signals the backend is unreachable or returned a
	// 5xx response. The handler maps this to 502 / 503 so a temporary
	// outage is distinguishable from a permanent client error.
	ErrUnavailable = errors.New("imagegen: backend unavailable")

	// ErrJobFailed signals the backend accepted the job but reported an
	// `error` terminal state (out-of-memory, prompt-policy reject, …).
	// The wrapped error preserves the backend's error_message.
	ErrJobFailed = errors.New("imagegen: job failed")
)
