package filters

import "testing"

func TestFilterRejects(t *testing.T) {
	f := New([]string{"admin", "blocked phrase", "spam.example"})

	cases := []struct {
		value string
		want  bool
	}{
		{"hello world", false},
		{"Admin", true},
		{"  ADMIN  ", true},
		{"the administrator", true},
		{"this contains a blocked phrase here", true},
		{"visit spam.example today", true},
		{"spam.example.com", true},
		{"spam", false},
		{"", false},
	}
	for _, c := range cases {
		if got := f.Rejects(c.value); got != c.want {
			t.Errorf("Rejects(%q) = %v, want %v", c.value, got, c.want)
		}
	}

	empty := New(nil)
	if empty.Rejects("admin") {
		t.Error("empty filter rejected a value")
	}
}

func TestCountURLs(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"no urls here", 0},
		{"see https://example.com", 1},
		{"http://a.example and https://b.example", 2},
		{"ftp://not-counted.example", 0},
		{"https://example.com https://example.com https://example.com", 3},
	}
	for _, c := range cases {
		if got := CountURLs(c.text); got != c.want {
			t.Errorf("CountURLs(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}
