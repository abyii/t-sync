package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/abyii/zip-xxh3"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func archiveEntries(t *testing.T, src, ignoreFile string) map[string]string {
	t.Helper()
	var buf bytes.Buffer
	if err := CreateZipArchive(src, &buf, DefaultCompressionLevel, "", ignoreFile); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("archive unreadable: %v", err)
	}
	entries := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[f.Name] = string(b)
	}
	return entries
}

func TestArchiveEntriesUseForwardSlashes(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"a/b/c.txt": "hello", "top.txt": "top"})

	got := archiveEntries(t, src, "")
	want := map[string]string{"a/": "", "a/b/": "", "a/b/c.txt": "hello", "top.txt": "top"}
	if len(got) != len(want) {
		t.Fatalf("entries %v, want %v", got, want)
	}
	for name, content := range want {
		if got[name] != content {
			t.Fatalf("entry %q = %q, want %q (all: %v)", name, got[name], content, got)
		}
	}
}

func TestDotfileIgnoreRuleDoesNotSkipRoot(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"keep.txt":      "k",
		".hidden":       "h",
		".git/config":   "g",
		"sub/song.mp3":  "m",
		"sub/notes.txt": "n",
	})
	ignore := filepath.Join(t.TempDir(), ".tsyncignore")
	if err := os.WriteFile(ignore, []byte(".*\n*.mp3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var names []string
	for name := range archiveEntries(t, src, ignore) {
		names = append(names, name)
	}
	slices.Sort(names)
	if want := []string{"keep.txt", "sub/", "sub/notes.txt"}; !slices.Equal(names, want) {
		t.Fatalf("entries %v, want %v", names, want)
	}
}
