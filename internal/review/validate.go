package review

import (
	"github.com/tanifumiya/sitatame/internal/diffmodel"
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
	idx := buildDiffIndex(files)
	for i := range r.Comments {
		c := &r.Comments[i]
		if c.Kind == KindReview {
			continue
		}
		validateAnchor(&c.Anchor, &c.State, idx)
	}
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
