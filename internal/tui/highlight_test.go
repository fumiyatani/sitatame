package tui

import (
	"strings"
	"testing"
)

func TestHasComment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []int
		want bool
	}{
		{"nil", nil, false},
		{"empty", []int{}, false},
		{"one", []int{0}, true},
		{"many", []int{2, 5, 9}, true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := hasComment(c.in); got != c.want {
				t.Fatalf("hasComment(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestApplyCommentHighlight_WrapsWithSGR(t *testing.T) {
	t.Parallel()
	got := applyCommentHighlight("42")
	if !strings.HasPrefix(got, commentColorFG) {
		t.Errorf("missing leading SGR: %q", got)
	}
	if !strings.HasSuffix(got, commentReset) {
		t.Errorf("missing trailing reset: %q", got)
	}
	// stripANSI で素の中身が復元できることもセットで確認する。
	if stripped := stripANSI(got); stripped != "42" {
		t.Errorf("payload corrupted: stripANSI(%q) = %q, want %q", got, stripped, "42")
	}
}

func TestApplyCommentHighlight_EmptyPassthrough(t *testing.T) {
	t.Parallel()
	if got := applyCommentHighlight(""); got != "" {
		t.Errorf("empty should pass through, got %q", got)
	}
}
