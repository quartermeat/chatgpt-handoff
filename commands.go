package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed PROMPT.md
var firstExportPrompt string

const updateTemplate = `I exported your memories about me on {{SINCE}}. Now send only what is **new or changed since then**, as files, one YAML chunk per reply.

Rules:
- Start each chunk with ` + "`version: 1`" + ` and a ` + "`header`" + ` containing ` + "`title`" + `, ` + "`chunk`" + ` (starting at 1), and ` + "`more`" + ` (true until the last chunk).
- Put every file under ` + "`{{FOLDER}}/`" + `. Never reuse a path from the earlier export.
- In chunk 1, create ` + "`{{FOLDER}}/README.md`" + `. List each update file, say which earlier topic file it amends, and give the date range it covers.
- Name each update file after the topic it amends, keeping that topic's folder. For example, changes to ` + "`03-career/project-management.md`" + ` go in ` + "`{{FOLDER}}/03-career/project-management.md`" + `. New topics get a new descriptive path.
- In each file, use three sections: **New**, **Changed** (old -> new, with the date when known), and **No longer true**. Skip topics with no changes.
- Include only details you actually know. Mark uncertainty and dates. Do not restate unchanged facts. Omit secrets, account identifiers, and inferred sensitive facts.
- If nothing changed, return a single chunk containing only the README, saying so.
- Return one YAML document only, inside one code fence, then stop. Wait for me to say "next" before sending the next chunk.

Earlier topic files, for reference:
{{FILES}}

Use this shape:

` + "```yaml\n" + `version: 1
header:
  title: Updates since {{SINCE}}
  chunk: 1
  more: true
files:
  - path: {{FOLDER}}/README.md
    content: |
      # Updates since {{SINCE}}
      ...
` + "```\n"

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var zipPattern = regexp.MustCompile(`^chatgpt-handoff-(\d{4})(\d{2})(\d{2})\.zip$`)
var rawPattern = regexp.MustCompile(`^update-(\d{4}-\d{2}-\d{2})-chunk-(\d{3})\.yaml$`)

