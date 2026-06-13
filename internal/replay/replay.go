//go:build tui_e2e

// Package replay drives the compiled sitatame binary inside a pseudo-terminal
// and re-builds the rendered screen via a VT100 emulator so smoke tests can
// assert on what a real user would see.
//
// The package is test-only in spirit: it exposes constructors that take a
// *testing.T and skip when the host kernel refuses to hand out a pty (sandbox,
// container without /dev/ptmx, etc.). It deliberately stays in internal/ so
// non-test packages do not depend on it.
//
// The whole package compiles only under the `tui_e2e` build tag so the default
// `go test ./...` run stays fast and self-contained. Use
// `make test-tui-e2e` (or `go test -tags tui_e2e ./...`) to exercise it.
package replay

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// Session is a running sitatame binary driven through a pty. Term holds the
// virtual terminal that mirrors what the program drew; tests typically poll
// Screen() / WaitFor() to assert on visible state.
type Session struct {
	Cols, Rows int
	Env        []string
	Term       vt10x.Terminal

	t        *testing.T
	pty      *os.File
	cmd      *exec.Cmd
	stdoutMu sync.Mutex
	stdout   strings.Builder

	waitOnce sync.Once
	waitErr  error
	exitCode int32

	// pumpDone is closed by the pump goroutine once the pty reader has drained
	// (typically when the child process exits and the master returns EOF).
	// WaitForExit blocks on it so callers see a fully-flushed Stdout(); see the
	// API contract on WaitForExit.
	pumpDone chan struct{}

	closeOnce sync.Once
}

// ptyAvailable probes whether the host kernel will hand us a pty. Sandboxes
// (notably macOS Seatbelt with restricted /dev/ptmx access) return EPERM here;
// rather than failing the whole test binary we let callers Skip cleanly.
var ptyAvailable = func() bool {
	master, slave, err := pty.Open()
	if err != nil {
		return false
	}
	_ = master.Close()
	_ = slave.Close()
	return true
}()

// SkipIfNoPTY is a one-line helper for test callers: skip the current test if
// the sandbox cannot give us a pty. Centralising the message keeps the skip
// reason consistent across smoke tests.
func SkipIfNoPTY(t *testing.T) {
	t.Helper()
	if !ptyAvailable {
		t.Skip("replay: host kernel does not permit pty allocation (sandbox?)")
	}
}

// Start launches binPath inside a pty, with cwd set to repoDir, and wires its
// output into a 80x24 vt10x terminal. It is the caller's responsibility to
// arrange a temporary git repository under repoDir before calling.
//
// The returned Session has a background goroutine copying pty output into the
// virtual terminal; tests must call Close before the test exits to avoid leaks.
// t.Cleanup is wired automatically.
func Start(t *testing.T, binPath string, repoDir string, args ...string) *Session {
	t.Helper()
	SkipIfNoPTY(t)

	const (
		cols = 120
		rows = 40
	)

	cmd := exec.Command(binPath, args...)
	cmd.Dir = repoDir
	// Inherit the caller's environment so $PATH (for `git`) is available, then
	// allow callers to override entries via Session.Env before Start? Not
	// exposed today; tests set SITATAME_HOME via os.Setenv on the test process
	// because cmd.Env is wholesale-replace and listing every var is noisy.
	cmd.Env = os.Environ()

	ws := &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}
	master, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		t.Fatalf("replay: pty.StartWithSize: %v", err)
	}

	term := vt10x.New(vt10x.WithSize(cols, rows))

	s := &Session{
		Cols:     cols,
		Rows:     rows,
		Term:     term,
		t:        t,
		pty:      master,
		cmd:      cmd,
		pumpDone: make(chan struct{}),
	}

	// Pump pty output into the virtual terminal. We tee into stdout so tests
	// can read post-exit stdout content (e.g. SITATAME_REVIEW=... line printed
	// after the alt screen is torn down) without the vt10x scrollback.
	go s.pump()

	t.Cleanup(func() {
		_, _ = s.Close()
	})

	return s
}

