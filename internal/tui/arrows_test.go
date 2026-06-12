package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// Arrow keys are aliases for j / k / n / p. bubbletea reports them via
// tea.KeyMsg.String() as "down" / "up" / "right" / "left", which the
// dispatch in Update matches against the KeyDownArrow etc. constants.
//
// These tests pin the alias mapping for both unified and split layouts:
// the underlying navigation helpers (moveCursorBy, jumpFile, and their
// split twins) are already covered elsewhere, so the assertions here
// stay narrow — same observable outcome as the letter key, no more.

func TestArrowKeys_UnifiedCursorMove(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a.go", []byte{' ', ' ', ' ', ' '})}
	m := setSize(New(files, review.Review{}), 60, 20)

	// Two presses of Down: cursor advances by 2 (skipping any non-movable
	// rows the same way j would — we only assert the +2 delta against the
	// letter-key baseline below).
	mDownArrow := sendNamedKey(sendNamedKey(m, tea.KeyDown), tea.KeyDown)
	mDownLetter := sendKey(sendKey(m, "j"), "j")
	if mDownArrow.Cursor() != mDownLetter.Cursor() {
		t.Fatalf("Down arrow cursor = %d, want same as j = %d", mDownArrow.Cursor(), mDownLetter.Cursor())
	}
	if mDownArrow.Cursor() <= m.Cursor() {
		t.Fatalf("Down arrow did not advance cursor: before=%d after=%d", m.Cursor(), mDownArrow.Cursor())
	}

	// Up arrow reverses one step.
	mUp := sendNamedKey(mDownArrow, tea.KeyUp)
	mUpLetter := sendKey(mDownArrow, "k")
	if mUp.Cursor() != mUpLetter.Cursor() {
		t.Fatalf("Up arrow cursor = %d, want same as k = %d", mUp.Cursor(), mUpLetter.Cursor())
	}
	if mUp.Cursor() >= mDownArrow.Cursor() {
		t.Fatalf("Up arrow did not retreat cursor: before=%d after=%d", mDownArrow.Cursor(), mUp.Cursor())
	}
}

func TestArrowKeys_UnifiedFileJump(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		hunkFile("a.go", []byte{' ', ' '}),
		hunkFile("b.go", []byte{' ', ' '}),
		hunkFile("c.go", []byte{' ', ' '}),
	}
	m := setSize(New(files, review.Review{}), 60, 20)

	// Right arrow jumps to the next file header.
	mRight := sendNamedKey(m, tea.KeyRight)
	if mRight.rows[mRight.Cursor()].kind != rowFileHeader {
		t.Fatalf("Right arrow did not land on file header: kind=%v cursor=%d", mRight.rows[mRight.Cursor()].kind, mRight.Cursor())
	}
	if mRight.rows[mRight.Cursor()].fileIdx != 1 {
		t.Fatalf("Right arrow landed on file %d, want 1", mRight.rows[mRight.Cursor()].fileIdx)
	}

	// Parity with the letter key.
	mNext := sendKey(m, "n")
	if mRight.Cursor() != mNext.Cursor() {
		t.Fatalf("Right arrow cursor = %d, want same as n = %d", mRight.Cursor(), mNext.Cursor())
	}

	// Left arrow walks back. From file 1, Left must end on file 0's header.
	mLeft := sendNamedKey(mRight, tea.KeyLeft)
	if mLeft.rows[mLeft.Cursor()].kind != rowFileHeader {
		t.Fatalf("Left arrow did not land on file header: kind=%v cursor=%d", mLeft.rows[mLeft.Cursor()].kind, mLeft.Cursor())
	}
	if mLeft.rows[mLeft.Cursor()].fileIdx != 0 {
		t.Fatalf("Left arrow landed on file %d, want 0", mLeft.rows[mLeft.Cursor()].fileIdx)
	}

	// Parity with p.
	mPrev := sendKey(mRight, "p")
	if mLeft.Cursor() != mPrev.Cursor() {
		t.Fatalf("Left arrow cursor = %d, want same as p = %d", mLeft.Cursor(), mPrev.Cursor())
	}
}

func TestArrowKeys_SplitCursorMove(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a.go", []byte{' ', ' ', ' ', ' '})}
	m := setSize(New(files, review.Review{}), 60, 20)
	m = enterSplit(t, m)

	beforeArrow := m.splitCursor
	mDown := sendNamedKey(sendNamedKey(m, tea.KeyDown), tea.KeyDown)
	if mDown.layout != LayoutSplit {
		t.Fatalf("layout flipped off split: %v", mDown.layout)
	}
	if mDown.splitCursor <= beforeArrow {
		t.Fatalf("Down arrow did not advance splitCursor: before=%d after=%d", beforeArrow, mDown.splitCursor)
	}
	mDownLetter := sendKey(sendKey(m, "j"), "j")
	if mDown.splitCursor != mDownLetter.splitCursor {
		t.Fatalf("Down arrow splitCursor = %d, want same as j = %d", mDown.splitCursor, mDownLetter.splitCursor)
	}

	mUp := sendNamedKey(mDown, tea.KeyUp)
	mUpLetter := sendKey(mDown, "k")
	if mUp.splitCursor != mUpLetter.splitCursor {
		t.Fatalf("Up arrow splitCursor = %d, want same as k = %d", mUp.splitCursor, mUpLetter.splitCursor)
	}
}

func TestArrowKeys_SplitFileJump(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		hunkFile("a.go", []byte{' ', ' '}),
		hunkFile("b.go", []byte{' ', ' '}),
	}
	m := setSize(New(files, review.Review{}), 60, 20)
	m = enterSplit(t, m)

	mRight := sendNamedKey(m, tea.KeyRight)
	mNext := sendKey(m, "n")
	if mRight.splitCursor != mNext.splitCursor {
		t.Fatalf("Right arrow splitCursor = %d, want same as n = %d", mRight.splitCursor, mNext.splitCursor)
	}

	mLeft := sendNamedKey(mRight, tea.KeyLeft)
	mPrev := sendKey(mRight, "p")
	if mLeft.splitCursor != mPrev.splitCursor {
		t.Fatalf("Left arrow splitCursor = %d, want same as p = %d", mLeft.splitCursor, mPrev.splitCursor)
	}
}
