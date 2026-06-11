package review

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
)

const (
	branchPrefixMax = 32
	branchHashLen   = 8
)

// ProjectSlug returns "<safe-basename>__<sha1-8>" for a repository absolute
// path. The basename keeps the directory human-recognisable in `ls
// ~/.sitatame`, while the path hash guarantees that distinct checkouts of the
// same repo (e.g. worktrees or two clones with the same name) get distinct
// per-project directories. When repoAbsPath is empty (defensive), we return
// "project__" + sha1("")[:8] so callers never see a path with two consecutive
// slashes.
func ProjectSlug(repoAbsPath string) string {
	base := filepath.Base(repoAbsPath)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "project"
	}
	prefix := safePrefix(base)
	if prefix == "branch" {
		// safePrefix falls back to "branch" when nothing safe survives; for a
		// project slug "project" reads better.
		prefix = "project"
	}
	sum := sha1.Sum([]byte(repoAbsPath))
	return prefix + "__" + hex.EncodeToString(sum[:])[:branchHashLen]
}

// BranchSlug returns "<safe-prefix>__<sha1-8>" per PRD branch slug rules.
// safe-prefix is the first 32 chars of branch with any byte outside
// [a-zA-Z0-9._-] replaced by '_' (or "branch" if the prefix becomes empty).
// sha1-8 is the first 8 hex chars of sha1(branch UTF-8 bytes).
func BranchSlug(branch string) string {
	prefix := safePrefix(branch)
	sum := sha1.Sum([]byte(branch))
	return prefix + "__" + hex.EncodeToString(sum[:])[:branchHashLen]
}

func safePrefix(branch string) string {
	if branch == "" {
		return "branch"
	}
	head := branch
	if len(head) > branchPrefixMax {
		head = head[:branchPrefixMax]
	}
	buf := make([]byte, len(head))
	hasSafe := false
	for i := 0; i < len(head); i++ {
		c := head[i]
		if isSafeByte(c) {
			buf[i] = c
			hasSafe = true
		} else {
			buf[i] = '_'
		}
	}
	if !hasSafe {
		return "branch"
	}
	return string(buf)
}

func isSafeByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '.' || c == '_' || c == '-':
		return true
	}
	return false
}
