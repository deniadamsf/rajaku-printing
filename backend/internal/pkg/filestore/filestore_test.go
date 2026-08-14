package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAbsPath_Traversal(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	cases := []string{
		"../escape.txt",
		"foo/../../etc/passwd",
		"/absolute/nope",
	}
	for _, c := range cases {
		if _, err := s.AbsPath(c); !errors.Is(err, ErrPathTraversal) {
			t.Errorf("AbsPath(%q) want ErrPathTraversal, got %v", c, err)
		}
	}
}

func TestAbsPath_Empty(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.AbsPath(""); !errors.Is(err, ErrEmptySubpath) {
		t.Errorf("empty subpath want ErrEmptySubpath, got %v", err)
	}
}

func TestSaveAndDelete(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	body := "hello proof of transfer"
	n, err := s.Save(context.Background(), "payment_proofs/2026/08/abc.txt", strings.NewReader(body))
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if n != int64(len(body)) {
		t.Errorf("want %d bytes written, got %d", len(body), n)
	}

	got, err := os.ReadFile(filepath.Join(dir, "payment_proofs/2026/08/abc.txt"))
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(got) != body {
		t.Errorf("content roundtrip failed: %q vs %q", got, body)
	}

	if err := s.Delete(context.Background(), "payment_proofs/2026/08/abc.txt"); err != nil {
		t.Errorf("delete: %v", err)
	}
	if err := s.Delete(context.Background(), "payment_proofs/2026/08/abc.txt"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete want ErrNotFound, got %v", err)
	}
}

func TestSave_LeavesNoTempOnFailure(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	// Passing a nil reader triggers io.Copy error on nil Read — but a real
	// forceable failure is easier via a reader that errors out.
	_, err := s.Save(context.Background(), "payment_proofs/x.txt", errReader{})
	if err == nil {
		t.Fatal("expected error from failing reader")
	}
	// No temp file should linger in the target dir.
	entries, _ := os.ReadDir(filepath.Join(dir, "payment_proofs"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".rajaku-upload-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
		if e.Name() == "x.txt" {
			t.Errorf("target file created despite copy failure")
		}
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }
