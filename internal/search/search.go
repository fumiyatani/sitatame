// Package search provides a portable regexp-based scan over the on-disk review
// tree. It is the fallback path used when ripgrep is not available; the cmd
// layer prefers `rg -n` when present for speed and feature parity with the
// user's grep workflow.
package search

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Hit is a single match: a line in a file plus the match's location. Path is
// always absolute or repo-relative depending on what the caller passed to Walk.
type Hit struct {
	Path string
	Line int
	Text string
}

// Walk traverses root, opens every regular file with a .md suffix, and yields
// hits whose lines match re. Symlinks and non-regular files are skipped.
// Errors reading individual files are silently ignored so a single broken
// file doesn't abort an otherwise useful search.
func Walk(root string, re *regexp.Regexp) ([]Hit, error) {
	if re == nil {
		return nil, nil
	}
	var out []Hit
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(p), ".md") {
			return nil
		}
		hits, perr := scanFile(p, re)
		if perr != nil {
			return nil
		}
		out = append(out, hits...)
		return nil
	})
	return out, err
}

func scanFile(path string, re *regexp.Regexp) ([]Hit, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hits []Hit
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		text := sc.Text()
		if re.MatchString(text) {
			hits = append(hits, Hit{Path: path, Line: lineNo, Text: text})
		}
	}
	if err := sc.Err(); err != nil {
		return hits, err
	}
	return hits, nil
}
