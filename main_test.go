package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportAndNoOverwrite(t *testing.T) {
	base := t.TempDir()
	input := filepath.Join(base, "bundle.yaml")
	output := filepath.Join(base, "out")
	yml := "version: 1\nheader:\n  title: Background\n  chunk: 1\n  more: true\nfiles:\n  - path: notes/about.md\n    content: |\n      Known fact.\n"
	if err := os.WriteFile(input, []byte(yml), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(input, output); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(output, "notes/about.md"))
	if err != nil || string(got) != "Known fact.\n" {
		t.Fatalf("content: %q, %v", got, err)
	}
	if err := run(input, output); err == nil || !strings.Contains(err.Error(), "refusing to replace") {
		t.Fatalf("overwrite: %v", err)
	}
}

func TestRejectTraversalAndSymlink(t *testing.T) {
	base := t.TempDir()
	input := filepath.Join(base, "bundle.yaml")
	output := filepath.Join(base, "out")
	if err := os.WriteFile(input, []byte("version: 1\nfiles:\n  - path: ../escape\n    content: no\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(input, output); err == nil {
		t.Fatal("accepted path traversal")
	}
	if _, err := os.Stat(filepath.Join(base, "escape")); !os.IsNotExist(err) {
		t.Fatalf("escape: %v", err)
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(output, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, []byte("version: 1\nfiles:\n  - path: linked/escape\n    content: no\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(input, output); err == nil {
		t.Fatal("accepted symlink parent")
	}
}
