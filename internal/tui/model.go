package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// QuitReason carries why the TUI exited so the caller can decide what to do
// with the in-memory review. The runner exposes the final reason via the
// Model's QuitReason() accessor after tea.Quit.
type QuitReason int

const (
	// QuitNone is the initial value before the user has chosen to leave. The
	// runner treats it like QuitDraft: any uncommitted in-memory review should
	// still be persisted as a draft so work isn't lost on unexpected exits.
	QuitNone QuitReason = iota
	// QuitDraft is set by `q`; the caller saves the review to drafts/ and
	// returns a non-zero exit code.
	QuitDraft
	// QuitPromote is set by `s`; the caller saves a draft, atomically promotes
	// it to reviews/, and prints `SITATAME_REVIEW=<abs>` on stdout.
	QuitPromote
)

// Model is the bubbletea model for the diff review TUI.
//
// T11 covers the skeleton: window-size handling, q to quit, ? to toggle help.
// Navigation, selection, comment modal, and save flow arrive in T12+.
type Model struct {
	Files  []diffmodel.File
	Review review.Review

	rows       []row
	overlay    map[int][]int
	lnBaseW    int
	lnHeadW    int
	cursor     int
	top        int
	width      int
	height     int
	showHelp   bool
	quitting   bool
	quitReason QuitReason
	selection  *Selection
	modal      *modal

	layout             LayoutMode
	splitRows          []splitRow
	splitOverlay       map[int]splitOverlayEntry
	splitCursor        int
	splitTop           int
	splitPreferredSide review.Side

	// statusMsg is a transient message shown at the trailing edge of the
	// status bar (currently used to surface "split is preview-only" when
	// the user tries to comment / select in split mode). Cleared on the
	// next key press.
	statusMsg string

	// lastToggledAnchor remembers the anchor_id touched by the most recent
	// `x` press so a follow-up `x` on the same row undoes that exact
	// comment instead of falling back to "last open / last resolved" — the
	// latter silently mutates an unrelated neighbor when the row hosts
	// `[open A, resolved B]`. Cleared whenever the cursor moves so a fresh
	// row starts back at the open-biased default.
	lastToggledAnchor string
}

const previewOnlyMsg = "split is preview-only — press Tab to return"

func New(files []diffmodel.File, r review.Review) Model {
	rows := buildRows(files)
	bw, hw := lineNumberWidths(files)
	return Model{
		Files:   files,
		Review:  r,
		rows:    rows,
		overlay: buildOverlay(rows, files, r.Comments),
		lnBaseW: bw,
		lnHeadW: hw,
		// Reasonable default until the first WindowSizeMsg arrives so View()
		// before the first resize still produces a non-empty body.
		height: 24,
		width:  80,
	}
}

// Overlay returns the row-to-comment-index map. Test-only accessor.
func (m Model) Overlay() map[int][]int { return m.overlay }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		cmd := m.updateModal(msg)
		return m, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollToCursor()
		if m.layout == LayoutSplit {
			m.scrollSplitToCursor()
		}
		return m, nil
	case tea.KeyMsg:
		// Any key clears the previous transient message; guards below re-set
		// it for the keys we intercept in split mode.
		m.statusMsg = ""
		switch msg.String() {
		case KeyQuit, KeyQuitCtrl:
			m.quitting = true
			m.quitReason = QuitDraft
			return m, tea.Quit
		case KeySave:
			m.quitting = true
			m.quitReason = QuitPromote
			return m, tea.Quit
		case KeyHelp:
			m.showHelp = !m.showHelp
			return m, nil
		case KeyEsc:
			if m.showHelp {
				m.showHelp = false
			}
			m.clearSelection()
			return m, nil
		case KeyToggleLayout:
			m.toggleLayout()
			return m, nil
		case KeyDown:
			if m.layout == LayoutSplit {
				m.moveSplitCursorBy(1)
			} else {
				m.moveCursorBy(1)
				m.extendSelection()
			}
			return m, nil
		case KeyUp:
			if m.layout == LayoutSplit {
				m.moveSplitCursorBy(-1)
			} else {
				m.moveCursorBy(-1)
				m.extendSelection()
			}
			return m, nil
		case KeyNextFile:
			if m.layout == LayoutSplit {
				m.jumpSplitFile(1)
			} else {
				m.jumpFile(1)
				m.clearSelection()
			}
			return m, nil
		case KeyPrevFile:
			if m.layout == LayoutSplit {
				m.jumpSplitFile(-1)
			} else {
				m.jumpFile(-1)
				m.clearSelection()
			}
			return m, nil
		case KeySelectKey:
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return m, nil
			}
			m.startSelection()
			return m, nil
		case "c":
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return m, nil
			}
			m.openCommentModal()
			return m, nil
		case "R":
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return m, nil
			}
			m.openReviewModal()
			return m, nil
		case KeyResolveToggle:
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return m, nil
			}
			m.toggleResolvedAtCursor()
			return m, nil
		}
	}
	return m, nil
}

