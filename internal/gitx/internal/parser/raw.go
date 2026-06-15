// Package parser contains pure parsing functions for git diff output streams.
// It has no dependency on exec or any git runner, making it testable in isolation.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

// RawEntry mirrors a single line of `git diff --raw -z --find-renames --find-copies`.
//
// Format (per entry, NUL-separated paths):
//
//	:<srcmode> <dstmode> <srcsha> <dstsha> <status>\0<path1>[\0<path2>]
//
// For R/C statuses, <status> carries a similarity score (e.g. "R100").
type RawEntry struct {
	SrcMode    string
	DstMode    string
	SrcSHA     string
	DstSHA     string
	Status     diffmodel.Status
	Similarity int
	PrePath    string
	PostPath   string
}

// NumstatEntry mirrors a single line of `git diff --numstat -z --find-renames --find-copies`.
//
// Format (per entry):
//
//	<added>\t<deleted>\t\0<path1>[\0<path2>]
//
// Binary files have "-" for both counts.
type NumstatEntry struct {
	Added    int
	Deleted  int
	Binary   bool
	PrePath  string // empty unless rename/copy
	PostPath string // path1 for non-renames; rename target otherwise
}

// ParseRawZ parses the output of `git diff --raw -z --find-renames --find-copies`.
// The full output is one big NUL-delimited stream; every entry begins with ':'.
func ParseRawZ(s string) ([]RawEntry, error) {
	var out []RawEntry
	if s == "" {
		return out, nil
	}
	parts := strings.Split(s, "\x00")
	// parts trail with an empty element after the final NUL; trim it.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	for i := 0; i < len(parts); {
		header := parts[i]
		if !strings.HasPrefix(header, ":") {
			return nil, fmt.Errorf("raw entry %d: missing ':' prefix: %q", i, header)
		}
		fields := strings.Fields(strings.TrimPrefix(header, ":"))
		if len(fields) != 5 {
			return nil, fmt.Errorf("raw entry %d: want 5 fields, got %d: %q", i, len(fields), header)
		}
		statusRaw := fields[4]
		entry := RawEntry{
			SrcMode: fields[0],
			DstMode: fields[1],
			SrcSHA:  fields[2],
			DstSHA:  fields[3],
		}
		st, sim, err := ParseStatus(statusRaw)
		if err != nil {
			return nil, fmt.Errorf("raw entry %d: %w", i, err)
		}
		entry.Status = st
		entry.Similarity = sim

		// Renames / copies use 2 path fields, others use 1.
		if i+1 >= len(parts) {
			return nil, fmt.Errorf("raw entry %d: missing path after header", i)
		}
		switch entry.Status {
		case diffmodel.StatusRenamed, diffmodel.StatusCopied:
			if i+2 >= len(parts) {
				return nil, fmt.Errorf("raw entry %d: rename/copy missing dst path", i)
			}
			entry.PrePath = parts[i+1]
			entry.PostPath = parts[i+2]
			i += 3
		case diffmodel.StatusDeleted:
			entry.PrePath = parts[i+1]
			entry.PostPath = ""
			i += 2
		case diffmodel.StatusAdded:
			entry.PrePath = ""
			entry.PostPath = parts[i+1]
			i += 2
		default:
			entry.PrePath = parts[i+1]
			entry.PostPath = parts[i+1]
			i += 2
		}
		out = append(out, entry)
	}
	return out, nil
}

// ParseStatus parses a raw status field (e.g. "M", "R100", "C75") into a
// Status and similarity score.
func ParseStatus(s string) (diffmodel.Status, int, error) {
	if s == "" {
		return 0, 0, fmt.Errorf("empty status")
	}
	st := diffmodel.Status(s[0])
	if !st.Valid() {
		return 0, 0, fmt.Errorf("unknown status %q", s)
	}
	if len(s) == 1 {
		return st, 0, nil
	}
	// Trailing similarity, e.g. "R100" or "C75".
	sim, err := strconv.Atoi(s[1:])
	if err != nil {
		return 0, 0, fmt.Errorf("status %q: bad similarity: %w", s, err)
	}
	return st, sim, nil
}

