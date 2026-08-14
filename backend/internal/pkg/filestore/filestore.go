// Package filestore is a thin wrapper over the local filesystem for user-uploaded
// content (payment proofs, design files). It centralizes:
//
//   - Root-relative path resolution (rejecting traversal attempts).
//   - Atomic-ish save via temp-file-then-rename.
//   - Deletion.
//
// Design notes:
//   - Callers provide the relative subpath — filestore never generates filenames
//     because domain modules encode meaning (proof_id / design_id) into names.
//   - Callers MUST pre-validate content type / size before Save. filestore does
//     not sniff mime types; that's a domain concern.
//   - Not a general-purpose "storage abstraction" — no S3/GCS interface here.
//     Spec §19 pins us to local VPS disk for MVP; abstract later if/when needed.
package filestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrPathTraversal is returned when a subpath tries to escape the storage
	// root (e.g. via "../"). Caller likely has a bug or is under attack.
	ErrPathTraversal = errors.New("filestore: subpath escapes storage root")
	// ErrEmptySubpath — defensive; callers should always supply a non-empty
	// destination.
	ErrEmptySubpath = errors.New("filestore: subpath must not be empty")
	// ErrNotFound — path did not exist for read/delete.
	ErrNotFound = errors.New("filestore: file not found")
)

type Store struct {
	root string
}

// New returns a Store rooted at absRoot. Caller has already ensured the dir
// exists and is writable (config.ensureWritableDir).
func New(absRoot string) *Store {
	// Normalize once so downstream comparisons work.
	return &Store{root: filepath.Clean(absRoot)}
}

// AbsPath resolves a subpath under the root, rejecting traversal. Exported so
// handlers/tests can build serving paths without duplicating the check.
func (s *Store) AbsPath(subpath string) (string, error) {
	if strings.TrimSpace(subpath) == "" {
		return "", ErrEmptySubpath
	}
	// Reject NUL bytes and absolute paths outright.
	if strings.ContainsRune(subpath, 0) {
		return "", ErrPathTraversal
	}
	// Reject absolute paths (OS-specific IsAbs) AND any leading slash/backslash —
	// on Windows "/foo" is not IsAbs but still indicates a callsite bug (they
	// meant a root-relative URL, not a filestore subpath).
	if filepath.IsAbs(subpath) || strings.HasPrefix(subpath, "/") || strings.HasPrefix(subpath, `\`) {
		return "", ErrPathTraversal
	}
	joined := filepath.Join(s.root, subpath)
	clean := filepath.Clean(joined)
	// Ensure the cleaned path is under root (defense against "..").
	rel, err := filepath.Rel(s.root, clean)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", ErrPathTraversal
	}
	return clean, nil
}

// Save streams `r` to <root>/<subpath>, creating parent dirs as needed. Writes
// to a sibling temp file first, then renames — protects against half-written
// files if the process dies mid-write.
func (s *Store) Save(_ context.Context, subpath string, r io.Reader) (int64, error) {
	abs, err := s.AbsPath(subpath)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return 0, fmt.Errorf("filestore: mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(abs), ".rajaku-upload-*")
	if err != nil {
		return 0, fmt.Errorf("filestore: create temp: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we return before rename.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	n, copyErr := io.Copy(tmp, r)
	closeErr := tmp.Close()
	if copyErr != nil {
		return n, fmt.Errorf("filestore: copy: %w", copyErr)
	}
	if closeErr != nil {
		return n, fmt.Errorf("filestore: close temp: %w", closeErr)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return n, fmt.Errorf("filestore: rename: %w", err)
	}
	success = true
	return n, nil
}

// Delete removes <root>/<subpath>. Returns ErrNotFound if the file doesn't
// exist (caller can ignore-if-not-found by checking the error).
func (s *Store) Delete(_ context.Context, subpath string) error {
	abs, err := s.AbsPath(subpath)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("filestore: delete: %w", err)
	}
	return nil
}
