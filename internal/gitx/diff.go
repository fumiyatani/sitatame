package gitx

import (
	"fmt"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/gitx/internal/parser"
)

// DiffSource selects which set of changes Diff should fuse.
type DiffSource int

const (
	// SourceRange compares Base..HEAD (the historical default).
	SourceRange DiffSource = iota
	// SourceStaged compares the index against HEAD (`git diff --cached`).
	SourceStaged
	// SourceWorking compares the working tree against HEAD (`git diff HEAD`).
	// This includes both staged and unstaged edits to tracked files; untracked
	// files are not part of the diff (matching git's own behavior).
	SourceWorking
)

// DiffSpec parameterizes Diff. Base is consulted only when Source == SourceRange.
type DiffSpec struct {
	Source DiffSource
	Base   string
}

// Diff returns the file-level diff for the given spec by fusing three git
// streams: `--raw -z` (file metadata + statuses), `--numstat -z` (binary
// markers), and `--patch --no-color` (hunks).
func (r *Repo) Diff(spec DiffSpec) ([]diffmodel.File, error) {
	srcArgs, err := diffSourceArgs(spec)
	if err != nil {
		return nil, err
	}
	rawOut, err := r.run(diffArgs(srcArgs, "--raw", "-z")...)
	if err != nil {
		return nil, fmt.Errorf("git diff --raw: %w", err)
	}
	numOut, err := r.run(diffArgs(srcArgs, "--numstat", "-z")...)
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w", err)
	}
	patchOut, err := r.run(diffArgs(srcArgs, "--patch", "--no-color")...)
	if err != nil {
		return nil, fmt.Errorf("git diff --patch: %w", err)
	}

	return assembleDiff(rawOut, numOut, patchOut)
}

// assembleDiff fuses the three git diff output streams into a slice of Files.
// Extracted from Diff() so it can be called directly with stub runner output in tests.
func assembleDiff(rawOut, numOut, patchOut string) ([]diffmodel.File, error) {
	rawEntries, err := parser.ParseRawZ(rawOut)
	if err != nil {
		return nil, err
	}
	numEntries, err := parser.ParseNumstatZ(numOut)
	if err != nil {
		return nil, err
	}
	patchEntries, err := parser.ParsePatch(patchOut)
	if err != nil {
		return nil, err
	}

	files := parser.JoinRawAndNumstat(rawEntries, numEntries)

	// Patch entries are addressed by post-image path for non-deletes and by
	// pre-image path for deletes. Index both so the lookup works for either.
	patchByB := make(map[string]parser.PatchEntry, len(patchEntries))
	patchByA := make(map[string]parser.PatchEntry, len(patchEntries))
	for _, pe := range patchEntries {
		if pe.BPath != "" {
			patchByB[pe.BPath] = pe
		}
		if pe.APath != "" {
			patchByA[pe.APath] = pe
		}
	}

	for i := range files {
		f := &files[i]
		var pe parser.PatchEntry
		var ok bool
		switch f.Status {
		case diffmodel.StatusDeleted:
			pe, ok = patchByA[f.PrePath]
		default:
			pe, ok = patchByB[f.PostPath]
			if !ok && f.PrePath != "" {
				pe, ok = patchByA[f.PrePath]
			}
		}
		if !ok {
			continue
		}
		if pe.Binary {
			f.Binary = true
		}
		if !f.Binary {
			f.Hunks = pe.Hunks
			for j := range f.Hunks {
				diffmodel.AssignLineNumbers(&f.Hunks[j])
			}
		}
	}

	return files, nil
}

// diffSourceArgs returns the source-selecting args (range / --cached / HEAD)
// that come AFTER the format flags. Order matters: format flags first, then
// `--find-renames --find-copies`, then these source args last.
func diffSourceArgs(spec DiffSpec) ([]string, error) {
	switch spec.Source {
	case SourceRange:
		if spec.Base == "" {
			return nil, fmt.Errorf("DiffSpec: SourceRange requires Base")
		}
		// `--end-of-options` (git 2.24+) prevents the range from being parsed as a flag.
		return []string{"--end-of-options", spec.Base + "..HEAD"}, nil
	case SourceStaged:
		return []string{"--cached"}, nil
	case SourceWorking:
		return []string{"--end-of-options", "HEAD"}, nil
	default:
		return nil, fmt.Errorf("DiffSpec: unknown Source %d", spec.Source)
	}
}

// diffArgs builds the full git diff arg list: subcommand + format flags +
// rename/copy detection + source selectors.
func diffArgs(src []string, formatFlags ...string) []string {
	out := make([]string, 0, 1+len(formatFlags)+2+len(src))
	out = append(out, "diff")
	out = append(out, formatFlags...)
	out = append(out, "--find-renames", "--find-copies")
	out = append(out, src...)
	return out
}