func (s *Session) pump() {
	// Signal WaitForExit (and any other sync points) that we have drained the
	// pty reader. Closing on return covers both the EOF path (child exited and
	// the master returned io.EOF) and any unexpected read error.
	defer close(s.pumpDone)
	br := bufio.NewReader(s.pty)
	buf := make([]byte, 4096)
	// leftover holds bytes that vt10x refused on the previous iteration because
	// they were the prefix of an incomplete UTF-8 rune (vt10x.Write returns a
	// short write of written-1 in that case — see vt_posix.go). Without
	// carrying those bytes forward, the em-dashes and box drawing characters
	// used by the help modal and split layout get truncated and screen
	// comparisons go flaky. stdout always receives the raw bytes because the
	// underlying strings.Builder never short-writes, so the two sinks stay in
	// lockstep on a per-chunk basis.
	var leftover []byte
	for {
		n, err := br.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			s.stdoutMu.Lock()
			s.stdout.Write(chunk)
			s.stdoutMu.Unlock()

			// Prepend any rune-prefix bytes that vt10x rejected last round.
			data := chunk
			if len(leftover) > 0 {
				data = append(leftover, chunk...)
			}
			// vt10x.Terminal embeds io.Writer; Write parses sequences into screen state.
			nw, werr := s.Term.Write(data)
			if werr != nil {
				return
			}
			// Anything vt10x did not consume is a partial rune — keep it for
			// the next iteration. `rest` may alias either buf (when leftover
			// was empty) or leftover's own backing array (when we appended on
			// top of it), so copy into a fresh slice to avoid the next read
			// trampling those bytes.
			if nw < len(data) {
				rest := data[nw:]
				next := make([]byte, len(rest))
				copy(next, rest)
				leftover = next
			} else {
				leftover = leftover[:0]
			}
		}
		if err != nil {
			return
		}
	}
}

// Send writes input bytes straight into the pty master. Symbolic key handling
// (e.g. "<esc>" -> 0x1b) is provided as a thin convenience so call sites can
// stay readable; everything else is sent verbatim.
//
// Recognised tokens (case-insensitive): "<esc>", "<enter>", "<tab>", "<ctrl+s>",
// "<ctrl+c>". Multiple tokens / literal chars can be mixed.
func (s *Session) Send(input string) {
	s.t.Helper()
	out := expandKeys(input)
	if _, err := s.pty.Write([]byte(out)); err != nil {
		s.t.Fatalf("replay: write to pty: %v", err)
	}
}

func expandKeys(input string) string {
	// Cheap manual scanner — we keep this dependency-free and the token set
	// small so callers can spot mistakes by reading the constant table.
	replacements := []struct{ in, out string }{
		{"<esc>", "\x1b"},
		{"<ESC>", "\x1b"},
		{"<enter>", "\r"},
		{"<ENTER>", "\r"},
		{"<tab>", "\t"},
		{"<TAB>", "\t"},
		{"<ctrl+s>", "\x13"},
		{"<CTRL+S>", "\x13"},
		{"<ctrl+c>", "\x03"},
		{"<CTRL+C>", "\x03"},
	}
	out := input
	for _, r := range replacements {
		out = strings.ReplaceAll(out, r.in, r.out)
	}
	return out
}

