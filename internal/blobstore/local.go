package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// LocalStore writes blobs to a single directory on disk. Mounting that
// directory from a Kubernetes PVC gives the binary a persistent, durable
// blob store without an external object-storage dependency.
//
// Layout:
//
//	<root>/<id>.<ext>
//
// A flat layout is fine for the corpus this service actually carries
// (low thousands of images at most). If we ever overflow ext4's
// directory size sweet spot, a `<a>/<b>/<id>.<ext>` shard prefix is
// trivial to add behind the same interface.
type LocalStore struct {
	root string
}

// NewLocalStore validates that root exists and is writable, creating it if
// necessary, and returns a Store backed by it.
func NewLocalStore(root string) (*LocalStore, error) {
	if root == "" {
		return nil, errors.New("blobstore: local root must not be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("blobstore: resolve root: %w", err)
	}
	// Mode 0o755: directory is world-readable so the static-file path is
	// trivial; bytes are non-secret. Write is owner-only.
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("blobstore: create root %q: %w", abs, err)
	}
	// Smoke-write a marker so a misconfigured RO mount fails at startup
	// rather than on the first user upload.
	probe := filepath.Join(abs, ".writable")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return nil, fmt.Errorf("blobstore: probe write %q: %w", abs, err)
	}
	_ = os.Remove(probe)
	return &LocalStore{root: abs}, nil
}

// Root returns the absolute root directory; useful for tests and for
// startup logs (do NOT log this if it ever contains user identifiers).
func (s *LocalStore) Root() string { return s.root }

func (s *LocalStore) leaf(id uuid.UUID, ext string) (string, error) {
	if ext == "" || strings.ContainsAny(ext, `/\.`) {
		// Reject path separators and a literal '.' to keep the leaf
		// string-comparable to a known pattern. The dot is added by
		// us, never by the caller, so callers passing "png" not
		// ".png" is the documented contract.
		return "", ErrInvalidExt
	}
	return id.String() + "." + ext, nil
}

// Put streams r into <root>/<id>.<ext>. Files are written via a temp
// sibling + rename so a partial write never appears at the destination
// path; an in-flight upload that dies mid-stream leaves at most a
// `<id>.<ext>.tmp.<rand>` sibling (cleaned up on next process restart
// by the operator if it ever matters; small footprint).
func (s *LocalStore) Put(_ context.Context, id uuid.UUID, ext string, r io.Reader) error {
	leaf, err := s.leaf(id, ext)
	if err != nil {
		return err
	}
	full := filepath.Join(s.root, leaf)

	tmp, err := os.CreateTemp(s.root, leaf+".tmp.*")
	if err != nil {
		return fmt.Errorf("blobstore: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, r); err != nil {
		return fmt.Errorf("blobstore: copy: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("blobstore: fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("blobstore: close: %w", err)
	}
	if err := os.Rename(tmpPath, full); err != nil {
		return fmt.Errorf("blobstore: rename: %w", err)
	}
	cleanup = false
	return nil
}

// Path returns the server-relative URL where this blob is reachable.
//
// Important: this is intentionally a path on the Logos API itself
// (`/api/v1/images/{id}/blob`) rather than a static file path under
// `/uploads`. Going through the handler means we can enforce the same
// CORS / auth surface as the rest of the API, and we can drop a CDN /
// object-storage swap behind the interface later without rewriting any
// rows.
func (s *LocalStore) Path(id uuid.UUID, _ string) string {
	return "/api/v1/images/" + id.String() + "/blob"
}

// Open returns the blob's `fs.File` so handlers can pass it through to
// `http.ServeContent`. Returns fs.ErrNotExist when the blob is missing.
func (s *LocalStore) Open(id uuid.UUID, ext string) (fs.File, error) {
	leaf, err := s.leaf(id, ext)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(s.root, leaf))
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Delete removes the blob; missing blobs are tolerated (NotExist is treated
// as a no-op so the row-delete handler stays idempotent).
func (s *LocalStore) Delete(id uuid.UUID, ext string) error {
	leaf, err := s.leaf(id, ext)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.root, leaf)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// Compile-time check that LocalStore satisfies the Store interface.
var _ Store = (*LocalStore)(nil)