// toggleResolvedAtCursor flips the state of the comment anchored at the
// cursor row between open and resolved. Target selection has two tiers:
//
//  1. If a previous `x` on this row toggled anchor X and X is still on the
//     row, X is flipped again. This is the undo path: pressing `x` twice in
//     a row on the same line must touch the *same* comment so a row of
//     `[open A, resolved B]` doesn't silently mutate B on the second press.
//  2. Otherwise the open-biased default applies:
//     - any open comment exists → the last open one is resolved
//     - only resolved comments remain → the last resolved one is reopened
//
// Stale comments are ignored entirely — the underlying code has drifted
// and silently resolving them would hide a follow-up.
//
// On a successful toggle the status bar echoes `resolved: <anchor_id>` or
// `reopened: <anchor_id>` so the reviewer can see which anchor was touched
// even when several comments share the row, and `lastToggledAnchor` is
// updated so a subsequent `x` re-targets the same comment.
func (m *Model) toggleResolvedAtCursor() {
	idxs := m.overlay[m.cursor]
	if len(idxs) == 0 {
		return
	}

	stickyIdx := -1
	lastOpen := -1
	lastResolved := -1
	for _, idx := range idxs {
		if idx < 0 || idx >= len(m.Review.Comments) {
			continue
		}
		c := m.Review.Comments[idx]
		if m.lastToggledAnchor != "" && c.AnchorID == m.lastToggledAnchor {
			// Only open/resolved comments are toggleable; stale stays out
			// of the sticky path too.
			if c.State == review.StateOpen || c.State == review.StateResolved {
				stickyIdx = idx
			}
		}
		switch c.State {
		case review.StateOpen:
			lastOpen = idx
		case review.StateResolved:
			lastResolved = idx
		}
		// StateStale: ignored on purpose.
	}

	target := -1
	switch {
	case stickyIdx >= 0:
		target = stickyIdx
	case lastOpen >= 0:
		target = lastOpen
	case lastResolved >= 0:
		target = lastResolved
	}
	if target < 0 {
		return
	}

	switch m.Review.Comments[target].State {
	case review.StateOpen:
		m.Review.Comments[target].State = review.StateResolved
		m.statusMsg = "resolved: " + m.Review.Comments[target].AnchorID
	case review.StateResolved:
		m.Review.Comments[target].State = review.StateOpen
		m.statusMsg = "reopened: " + m.Review.Comments[target].AnchorID
	}
	m.lastToggledAnchor = m.Review.Comments[target].AnchorID
}

// invalidateLastToggle drops the sticky resolve anchor so the next `x`
// press falls back to the open-biased default. Centralised so every
// cursor/layout mutation path can call a single helper instead of touching
// the field directly — past regressions (split nav, Tab toggle) all stemmed
// from forgetting to clear it on a new mutation path.
func (m *Model) invalidateLastToggle() { m.lastToggledAnchor = "" }

// Cursor returns the current row index (test-only accessor).
func (m Model) Cursor() int { return m.cursor }

// Top returns the current scroll position (test-only accessor).
func (m Model) Top() int { return m.top }

// Rows returns the number of flat rows (test-only accessor).
func (m Model) Rows() int { return len(m.rows) }

// SelectionState returns the current selection or nil.
func (m Model) SelectionState() *Selection { return m.selection }

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.showHelp {
		return helpView()
	}
	if m.modal != nil {
		return modalView(m)
	}
	if m.layout == LayoutSplit {
		return mainViewSplit(m)
	}
	return mainView(m)
}

// Quitting reports whether the model has signaled tea.Quit. Exposed for tests.
func (m Model) Quitting() bool { return m.quitting }

// QuitReason returns why the model asked to quit. Defaults to QuitNone before
// any quit key is pressed.
func (m Model) QuitReason() QuitReason { return m.quitReason }

// ShowingHelp reports whether the help modal is currently visible. Exposed for tests.
func (m Model) ShowingHelp() bool { return m.showHelp }
