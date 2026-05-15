package blobstore_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/blobstore"
)

func newStore(t *testing.T) *blobstore.LocalStore {
	t.Helper()
	dir := t.TempDir()
	s, err := blobstore.NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	return s
}

func TestLocalStore_PutOpenRoundTrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := uuid.New()

	body := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 64))
	if err := s.Put(context.Background(), id, "png", bytes.NewReader(body)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	f, err := s.Open(id, "png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("round-tripped body diverges (len=%d want=%d)", len(got), len(body))
	}

	// Stat must work so http.ServeContent has a length and ModTime.
	stat, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Size() != int64(len(body)) {
		t.Fatalf("Size = %d want %d", stat.Size(), len(body))
	}
}

func TestLocalStore_PathShape(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := uuid.MustParse("00000000-0000-0000-0000-000000000123")
	got := s.Path(id, "png")
	want := "/api/v1/images/00000000-0000-0000-0000-000000000123/blob"
	if got != want {
		t.Fatalf("Path = %q want %q", got, want)
	}
}

func TestLocalStore_OpenMissingReturnsNotExist(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := s.Open(uuid.New(), "png")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open(missing) = %v want fs.ErrNotExist", err)
	}
}

func TestLocalStore_DeleteMissingIsNoop(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	// Delete on a never-written id must not error: the row-delete handler
	// is best-effort cleaning up the disk after the row is already gone.
	if err := s.Delete(uuid.New(), "png"); err != nil {
		t.Fatalf("Delete(missing): %v", err)
	}
}

func TestLocalStore_PutAtomicReplace(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := uuid.New()

	if err := s.Put(context.Background(), id, "png", bytes.NewReader([]byte("v1"))); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := s.Put(context.Background(), id, "png", bytes.NewReader([]byte("v2"))); err != nil {
		t.Fatalf("Put v2: %v", err)
	}
	f, err := s.Open(id, "png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	got, _ := io.ReadAll(f)
	if string(got) != "v2" {
		t.Fatalf("after replace got = %q want v2", string(got))
	}
}

func TestLocalStore_PutDoesNotLeaveTempOnSuccess(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := uuid.New()
	if err := s.Put(context.Background(), id, "png", bytes.NewReader([]byte("ok"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	entries, err := os.ReadDir(s.Root())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Fatalf("unexpected leftover temp file: %s", e.Name())
		}
	}
}

func TestLocalStore_RejectsBadExtensions(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := uuid.New()
	cases := map[string]string{
		"empty":     "",
		"with dot":  ".png",
		"slash":     "p/g",
		"backslash": `p\g`,
	}
	for name, ext := range cases {
		t.Run(name, func(t *testing.T) {
			if err := s.Put(context.Background(), id, ext, bytes.NewReader([]byte("x"))); !errors.Is(err, blobstore.ErrInvalidExt) {
				t.Fatalf("Put(ext=%q) = %v want ErrInvalidExt", ext, err)
			}
		})
	}
}

func TestNewLocalStore_CreatesAndProbesRoot(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "nested", "blobs")
	s, err := blobstore.NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	if s.Root() != mustAbs(t, dir) {
		t.Fatalf("Root = %q want %q", s.Root(), dir)
	}
	// Probe file must be cleaned up.
	if _, err := os.Stat(filepath.Join(s.Root(), ".writable")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("probe marker not cleaned: %v", err)
	}
}

func TestNewLocalStore_RejectsEmptyRoot(t *testing.T) {
	t.Parallel()
	if _, err := blobstore.NewLocalStore(""); err == nil {
		t.Fatal("NewLocalStore(\"\") = nil, want error")
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}