// WaitFor polls the virtual terminal screen until substring appears, or until
// timeout elapses. Returns nil on success and a descriptive error otherwise so
// callers can include the substring in their failure message.
func (s *Session) WaitFor(substring string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if strings.Contains(s.Screen(), substring) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("replay: timeout waiting for " + substring)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// WaitForScreenChange polls until Screen() differs from prev, or until
// timeout elapses. This is the right primitive for asserting on a re-render
// triggered by a side effect (SIGWINCH, async paint) where WaitFor would
// short-circuit on substrings that survived from the previous frame.
//
// Callers should snapshot prev with Screen() immediately before triggering the
// change so the diff window is as narrow as possible.
func (s *Session) WaitForScreenChange(prev string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if s.Screen() != prev {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("replay: timeout waiting for screen change")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// WaitForExit blocks until the child exits AND the pty reader drains, or
// until timeout elapses. Returns the exit code; callers can also read Stdout
// afterwards.
//
// API contract: once WaitForExit returns nil, Stdout() reflects every byte the
// child wrote into the pty, including bytes emitted just before exit (e.g. the
// SITATAME_REVIEW=<path> line printed after the alt screen is torn down).
// Without the drain step there is a race where cmd.Wait() returns before the
// pump goroutine finishes copying the final chunk into the stdout buffer, which
// makes assertions on such trailing lines flaky.
func (s *Session) WaitForExit(timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)

	done := make(chan error, 1)
	go func() {
		s.waitOnce.Do(func() {
			s.waitErr = s.cmd.Wait()
			if ee, ok := s.waitErr.(*exec.ExitError); ok {
				atomic.StoreInt32(&s.exitCode, int32(ee.ExitCode()))
			} else if s.waitErr == nil {
				atomic.StoreInt32(&s.exitCode, int32(s.cmd.ProcessState.ExitCode()))
			}
		})
		done <- s.waitErr
	}()
	select {
	case <-done:
	case <-time.After(time.Until(deadline)):
		return -1, errors.New("replay: timeout waiting for child exit")
	}

	// Child has exited; the pty master should return EOF imminently, which will
	// drive pump() to return and close pumpDone. We still bound the wait so a
	// stuck reader does not hang the test indefinitely.
	select {
	case <-s.pumpDone:
	case <-time.After(time.Until(deadline)):
		return -1, errors.New("replay: timeout waiting for pty reader to drain")
	}
	return int(atomic.LoadInt32(&s.exitCode)), nil
}

// Screen renders the virtual terminal to a single newline-separated string.
// Trailing whitespace on each row is preserved so callers can match on padded
// status lines if they need to.
func (s *Session) Screen() string {
	s.Term.Lock()
	defer s.Term.Unlock()
	cols, rows := s.Term.Size()
	var b strings.Builder
	b.Grow(rows * (cols + 1))
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			g := s.Term.Cell(x, y)
			if g.Char == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(g.Char)
		}
		if y < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Resize updates both the pty winsize (delivering SIGWINCH to the child) and
// the virtual terminal so subsequent Screen() calls reflect the new geometry.
func (s *Session) Resize(cols, rows int) error {
	if err := pty.Setsize(s.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}); err != nil {
		return err
	}
	s.Term.Resize(cols, rows)
	s.Cols, s.Rows = cols, rows
	return nil
}

// Stdout returns the entire byte stream the program emitted into the pty so
// far. Useful for asserting on post-exit output such as the
// SITATAME_REVIEW=<path> line printed after the alt screen is torn down.
//
// The return value is only guaranteed to contain every emitted byte after
// WaitForExit (or Close) returns successfully; mid-run callers may observe a
// prefix because the pump goroutine writes asynchronously.
func (s *Session) Stdout() string {
	s.stdoutMu.Lock()
	defer s.stdoutMu.Unlock()
	return s.stdout.String()
}

// Close closes the pty (which delivers EOF / SIGHUP to the child) and returns
// the child's exit code. Safe to call from t.Cleanup even after WaitForExit.
func (s *Session) Close() (int, error) {
	var code int
	var err error
	s.closeOnce.Do(func() {
		_ = s.pty.Close()
		// Give the child a moment to flush on its own; if it does not, kill.
		done := make(chan struct{})
		go func() {
			s.waitOnce.Do(func() {
				s.waitErr = s.cmd.Wait()
				if ee, ok := s.waitErr.(*exec.ExitError); ok {
					atomic.StoreInt32(&s.exitCode, int32(ee.ExitCode()))
				} else if s.waitErr == nil && s.cmd.ProcessState != nil {
					atomic.StoreInt32(&s.exitCode, int32(s.cmd.ProcessState.ExitCode()))
				}
			})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = s.cmd.Process.Kill()
			<-done
		}
		// Wait for the pump goroutine to finish so Stdout() reflects every byte
		// the child wrote. pty.Close() above guarantees the master read returns
		// promptly, so this is essentially instant in the happy path.
		select {
		case <-s.pumpDone:
		case <-time.After(1 * time.Second):
		}
		code = int(atomic.LoadInt32(&s.exitCode))
		if s.waitErr != nil {
			if _, ok := s.waitErr.(*exec.ExitError); !ok && !errors.Is(s.waitErr, io.EOF) {
				err = s.waitErr
			}
		}
	})
	return code, err
}
