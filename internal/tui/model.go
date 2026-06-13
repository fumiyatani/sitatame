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

	// filePicker is the modal state for the `f` jump-to-file overlay. nil
	// when closed; non-nil drives a dedicated Update path (updateFilePicker)
	// and replaces the diff view with filePickerView. Kept as a separate
	// field from `modal` because the textarea modal has its own state shape
	// and confirm/cancel semantics, and forcing both through a single union
	// would just push the kind switch into every helper.
	filePicker *filePicker

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
	if m.filePicker != nil {
		// File picker has its own narrow key set (j/k, up/down, enter, esc);
		// the diff's normal binding table is suppressed while it's open so
		// `q` etc. don't quit out from under the user. WindowSize still
		// flows through so the picker height stays in sync.
		return m.updateFilePicker(msg)
	}
	if m.modal != nil {
		cmd := m.updateModal(msg)
		return m, cmd
	}
	cmd := m.updateMain(msg)
	return m, cmd
}

// updateMain handles the main diff-view branch of Update — every message
// path that runs when neither the file picker nor a textarea modal is open.
// Mutates the receiver and returns the tea.Cmd to forward; callers in Update
// wrap it back into the (tea.Model, tea.Cmd) shape bubbletea expects.
//
// Extracted from Update so the dispatcher at the top stays a flat read of
// "picker → modal → main", matching the other two updateXxx helpers
// (updateFilePicker, updateModal) that already follow this split.
func (m *Model) updateMain(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollToCursor()
		if m.layout == LayoutSplit {
			m.scrollSplitToCursor()
		}
		return nil
	case tea.MouseMsg:
		// Help is rendered as a full-screen overlay, so silently scrolling
		// the diff behind it would be invisible until the user closes help —
		// a stealth state change. Drop wheel events while help is up.
		if m.showHelp {
			return nil
		}
		// Only wheel-press events scroll; releases, drags, and other buttons
		// are ignored so we don't fight the natural single-tick model.
		if msg.Action != tea.MouseActionPress {
			return nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.layout == LayoutSplit {
				m.scrollSplitViewportBy(-mouseWheelStep)
			} else {
				m.scrollViewportBy(-mouseWheelStep)
			}
		case tea.MouseButtonWheelDown:
			if m.layout == LayoutSplit {
				m.scrollSplitViewportBy(mouseWheelStep)
			} else {
				m.scrollViewportBy(mouseWheelStep)
			}
		case tea.MouseButtonLeft:
			m.handleLeftClick(msg.Y)
		}
		return nil
	case tea.KeyMsg:
		// Any key clears the previous transient message; guards below re-set
		// it for the keys we intercept in split mode.
		m.statusMsg = ""
		switch msg.String() {
		case KeyQuit, KeyQuitCtrl:
			m.quitting = true
			m.quitReason = QuitDraft
			return tea.Quit
		case KeySave:
			m.quitting = true
			m.quitReason = QuitPromote
			return tea.Quit
		case KeyHelp:
			m.showHelp = !m.showHelp
			return nil
		case KeyEsc:
			if m.showHelp {
				m.showHelp = false
			}
			m.clearSelection()
			return nil
		case KeyToggleLayout:
			m.toggleLayout()
			return nil
		case KeyDown, KeyDownArrow:
			if m.layout == LayoutSplit {
				m.moveSplitCursorBy(1)
			} else {
				m.moveCursorBy(1)
				m.extendSelection()
			}
			return nil
		case KeyUp, KeyUpArrow:
			if m.layout == LayoutSplit {
				m.moveSplitCursorBy(-1)
			} else {
				m.moveCursorBy(-1)
				m.extendSelection()
			}
			return nil
		case KeyNextFile, KeyRightArrow:
			if m.layout == LayoutSplit {
				m.jumpSplitFile(1)
			} else {
				m.jumpFile(1)
				m.clearSelection()
			}
			return nil
		case KeyPrevFile, KeyLeftArrow:
			if m.layout == LayoutSplit {
				m.jumpSplitFile(-1)
			} else {
				m.jumpFile(-1)
				m.clearSelection()
			}
			return nil
		case KeySelectKey:
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return nil
			}
			m.startSelection()
			return nil
		case "c":
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return nil
			}
			m.openCommentModal()
			return nil
		case "R":
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return nil
			}
			m.openReviewModal()
			return nil
		case KeyResolveToggle:
			if m.layout == LayoutSplit {
				m.statusMsg = previewOnlyMsg
				return nil
			}
			m.toggleResolvedAtCursor()
			return nil
		case KeyFilePicker:
			if m.showHelp {
				// Help is a full-screen overlay; popping a second modal on top
				// would compete for the same cells and leave the user without
				// any signal that help is still "behind" the picker. Ignore
				// the key — the user can press `?` to dismiss help first.
				return nil
			}
			if m.layout == LayoutSplit {
				// File picker would land on a unified row index that the
				// split cursor can't consume without translation. Guard it
				// behind the same preview-only banner as the other unified
				// actions until split learns its own jump path.
				m.statusMsg = previewOnlyMsg
				return nil
			}
			m.openFilePicker()
			return nil
		}
	}
	return nil
}