func defaultArchive() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "private", "chatgpt-handoff", "context"), nil
}
func dateOK(s string) bool {
	if !datePattern.MatchString(s) {
		return false
	}
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}
func latestDate(archive string) (string, error) {
	var dates []string
	entries, err := os.ReadDir(filepath.Join(archive, "updates"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && dateOK(e.Name()) {
			dates = append(dates, e.Name())
		}
	}
	if len(dates) > 0 {
		sort.Strings(dates)
		return dates[len(dates)-1], nil
	}
	entries, err = os.ReadDir(filepath.Dir(archive))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, e := range entries {
		if m := zipPattern.FindStringSubmatch(e.Name()); m != nil {
			d := m[1] + "-" + m[2] + "-" + m[3]
			if dateOK(d) {
				dates = append(dates, d)
			}
		}
	}
	if len(dates) > 0 {
		sort.Strings(dates)
		return dates[len(dates)-1], nil
	}
	b, err := os.ReadFile(filepath.Join(archive, "README.md"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Export requested:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "Export requested:"))
			if dateOK(d) {
				return d, nil
			}
		}
	}
	return "", nil
}
func archiveFiles(archive string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(archive, func(path string, d os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == archive {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(archive, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.Contains(rel, "/") {
			return nil
		}
		if strings.HasPrefix(rel, "updates/") && strings.EqualFold(filepath.Base(rel), "README.md") {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}
func promptText(archive, today string) (string, string, error) {
	since, err := latestDate(archive)
	if err != nil {
		return "", "", err
	}
	if since == "" {
		return firstExportPrompt, "", nil
	}
	files, err := archiveFiles(archive)
	if err != nil {
		return "", "", err
	}
	listing := "(none)"
	if len(files) > 0 {
		listing = strings.Join(files, ", ")
	}
	folder := "updates/" + today
	p := strings.NewReplacer("{{SINCE}}", since, "{{TODAY}}", today, "{{FOLDER}}", folder, "{{FILES}}", listing).Replace(updateTemplate)
	return p, since, nil
}
func writePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("unsafe directory %s", filepath.Dir(path))
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("unsafe prompt path %s", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
func clipboard(write bool, data string) (string, error) {
	var command []string
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		command = []string{"wl-copy"}
	} else {
		command = []string{"xclip", "-selection", "clipboard"}
	}
	path, err := exec.LookPath(command[0])
	if err != nil {
		return "", fmt.Errorf("clipboard command %s unavailable: %w", command[0], err)
	}
	cmd := exec.Command(path, command[1:]...)
	if write {
		cmd.Stdin = strings.NewReader(data)
	}
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}
	return string(b), nil
}
func printJSON(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
func promptCommand(args []string) error {
	def, err := defaultArchive()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	archive := fs.String("archive", def, "archive directory")
	copyFlag := fs.Bool("copy", false, "copy to clipboard")
	jsonFlag := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("prompt takes no positional arguments")
	}
	today := time.Now().Format("2006-01-02")
	p, since, err := promptText(*archive, today)
	if err != nil {
		return err
	}
	promptPath := filepath.Join(filepath.Dir(*archive), "prompts", "update-"+today+".md")
	if err := writePrivate(promptPath, []byte(p)); err != nil {
		return err
	}
	copied := false
	if *copyFlag {
		if _, err := clipboard(true, p); err != nil {
			return err
		}
		copied = true
	}
	if *jsonFlag {
		return printJSON(map[string]any{"since": since, "folder": "updates/" + today, "prompt_path": promptPath, "copied": copied, "next_step": "paste the prompt into ChatGPT"})
	}
	fmt.Print(p)
	fmt.Printf("\nSaved prompt: %s\nNext: paste the prompt into ChatGPT.\n", promptPath)
	return nil
}
func stripFence(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 && (strings.TrimSpace(lines[0]) == "```yaml" || strings.TrimSpace(lines[0]) == "```yml" || strings.TrimSpace(lines[0]) == "```") && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		return []byte(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return b
}
func parseBundle(b []byte) (bundle, error) {
	var data bundle
	decoder := yaml.NewDecoder(bytes.NewReader(stripFence(b)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&data); err != nil {
		return data, fmt.Errorf("invalid YAML: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return data, errors.New("expected one YAML document")
	}
	if data.Version != 1 {
		return data, errors.New("version must be 1")
	}
	if strings.TrimSpace(data.Header.Title) == "" || data.Header.Chunk < 1 {
		return data, errors.New("header needs a title and positive chunk number")
	}
	if len(data.Files) == 0 {
		return data, errors.New("files must contain at least one entry")
	}
	seen := map[string]bool{}
	for _, f := range data.Files {
		if err := validPath(f.Path); err != nil {
			return data, fmt.Errorf("path %q: %w", f.Path, err)
		}
		if seen[f.Path] {
			return data, fmt.Errorf("duplicate path %q", f.Path)
		}
		seen[f.Path] = true
	}
	return data, nil
}
func matchingPromptDate(archive string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(archive), "prompts"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var dates []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "update-") && strings.HasSuffix(n, ".md") {
			d := strings.TrimSuffix(strings.TrimPrefix(n, "update-"), ".md")
			if dateOK(d) {
				dates = append(dates, d)
			}
		}
	}
	if len(dates) == 0 {
		return "", nil
	}
	sort.Strings(dates)
	return dates[len(dates)-1], nil
}
func importUpdate(archive string, b []byte) (map[string]any, error) {
	data, err := parseBundle(b)
	if err != nil {
		return nil, err
	}
	promptDate, err := matchingPromptDate(archive)
	if err != nil {
		return nil, err
	}
	date := ""
	firstExport := false
	if promptDate != "" {
		promptBytes, err := os.ReadFile(filepath.Join(filepath.Dir(archive), "prompts", "update-"+promptDate+".md"))
		if err != nil {
			return nil, err
		}
		firstExport = string(promptBytes) == firstExportPrompt
	}
	if firstExport {
		date = promptDate
	}
	paths := make([]string, 0, len(data.Files))
	for _, f := range data.Files {
		if firstExport {
			paths = append(paths, filepath.Join(archive, filepath.FromSlash(f.Path)))
			continue
		}
		parts := strings.Split(f.Path, "/")
		if len(parts) < 3 || parts[0] != "updates" || !dateOK(parts[1]) {
			return nil, fmt.Errorf("path %q must be under updates/<date>/", f.Path)
		}
		if date == "" {
			date = parts[1]
		}
		if parts[1] != date {
			return nil, errors.New("chunk spans update folders")
		}
		if promptDate != "" && date != promptDate {
			return nil, fmt.Errorf("path %q must be under updates/%s/", f.Path, promptDate)
		}
		paths = append(paths, filepath.Join(archive, filepath.FromSlash(f.Path)))
	}
	root, err := filepath.Abs(archive)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("unsafe archive directory")
	}
	rawDir := filepath.Join(filepath.Dir(root), "updates-raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	next := 1
	for _, e := range entries {
		if m := rawPattern.FindStringSubmatch(e.Name()); m != nil && m[1] == date {
			var n int
			fmt.Sscanf(m[2], "%d", &n)
			if n >= next {
				next = n + 1
			}
		}
	}
	rawPath := filepath.Join(rawDir, fmt.Sprintf("update-%s-chunk-%03d.yaml", date, next))
	for _, path := range paths {
		if err := safeParent(root, filepath.Dir(path)); err != nil {
			return nil, err
		}
		if _, err := os.Lstat(path); err == nil {
			return nil, fmt.Errorf("refusing to replace %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := os.MkdirAll(rawDir, 0700); err != nil {
		return nil, err
	}
	rawFile, err := os.OpenFile(rawPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	_, err = rawFile.Write(stripFence(b))
	closeErr := rawFile.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	for i, f := range data.Files {
		out, err := os.OpenFile(paths[i], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, err
		}
		_, err = io.WriteString(out, f.Content)
		closeErr = out.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	step := "done"
	if data.Header.More {
		step = "tell ChatGPT: next"
	}
	return map[string]any{"date": date, "chunk": data.Header.Chunk, "more": data.Header.More, "raw_path": rawPath, "created_paths": paths, "next_step": step, "summary": fmt.Sprintf("%d file(s) added", len(paths))}, nil
}
func importCommand(args []string) error {
	def, err := defaultArchive()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	archive := fs.String("archive", def, "archive directory")
	paste := fs.Bool("paste", false, "read clipboard")
	jsonFlag := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 || (*paste && fs.NArg() > 0) {
		return errors.New("usage: import [--archive DIR] [FILE|-|--paste]")
	}
	var b []byte
	if *paste {
		s, err := clipboard(false, "")
		if err != nil {
			return err
		}
		b = []byte(s)
	} else if fs.NArg() == 0 || fs.Arg(0) == "-" {
		b, err = io.ReadAll(io.LimitReader(os.Stdin, 10<<20))
	} else {
		b, err = os.ReadFile(fs.Arg(0))
	}
	if err != nil {
		return err
	}
	result, err := importUpdate(*archive, b)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Println("Raw:", result["raw_path"])
	for _, p := range result["created_paths"].([]string) {
		fmt.Println("Created:", p)
	}
	fmt.Println("Summary:", result["summary"])
	fmt.Println("Next:", result["next_step"])
	return nil
}
func statusCommand(args []string) error {
	def, err := defaultArchive()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	archive := fs.String("archive", def, "archive directory")
	jsonFlag := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("status takes no positional arguments")
	}
	last, err := latestDate(*archive)
	if err != nil {
		return err
	}
	current, err := matchingPromptDate(*archive)
	if err != nil {
		return err
	}
	count := 0
	more := false
	rawDir := filepath.Join(filepath.Dir(*archive), "updates-raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if current == "" {
		for _, e := range entries {
			if m := rawPattern.FindStringSubmatch(e.Name()); m != nil && m[1] > current {
				current = m[1]
			}
		}
	}
	latestName := ""
	for _, e := range entries {
		m := rawPattern.FindStringSubmatch(e.Name())
		if m == nil || m[1] != current {
			continue
		}
		count++
		if e.Name() > latestName {
			latestName = e.Name()
		}
	}
	if latestName != "" {
		b, err := os.ReadFile(filepath.Join(rawDir, latestName))
		if err != nil {
			return err
		}
		data, err := parseBundle(b)
		if err != nil {
			return err
		}
		more = data.Header.More
	}
	next := "run chatgpt-handoff prompt"
	if current != "" && count == 0 {
		next = "paste the prompt into ChatGPT, then import its reply"
	}
	if count > 0 {
		next = "done"
		if more {
			next = "tell ChatGPT: next"
		}
	}
	result := map[string]any{"last_date": last, "current_update": current, "chunks_imported": count, "more": more, "next_step": next}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Printf("Last date: %s\nCurrent update: %s\nChunks imported: %d\nMore: %t\nNext: %s\n", last, current, count, more, next)
	return nil
}
