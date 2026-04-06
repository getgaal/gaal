package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const tickInterval = 80 * time.Millisecond

// spinnerFrames is a braille-style spinner for TTY output.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ttyMu serialises all writes to the TTY (ConsoleHandler + Spinner).
// Both must hold this lock before writing to a shared os.File stderr.
var ttyMu sync.Mutex

// activeSpinner is the currently displayed spinner, or nil.
// Access is guarded by ttyMu.
var activeSpinner *Spinner

// Spinner displays an animated progress indicator on a TTY.
// All methods are nil-safe (no-ops when the receiver is nil).
type Spinner struct {
	out     io.Writer
	label   string
	frame   int
	drawn   bool // a spinner line is currently on screen (guarded by ttyMu)
	stop    chan struct{}
	stopped chan struct{}
}

// StartSpinner creates and starts a spinner for out.
// Returns nil when out is not a TTY or another spinner is already active.
func StartSpinner(out io.Writer, label string) *Spinner {
	if !isTTY(out) {
		return nil
	}

	ttyMu.Lock()
	if activeSpinner != nil {
		// Another spinner is already running; don't stack them.
		ttyMu.Unlock()
		return nil
	}
	s := &Spinner{
		out:     out,
		label:   label,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	activeSpinner = s
	ttyMu.Unlock()

	go s.run()
	return s
}

// Update changes the spinner label while it is running.
func (s *Spinner) Update(label string) {
	if s == nil {
		return
	}
	ttyMu.Lock()
	s.label = label
	ttyMu.Unlock()
}

// Done stops the spinner and prints a ✓ or ✗ final line.
func (s *Spinner) Done(ok bool, label string) {
	if s == nil {
		return
	}
	close(s.stop)

	// Wait for the goroutine with a safety timeout: the goroutine uses
	// TryLock so it always exits promptly, but the timeout guards against
	// any unexpected scheduling delay.
	select {
	case <-s.stopped:
	case <-time.After(500 * time.Millisecond):
	}

	ttyMu.Lock()
	defer ttyMu.Unlock()
	if activeSpinner == s {
		activeSpinner = nil
	}
	// Clear any residual spinner line (goroutine may have skipped via TryLock).
	s.clearLocked()
	if ok {
		fmt.Fprint(s.out, ansiGreen+"  ✓ "+ansiReset+label+"\n")
	} else {
		fmt.Fprint(s.out, ansiRed+"  ✗ "+ansiReset+label+"\n")
	}
}

// Stop stops the spinner silently (no final line printed).
func (s *Spinner) Stop() {
	if s == nil {
		return
	}
	close(s.stop)
	select {
	case <-s.stopped:
	case <-time.After(500 * time.Millisecond):
	}

	ttyMu.Lock()
	if activeSpinner == s {
		activeSpinner = nil
	}
	s.clearLocked()
	ttyMu.Unlock()
}

// clearLocked erases the spinner line. Caller must hold ttyMu.
func (s *Spinner) clearLocked() {
	if s.drawn {
		fmt.Fprintf(s.out, "\r\033[K") // CR + erase to end of line
		s.drawn = false
	}
}

func (s *Spinner) run() {
	defer close(s.stopped)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			// Best-effort clear: TryLock avoids blocking here.
			// Done() will call clearLocked() under the lock as a fallback.
			if ttyMu.TryLock() {
				s.clearLocked()
				ttyMu.Unlock()
			}
			return
		case <-ticker.C:
			// Use TryLock so this goroutine NEVER blocks on the mutex.
			// If the lock is held (e.g. ConsoleHandler is writing to stderr),
			// just skip this frame — the goroutine stays in the select loop
			// and remains responsive to <-s.stop.
			if ttyMu.TryLock() {
				frame := spinnerFrames[s.frame%len(spinnerFrames)]
				s.frame++
				fmt.Fprint(s.out, "\r"+ansiBlue+frame+ansiReset+" "+s.label)
				s.drawn = true
				ttyMu.Unlock()
			}
		}
	}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