// resolveTarget picks which comment on `row` the next `x` press would flip,
// matching the priority used by toggleResolvedAtCursor:
//
//  1. If a previous `x` on this row toggled anchor X and X is still present
//     in a toggleable state (open/resolved), X is chosen — the undo path.
//  2. Otherwise the open-biased default: any open comment exists → the last
//     open one, else the last resolved one.
//
// Stale comments are skipped entirely. Returns (-1, false) for empty
// overlays, stale-only rows, or rows where the overlay indexes nothing valid.
//
// Shared between toggleResolvedAtCursor (which mutates) and the hint label
// (which only describes) so the hint never disagrees with what `x` will do.
func (m Model) resolveTarget(row int) (int, bool) {
	idxs := m.overlay[row]
	if len(idxs) == 0 {
		return -1, false
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

	switch {
	case stickyIdx >= 0:
		return stickyIdx, true
	case lastOpen >= 0:
		return lastOpen, true
	case lastResolved >= 0:
		return lastResolved, true
	}
	return -1, false
}

// toggleResolvedAtCursor flips the state of the comment anchored at the
// cursor row between open and resolved. Target selection is delegated to
// resolveTarget so the hint label and the action stay in lock-step.
//
// On a successful toggle the status bar echoes `resolved: <anchor_id>` or
// `reopened: <anchor_id>` so the reviewer can see which anchor was touched
// even when several comments share the row, and `lastToggledAnchor` is
// updated so a subsequent `x` re-targets the same comment.
func (m *Model) toggleResolvedAtCursor() {
	target, ok := m.resolveTarget(m.cursor)
	if !ok {
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
	if m.filePicker != nil {
		return filePickerView(m)
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

// FilePicker returns the open file picker or nil. Test-only accessor.
func (m Model) FilePicker() *filePicker { return m.filePicker }

// statusBarRows is the height of the chrome above the diff viewport. View()
// emits the status bar on Y=0 and starts diff rows at Y=1, so any click with
// msg.Y < statusBarRows is on chrome and a click with
// msg.Y >= statusBarRows + viewportHeight() is on the hint line / padding.
const statusBarRows = 1

// handleLeftClick maps a click Y coordinate to a row index and moves the
// cursor (unified) or splitCursor (split) there. Clicks landing on chrome
// (status bar / hint line) or past the last rendered row are silently
// dropped — there's nothing meaningful to select on those rows and a no-op
// is less surprising than snapping to the nearest valid line.
func (m *Model) handleLeftClick(y int) {
	row := y - statusBarRows
	if row < 0 || row >= m.viewportHeight() {
		return
	}
	if m.layout == LayoutSplit {
		idx := m.splitTop + row
		if idx < 0 || idx >= len(m.splitRows) {
			return
		}
		if idx == m.splitCursor {
			return
		}
		m.splitCursor = idx
		m.invalidateLastToggle()
		m.refreshSplitPreferredSide()
		m.scrollSplitToCursor()
		return
	}
	idx := m.top + row
	if idx < 0 || idx >= len(m.rows) {
		return
	}
	if idx == m.cursor {
		return
	}
	m.cursor = idx
	m.invalidateLastToggle()
	m.scrollToCursor()
	if m.selection != nil {
		m.extendSelection()
	}
}
