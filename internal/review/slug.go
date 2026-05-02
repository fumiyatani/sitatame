package review

import (
	"crypto/sha1"
	"encoding/hex"
)

const (
	branchPrefixMax = 32
	branchHashLen   = 8
)

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