// ParseNumstatZ parses the output of `git diff --numstat -z --find-renames --find-copies`.
//
// Two record shapes (per `man git-diff` -z section):
//
//	non-rename:  <added>\t<deleted>\t<path>\0
//	rename/copy: <added>\t<deleted>\t\0<preimage>\0<postimage>\0
//
// Binary records use "-" for both counts. A trailing empty third field after the
// second tab is the marker that a rename pair follows.
func ParseNumstatZ(s string) ([]NumstatEntry, error) {
	var out []NumstatEntry
	if s == "" {
		return out, nil
	}
	i := 0
	for i < len(s) {
		nl := strings.IndexByte(s[i:], '\x00')
		if nl < 0 {
			return nil, fmt.Errorf("numstat: missing terminator at offset %d", i)
		}
		head := s[i : i+nl]
		i += nl + 1

		fields := strings.SplitN(head, "\t", 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("numstat: want 'added\\tdeleted\\t...', got %q", head)
		}
		entry := NumstatEntry{}
		switch fields[0] {
		case "-":
			if fields[1] != "-" {
				return nil, fmt.Errorf("numstat: expected paired '-' counts, got %q", head)
			}
			entry.Binary = true
		default:
			a, err := strconv.Atoi(fields[0])
			if err != nil {
				return nil, fmt.Errorf("numstat: added count %q: %w", fields[0], err)
			}
			d, err := strconv.Atoi(fields[1])
			if err != nil {
				return nil, fmt.Errorf("numstat: deleted count %q: %w", fields[1], err)
			}
			entry.Added = a
			entry.Deleted = d
		}

		if fields[2] == "" {
			// rename/copy: two more NUL-terminated paths follow
			pre, n, err := readNulString(s, i)
			if err != nil {
				return nil, fmt.Errorf("numstat: rename preimage: %w", err)
			}
			i += n
			post, n, err := readNulString(s, i)
			if err != nil {
				return nil, fmt.Errorf("numstat: rename postimage: %w", err)
			}
			i += n
			entry.PrePath = pre
			entry.PostPath = post
		} else {
			entry.PostPath = fields[2]
		}
		out = append(out, entry)
	}
	return out, nil
}

// readNulString reads a NUL-terminated string starting at offset i and returns
// the content plus the number of bytes consumed (including the trailing NUL).
func readNulString(s string, i int) (string, int, error) {
	if i >= len(s) {
		return "", 0, fmt.Errorf("unexpected EOF at offset %d", i)
	}
	nl := strings.IndexByte(s[i:], '\x00')
	if nl < 0 {
		return "", 0, fmt.Errorf("unterminated string at offset %d", i)
	}
	return s[i : i+nl], nl + 1, nil
}

// JoinKey identifies a file across the raw / numstat / patch streams.
type JoinKey struct {
	Status   diffmodel.Status
	PrePath  string
	PostPath string
	BlobBase string
	BlobHead string
}

// KeyForRaw derives the join key from a RawEntry.
func KeyForRaw(e RawEntry) JoinKey {
	switch e.Status {
	case diffmodel.StatusDeleted:
		return JoinKey{
			Status:   e.Status,
			PrePath:  e.PrePath,
			BlobBase: e.SrcSHA,
		}
	default:
		return JoinKey{
			Status:   e.Status,
			PrePath:  e.PrePath,
			PostPath: e.PostPath,
			BlobBase: e.SrcSHA,
			BlobHead: e.DstSHA,
		}
	}
}

// KeyForNumstat builds a join key matching KeyForRaw using path information
// alone. numstat doesn't expose blob SHAs or status — this is best-effort and
// consumers must fall back to path-only matching when SHA fields are empty.
func KeyForNumstat(e NumstatEntry) JoinKey {
	return JoinKey{
		PrePath:  e.PrePath,
		PostPath: e.PostPath,
	}
}

// JoinRawAndNumstat fuses raw and numstat streams into a slice of File records.
// numstat's only purpose at this stage is to mark binary files; counts aren't
// retained because the patch parser builds the line-level model.
func JoinRawAndNumstat(rawEntries []RawEntry, numstatEntries []NumstatEntry) []diffmodel.File {
	binByPaths := map[string]bool{}
	for _, ne := range numstatEntries {
		if !ne.Binary {
			continue
		}
		key := NumstatPathKey(ne)
		binByPaths[key] = true
	}

	out := make([]diffmodel.File, 0, len(rawEntries))
	for _, re := range rawEntries {
		f := diffmodel.File{
			Status:     re.Status,
			PrePath:    re.PrePath,
			PostPath:   re.PostPath,
			BlobBase:   re.SrcSHA,
			BlobHead:   re.DstSHA,
			ModeBase:   re.SrcMode,
			ModeHead:   re.DstMode,
			Similarity: re.Similarity,
		}
		switch re.Status {
		case diffmodel.StatusRenamed, diffmodel.StatusCopied:
			f.RenameFrom = re.PrePath
			f.RenameTo = re.PostPath
		}
		if binByPaths[RawPathKey(re)] {
			f.Binary = true
		}
		out = append(out, f)
	}
	return out
}

// RawPathKey derives the path key from a raw entry that matches the
// numstat-side key (PrePath/PostPath form).
func RawPathKey(e RawEntry) string {
	switch e.Status {
	case diffmodel.StatusRenamed, diffmodel.StatusCopied:
		return e.PrePath + "\x00" + e.PostPath
	case diffmodel.StatusDeleted:
		return "\x00" + e.PrePath
	case diffmodel.StatusAdded:
		return "\x00" + e.PostPath
	default:
		return "\x00" + e.PostPath
	}
}

// NumstatPathKey derives the path key from a numstat entry for binary lookup.
func NumstatPathKey(e NumstatEntry) string {
	if e.PrePath != "" {
		return e.PrePath + "\x00" + e.PostPath
	}
	return "\x00" + e.PostPath
}
