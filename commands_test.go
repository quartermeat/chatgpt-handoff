package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestDateDetection(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "context")
	put(t, filepath.Join(archive, "README.md"), "Export requested: 2026-01-02\n")
	if d, _ := latestDate(archive); d != "2026-01-02" {
		t.Fatal(d)
	}
	put(t, filepath.Join(base, "chatgpt-handoff-20260304.zip"), "")
	if d, _ := latestDate(archive); d != "2026-03-04" {
		t.Fatal(d)
	}
	put(t, filepath.Join(archive, "updates", "2026-04-05", "entry.md"), "new")
	if d, _ := latestDate(archive); d != "2026-04-05" {
		t.Fatal(d)
	}
}
func TestPromptContents(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "context")
	put(t, filepath.Join(archive, "README.md"), "Export requested: 2026-01-02\n")
	put(t, filepath.Join(archive, "01-personal", "profile.md"), "test")
	put(t, filepath.Join(archive, "updates", "2026-01-02", "change.md"), "test")
	put(t, filepath.Join(archive, "updates", "2026-01-02", "README.md"), "noise")
	p, since, err := promptText(archive, "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"2026-01-02", "updates/2026-10-26", "01-personal/profile.md", "updates/2026-01-02/change.md"} {
		if !strings.Contains(p, v) {
			t.Fatalf("missing %s", v)
		}
	}
	if strings.Contains(p, "updates/2026-01-02/README.md") || since != "2026-01-02" {
		t.Fatal("prompt listing or since date")
	}
	for _, rule := range []string{
		"version: 1", "header", "title", "chunk", "more", "files:", "path:", "content: |",
		"In chunk 1, create `updates/2026-10-26/README.md`", "List each update file", "date range it covers",
		"keeping that topic's folder", "updates/2026-10-26/03-career/project-management.md",
		"**New**", "**Changed**", "**No longer true**", "single chunk containing only the README",
		"one code fence, then stop", "say \"next\"", "01-personal/profile.md, updates/2026-01-02/change.md",
		"title: Updates since 2026-01-02", "- path: updates/2026-10-26/README.md",
	} {
		if !strings.Contains(p, rule) {
			t.Fatalf("missing prompt rule %q", rule)
		}
	}
	empty := filepath.Join(base, "empty")
	p, since, err = promptText(empty, "2026-10-26")
	if err != nil || p != firstExportPrompt || since != "" {
		t.Fatal("first export prompt")
	}
}
func TestReferenceShapeReplyImports(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "context")
	put(t, filepath.Join(archive, "README.md"), "Export requested: 2026-01-02\n")
	p, _, err := promptText(archive, "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(base, "prompts", "update-2026-10-26.md"), p)
	reply := "```yaml\nversion: 1\nheader:\n  title: Updates since 2026-01-02\n  chunk: 1\n  more: false\nfiles:\n  - path: updates/2026-10-26/README.md\n    content: |\n      # Updates since 2026-01-02\n      Nothing changed.\n```\n"
	result, err := importUpdate(archive, []byte(reply))
	if err != nil {
		t.Fatal(err)
	}
	if result["next_step"] != "done" {
		t.Fatal(result)
	}
	b, err := os.ReadFile(filepath.Join(archive, "updates", "2026-10-26", "README.md"))
	if err != nil || !strings.Contains(string(b), "Nothing changed.") {
		t.Fatal(err, string(b))
	}
}
func bundleFor(path string, more bool) []byte {
	return []byte("```yaml\nversion: 1\nheader:\n  title: Update\n  chunk: 1\n  more: " + map[bool]string{true: "true", false: "false"}[more] + "\nfiles:\n  - path: " + path + "\n    content: hello\n```\n")
}
func TestImportUpdate(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "context")
	put(t, filepath.Join(base, "prompts", "update-2026-10-26.md"), "update prompt")
	for _, path := range []string{"outside.md", "updates/2026-10-25/file.md", "updates/2026-10-26/../bad.md", "/tmp/escape"} {
		if _, err := importUpdate(archive, bundleFor(path, true)); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	result, err := importUpdate(archive, bundleFor("updates/2026-10-26/one.md", true))
	if err != nil {
		t.Fatal(err)
	}
	if result["next_step"] != "tell ChatGPT: next" || !strings.HasSuffix(result["raw_path"].(string), "chunk-001.yaml") {
		t.Fatal(result)
	}
	if _, err := importUpdate(archive, bundleFor("updates/2026-10-26/one.md", true)); err == nil {
		t.Fatal("overwrote path")
	}
	result, err = importUpdate(archive, bundleFor("updates/2026-10-26/two.md", false))
	if err != nil {
		t.Fatal(err)
	}
	if result["next_step"] != "done" || !strings.HasSuffix(result["raw_path"].(string), "chunk-002.yaml") {
		t.Fatal(result)
	}
	b, err := os.ReadFile(filepath.Join(archive, "updates", "2026-10-26", "two.md"))
	if err != nil || string(b) != "hello" {
		t.Fatal(err, string(b))
	}
	info, err := os.Stat(result["raw_path"].(string))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("raw file mode", err)
	}
	j, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(j, &v); err != nil || v["more"] != false || v["created_paths"] == nil {
		t.Fatal("JSON shape", err)
	}
}
func TestFirstExportImport(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "context")
	put(t, filepath.Join(base, "prompts", "update-2026-10-26.md"), firstExportPrompt)
	result, err := importUpdate(archive, bundleFor("README.md", false))
	if err != nil {
		t.Fatal(err)
	}
	if result["next_step"] != "done" {
		t.Fatal(result)
	}
}

func captureOutput(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	callErr := fn()
	w.Close()
	os.Stdout = old
	if callErr != nil {
		t.Fatal(callErr)
	}
	var out strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	r.Close()
	return out.String()
}
func TestCommandJSON(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "context")
	put(t, filepath.Join(archive, "README.md"), "Export requested: 2026-01-02\n")
	p := captureOutput(t, func() error { return promptCommand([]string{"--archive", archive, "--json"}) })
	var prompt map[string]any
	if err := json.Unmarshal([]byte(p), &prompt); err != nil {
		t.Fatal(err)
	}
	if prompt["since"] != "2026-01-02" || prompt["prompt_path"] == nil || prompt["copied"] != false || prompt["folder"] == nil || prompt["next_step"] == nil {
		t.Fatal(prompt)
	}
	s := captureOutput(t, func() error { return statusCommand([]string{"--archive", archive, "--json"}) })
	var status map[string]any
	if err := json.Unmarshal([]byte(s), &status); err != nil {
		t.Fatal(err)
	}
	if status["last_date"] != "2026-01-02" || status["chunks_imported"] != float64(0) || status["more"] != false || status["next_step"] == nil {
		t.Fatal(status)
	}
}
func TestImportWithoutPrompt(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "context")
	if _, err := importUpdate(archive, bundleFor("updates/2026-10-26/a.md", false)); err != nil {
		t.Fatal(err)
	}
}
