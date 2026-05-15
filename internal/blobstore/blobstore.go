// Package blobstore is a small abstraction over a content-addressed object
// store for image blobs.
//
// Today the only implementation is LocalStore (a directory on a Kubernetes
// PersistentVolumeClaim). The interface exists to keep the door open for a
// future MinIO-backed implementation without touching every handler.
//
// Design choices:
//
//   - The id of the row in the `images` table is the canonical identity of
//     the blob. The Store is responsible for the layout on disk; callers do
//     **not** know whether the bytes are sharded, hashed, or stored as a
//     flat directory.
//   - File extensions are tracked alongside the id so a `.png` and a `.jpg`
//     never collide. An empty extension is rejected — every blob carries a
//     non-empty extension derived from its content type at ingest time.
//   - Open returns an `fs.File` rather than `io.ReadSeekCloser` so handlers
//     can use `http.ServeContent` (which needs the Stat() shape) without a
//     manual buffering pass. The returned file MUST be closed by the caller.
//   - Errors from the underlying filesystem are wrapped with the operation
//     name and the id; callers translate `fs.ErrNotExist` into HTTP 404.
package blobstore

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"github.com/google/uuid"
)

// ErrInvalidExt is returned by every operation when the ext argument is empty
// or contains a path separator. Callers must always pass a clean leaf-only
// extension like "png" or "jpg" (without the leading dot).
var ErrInvalidExt = errors.New("blobstore: invalid extension")

// Store is the operational interface for image blob persistence.
type Store interface {
	// Put streams r into the store under (id, ext). If a blob already
	// exists at that key it is overwritten — the caller is responsible
	// for de-duplication if it cares.
	Put(ctx context.Context, id uuid.UUID, ext string, r io.Reader) error

	// Path returns a server-relative URL path that, when combined with
	// the corresponding `GET /api/v1/images/{id}/blob` route, lets a
	// browser fetch the bytes. This is what we persist into `images.url`
	// for `uploaded` / `generated` rows. The handler is then responsible
	// for translating the URL back into an Open() call.
	Path(id uuid.UUID, ext string) string

	// Open returns the blob bytes for (id, ext) along with a Stat handle
	// (so handlers can call http.ServeContent). It returns fs.ErrNotExist
	// when the blob is absent.
	Open(id uuid.UUID, ext string) (fs.File, error)

	// Delete removes the blob at (id, ext). Missing blobs are not an error
	// — the typical caller is the row-delete handler, which has already
	// removed the row and is best-effort cleaning up disk.
	Delete(id uuid.UUID, ext string) error
}
