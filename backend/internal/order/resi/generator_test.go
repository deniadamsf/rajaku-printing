package resi

import (
	"strings"
	"testing"
)

func TestGenerate_FormatAndCharset(t *testing.T) {
	for i := 0; i < 100; i++ {
		r, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.HasPrefix(r, Prefix) {
			t.Fatalf("missing prefix %q: %q", Prefix, r)
		}
		suffix := strings.TrimPrefix(r, Prefix)
		if len(suffix) != SuffixLen {
			t.Fatalf("suffix len: got %d want %d", len(suffix), SuffixLen)
		}
		for _, c := range suffix {
			if !strings.ContainsRune(Charset, c) {
				t.Fatalf("char %q not in charset %q (resi=%q)", c, Charset, r)
			}
		}
	}
}

func TestGenerate_UniquenessSampling(t *testing.T) {
	// Sample 10.000 resi, ekspektasi 0 collision (32^8 = 1.1 × 10^12).
	seen := make(map[string]struct{}, 10_000)
	for i := 0; i < 10_000; i++ {
		r, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if _, exists := seen[r]; exists {
			t.Fatalf("unexpected collision at i=%d: %q", i, r)
		}
		seen[r] = struct{}{}
	}
}

func TestGenerate_NoConfusableChars(t *testing.T) {
	// Charset sengaja tidak boleh mengandung I, O, 0, 1.
	for _, c := range Charset {
		switch c {
		case 'I', 'O', '0', '1':
			t.Fatalf("charset must NOT contain confusable %q", c)
		}
	}
}
