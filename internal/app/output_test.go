package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizePanPath(t *testing.T) {
	tests := map[string]string{
		"":              "/",
		"foo/bar":       "/foo/bar",
		"/foo//bar/":    "/foo/bar",
		"foo\\bar\\baz": "/foo/bar/baz",
	}
	for input, want := range tests {
		if got := normalizePanPath(input); got != want {
			t.Fatalf("normalizePanPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	err := writeOutput(&buf, "json", map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"ok": true`) {
		t.Fatalf("unexpected json output: %s", buf.String())
	}
}

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	err := writeOutput(&buf, "table", []map[string]any{{"name": "a", "size": 1}})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "name") || !strings.Contains(out, "size") || !strings.Contains(out, "a") {
		t.Fatalf("unexpected table output: %s", out)
	}
}
