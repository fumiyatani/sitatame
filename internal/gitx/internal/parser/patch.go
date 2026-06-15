package parser

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

// PatchEntry holds the parser output for a single `diff --git` block.
type PatchEntry struct {
	APath  string // path with the `a/` prefix stripped (preimage path)
	BPath  string // path with the `b/` prefix stripped (postimage path)
	Binary bool
	Hunks  []diffmodel.Hunk
}

// ParsePatch parses the output of `git diff --patch --no-color`.
//
// It is intentionally strict about line shapes but tolerant of optional headers
// (similarity index, rename from/to, mode changes, index lines). Anything we
// don't recognize between `diff --git` and the first `@@` is skipped.
func ParsePatch(s string) ([]PatchEntry, error) {
	if s == "" {
		return nil, nil
	}
	var (
		out []PatchEntry
		cur *PatchEntry
		hk  *diffmodel.Hunk
	)
	flushHunk := func() {
		if cur != nil && hk != nil {
			cur.Hunks = append(cur.Hunks, *hk)
		}
		hk = nil
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			out = append(out, *cur)
		}
		cur = nil
	}

	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			a, b, err := parseDiffGitHeader(line)
			if err != nil {
				return nil, err
			}
			cur = &PatchEntry{APath: a, BPath: b}
		case cur == nil:
			// stray header before any diff --git; ignore.
			continue
		case strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ"):
			cur.Binary = true
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			h, err := ParseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hk = h
		case hk != nil && (len(line) > 0 && (line[0] == ' ' || line[0] == '+' || line[0] == '-')):
			// "--- a/..." and "+++ b/..." headers also start with - / +. Skip those.
			if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
				continue
			}
			hk.Lines = append(hk.Lines, diffmodel.Line{
				Prefix: line[0],
				Text:   line[1:],
			})
		case hk != nil && strings.HasPrefix(line, `\ No newline at end of file`):
			// Marker for the previous content line; we keep semantic content as-is
			// since the absence of a trailing newline is an editor concern, not a
			// review concern.
			continue
		default:
			// Pre-hunk metadata (index, similarity, rename from/to, mode lines,
			// "--- /dev/null" for new files). Currently ignored — joinRawAndPatch
			// uses raw/numstat for status detection.
			continue
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flushFile()
	return out, nil
}

// parseDiffGitHeader extracts the a/ and b/ paths from `diff --git a/X b/Y`.
// The header form is fixed by git itself; quoted paths (paths with unusual
// characters) appear as `"a/..."` and `"b/..."`. We don't unquote here because
// MVP scope excludes such paths in tests, but we accept them syntactically.
func parseDiffGitHeader(line string) (string, string, error) {
	rest := strings.TrimPrefix(line, "diff --git ")
	// Find the boundary between `a/...` and `b/...`. Git emits a single space
	// between them; if a path itself contains `b/` we're stuck without quoting.
	// For MVP, split on " b/" (space + b + slash) which is unambiguous unless
	// a path literally contains that substring at the boundary.
	idx := strings.Index(rest, " b/")
	if idx < 0 {
		return "", "", fmt.Errorf("diff --git header without ` b/`: %q", line)
	}
	a := rest[:idx]
	b := rest[idx+1:]
	a = strings.TrimPrefix(strings.TrimPrefix(a, `"`), "a/")
	if strings.HasSuffix(a, `"`) {
		a = strings.TrimSuffix(a, `"`)
	}
	b = strings.TrimPrefix(strings.TrimPrefix(b, `"`), "b/")
	if strings.HasSuffix(b, `"`) {
		b = strings.TrimSuffix(b, `"`)
	}
	return a, b, nil
}

// ParseHunkHeader parses `@@ -a,b +c,d @@ trailer` into a Hunk with starts and
// counts populated. Counts default to 1 when omitted (`@@ -10 +10 @@`).
func ParseHunkHeader(line string) (*diffmodel.Hunk, error) {
	end := strings.Index(line[2:], "@@")
	if end < 0 {
		return nil, fmt.Errorf("hunk header missing closing `@@`: %q", line)
	}
	header := strings.TrimSpace(line[2 : 2+end])
	tail := ""
	if rest := line[2+end+2:]; rest != "" {
		tail = strings.TrimSpace(rest)
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "-") || !strings.HasPrefix(parts[1], "+") {
		return nil, fmt.Errorf("hunk header: want `-a,b +c,d`, got %q", header)
	}
	bStart, bLines, err := parseHunkRange(parts[0][1:])
	if err != nil {
		return nil, fmt.Errorf("hunk header base: %w", err)
	}
	hStart, hLines, err := parseHunkRange(parts[1][1:])
	if err != nil {
		return nil, fmt.Errorf("hunk header head: %w", err)
	}
	return &diffmodel.Hunk{
		BaseStart: bStart,
		BaseLines: bLines,
		HeadStart: hStart,
		HeadLines: hLines,
		Header:    tail,
	}, nil
}

func parseHunkRange(s string) (int, int, error) {
	if s == "" {
		return 0, 0, fmt.Errorf("empty range")
	}
	if comma := strings.Index(s, ","); comma >= 0 {
		start, err := strconv.Atoi(s[:comma])
		if err != nil {
			return 0, 0, err
		}
		count, err := strconv.Atoi(s[comma+1:])
		if err != nil {
			return 0, 0, err
		}
		return start, count, nil
	}
	start, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, err
	}
	return start, 1, nil
}
