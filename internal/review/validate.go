package review

import (
	"fmt"
	"io"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

// Validate updates each Comment's state by comparing its anchor to the current
// diff. The classification rules are:
//
//   - Anchor blob matches a current file on the same side -> StateOpen.
//     If the path also changed, the anchor's Path / RenameFrom / RenameTo are
//     updated so the UI can render it on the new path (rename-only case).
//   - Path matches but blob differs -> StateStale (rename+edit, or content
//     change at the same path).
//   - Neither blob nor path matches -> StateStale (file gone).
//
// kind == review comments have no anchor and are left unchanged. kind == file
// comments have no line numbers but are validated the same way.
func Validate(r *Review, files []diffmodel.File) {
	ValidateWithWarnings(r, files, nil)
}

// ValidateWithWarnings runs Validate and additionally surfaces legacy-anchor
// warnings (one line per offending comment) on the provided writer. Pass nil
// to suppress warnings — equivalent to Validate.
//
// Legacy anchors are draft comments saved before issues #36 / #19 were fixed:
// the buggy openCommentModal stored Side=head + Line=BaseLine when the user
// commented on a `-` row, producing a record that no consumer can interpret
// correctly. We detect the obvious shape (Side=head + blob matches BlobBase
// of some file) and emit a stderr line so the user notices before re-saving.
// We do NOT silently fix the side — the user's original intent (head vs. base)
// is ambiguous after the fact, and a silent flip would mask a real data
// corruption from the user's review of the draft.
func ValidateWithWarnings(r *Review, files []diffmodel.File, warnings io.Writer) {
	idx := buildDiffIndex(files)
	for i := range r.Comments {
		c := &r.Comments[i]
		if c.Kind == KindReview {
			continue
		}
		if warnings != nil {
			emitLegacyAnchorWarning(&c.Anchor, idx, warnings)
		}
		validateAnchor(&c.Anchor, &c.State, idx)
	}
}

// emitLegacyAnchorWarning detects the issues #36 / #19 legacy-anchor shape and
// writes a single line to w. The detection is intentionally conservative — we
// only flag the case where Side=head but the anchor's Blob is a known
// BlobBase, because that combination is impossible for a correctly-saved
// anchor and indicates the old openCommentModal stored BaseLine under
// Side=head.
func emitLegacyAnchorWarning(a *Anchor, idx diffIndex, w io.Writer) {
	if a.Side != SideHead || a.Blob == "" {
		return
	}
	if _, headHit := idx.headByBlob[a.Blob]; headHit {
		return // blob is a known head blob — anchor is internally consistent.
	}
	if _, baseHit := idx.baseByBlob[a.Blob]; !baseHit {
		return // blob unknown on both sides; nothing actionable.
	}
	id := a.AnchorID
	if id == "" {
		id = "<no-id>"
	}
	fmt.Fprintf(w,
		"sitatame: detected legacy anchor (id=%s, path=%s); side may be incorrect — please re-save.\n",
		id, a.Path)
}

type diffIndex struct {
	headByBlob map[string]diffmodel.File
	baseByBlob map[string]diffmodel.File
	headByPath map[string]diffmodel.File
	baseByPath map[string]diffmodel.File
}

func buildDiffIndex(files []diffmodel.File) diffIndex {
	idx := diffIndex{
		headByBlob: map[string]diffmodel.File{},
		baseByBlob: map[string]diffmodel.File{},
		headByPath: map[string]diffmodel.File{},
		baseByPath: map[string]diffmodel.File{},
	}
	for _, f := range files {
		if f.BlobHead != "" {
			idx.headByBlob[f.BlobHead] = f
		}
		if f.BlobBase != "" {
			idx.baseByBlob[f.BlobBase] = f
		}
		if f.PostPath != "" {
			idx.headByPath[f.PostPath] = f
		}
		if f.PrePath != "" {
			idx.baseByPath[f.PrePath] = f
		}
	}
	return idx
}

func validateAnchor(a *Anchor, state *State, idx diffIndex) {
	byBlob, byPath := idx.headByBlob, idx.headByPath
	pathInDiff := func(f diffmodel.File) string { return f.PostPath }
	originPath := func(f diffmodel.File) string { return f.PrePath }
	if a.Side == SideBase {
		byBlob, byPath = idx.baseByBlob, idx.baseByPath
		pathInDiff = func(f diffmodel.File) string { return f.PrePath }
		originPath = func(f diffmodel.File) string { return f.PostPath }
	}

	if a.Blob != "" {
		if f, ok := byBlob[a.Blob]; ok {
			// Blob matches: anchor still pins the right content, even if the
			// path moved. Update path metadata so the UI overlays on the new
			// location (rename-only).
			newPath := pathInDiff(f)
			if newPath != "" && newPath != a.Path {
				if a.RenameFrom == "" {
					a.RenameFrom = a.Path
				}
				a.RenameTo = newPath
				a.Path = newPath
				if other := originPath(f); other != "" && other != newPath {
					// Surface git-detected rename info too when present.
					a.RenameFrom = other
				}
			}
			*state = StateOpen
			return
		}
	}

	if _, ok := byPath[a.Path]; ok {
		// Same path but different blob: content drifted under us.
		*state = StateStale
		return
	}

	// File no longer in the diff at all.
	*state = StateStale
}
