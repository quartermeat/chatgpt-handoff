package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type bundle struct {
	Version int `yaml:"version"`
	Header  struct {
		Title string `yaml:"title"`
		Chunk int    `yaml:"chunk"`
		More  bool   `yaml:"more"`
	} `yaml:"header"`
	Files []struct {
		Path    string `yaml:"path"`
		Content string `yaml:"content"`
	} `yaml:"files"`
}

func main() {
	input := flag.String("input", "-", "YAML bundle path, or - for standard input")
	output := flag.String("out", "shared-memory", "directory for created files")
	flag.Parse()
	if err := run(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, "handoff:", err)
		os.Exit(1)
	}
}

func run(input, output string) error {
	var reader io.Reader = os.Stdin
	if input != "-" {
		file, err := os.Open(input)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}
	decoder := yaml.NewDecoder(io.LimitReader(reader, 10<<20))
	decoder.KnownFields(true)
	var data bundle
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("expected one YAML document")
	}
	if data.Version != 1 {
		return errors.New("version must be 1")
	}
	if data.Header.Title != "" || data.Header.Chunk != 0 || data.Header.More {
		if strings.TrimSpace(data.Header.Title) == "" || data.Header.Chunk < 1 {
			return errors.New("header needs a title and positive chunk number")
		}
		fmt.Printf("Chunk %d: %s (more: %t)\n", data.Header.Chunk, data.Header.Title, data.Header.More)
	}
	if len(data.Files) == 0 {
		return errors.New("files must contain at least one entry")
	}
	paths := make(map[string]bool)
	for _, file := range data.Files {
		if err := validPath(file.Path); err != nil {
			return fmt.Errorf("path %q: %w", file.Path, err)
		}
		if paths[file.Path] {
			return fmt.Errorf("duplicate path %q", file.Path)
		}
		paths[file.Path] = true
	}
	root, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("unsafe output directory %s", root)
	}
	for _, file := range data.Files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := safeParent(root, filepath.Dir(path)); err != nil {
			return err
		}
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to replace %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, file := range data.Files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := io.WriteString(f, file.Content)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println(path)
	}
	return nil
}

func validPath(path string) error {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") || strings.ContainsRune(path, 0) {
		return errors.New("must be a relative slash-separated path")
	}
	if filepath.Clean(path) != path || path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return errors.New("path traversal or noncanonical path")
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." || part == "" {
			return errors.New("invalid path segment")
		}
	}
	return nil
}

func safeParent(root, parent string) error {
	if parent == root {
		return nil
	}
	current := root
	relative, err := filepath.Rel(root, parent)
	if err != nil {
		return err
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("unsafe directory component %s", current)
		}
	}
	return nil
}
