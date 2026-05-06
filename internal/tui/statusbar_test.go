package tui

import (
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

func TestStatusBar_BinaryHint(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		{Status: diffmodel.StatusModified, PrePath: "a.go", PostPath: "a.go", Binary: false,
			Hunks: []diffmodel.Hunk{{
				BaseStart: 1, HeadStart: 1, BaseLines: 1, HeadLines: 1,
				Lines: []diffmodel.Line{{Prefix: ' ', Text: "x"}},
			}}},
		{Status: diffmodel.StatusModified, PrePath: "img.bin", PostPath: "img.bin", Binary: true},
	}
	m := New(files, review.Review{})

	// Cursor on file A header — status bar should NOT carry the file-comment hint.
	if got := m.View(); strings.Contains(got, "file-comment only") {
		t.Errorf("status hint should be absent on non-binary file: %q", got)
	}

	// Jump to file B (binary) — tag should appear.
	m, _ = applyKey(m, "n")
	if got := m.View(); !strings.Contains(got, "[binary] file-comment only") {
		t.Errorf("binary tag missing: %q", got)
	}
}

func TestHintLine_RangeModeIndicator(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := setSize(New(files, review.Review{}), 60, 12)

	if strings.Contains(m.View(), "-- RANGE --") {
		t.Errorf("RANGE tag should be absent before r is pressed")
	}

	// Move into a content row, enter range mode.
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // first content line
	m, _ = applyKey(m, "r")

	if !strings.Contains(m.View(), "-- RANGE --") {
		t.Errorf("RANGE tag missing after r: %q", m.View())
	}

	// Esc clears selection — tag should disappear.
	m, _ = applyKey(m, "esc")
	if strings.Contains(m.View(), "-- RANGE --") {
		t.Errorf("RANGE tag should clear with Esc: %q", m.View())
	}
}

func TestStatusBar_FileCount(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		{Status: diffmodel.StatusModified, PrePath: "a", PostPath: "a"},
		{Status: diffmodel.StatusAdded, PostPath: "b"},
		{Status: diffmodel.StatusDeleted, PrePath: "c"},
	}
	m := New(files, review.Review{})
	if got := m.View(); !strings.Contains(got, "[1/3 files]") {
		t.Errorf("file count missing: %q", got)
	}
}
