package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// requestTimeout bounds every API call; full logs of long tasks are the slowest.
const requestTimeout = 60 * time.Second

// screen is one level of the navigation stack. Screens are pointers and
// mutate in place. App sends keys to the top screen, results only to the
// screen that asked for them, and everything else to every screen.
type screen interface {
	init() tea.Cmd
	update(msg tea.Msg) tea.Cmd
	view() string
	title() string
	help() string
	// capturing is true while a text input has focus, so q, esc and ? are
	// typed into it instead of being handled by App.
	capturing() bool
	// busy is true while the screen waits for Airflow; App shows a spinner.
	busy() bool
	// close cancels the screen's in-flight requests once it is left.
	close()
}

// base ties a screen's requests to its lifetime: they run under ctx, which
// is cancelled when the screen is closed. Screens opened from this one
// derive their ctx from it. seq numbers the screen's loads, so an answer to
// a load that a newer one has replaced can be told apart and dropped.
type base struct {
	ctx    context.Context
	cancel context.CancelFunc
	seq    int
}

func newBase(parent context.Context) base {
	ctx, cancel := context.WithCancel(parent)
	return base{ctx: ctx, cancel: cancel}
}

func (b *base) close() { b.cancel() }

func (b *base) nextSeq() int {
	b.seq++
	return b.seq
}

// result is the answer to one request, addressed to the screen that made it.
type result[T any] struct {
	to  screen
	seq int
	val T
	err error
}

func (r result[T]) target() screen { return r.to }

// addressed is implemented by every result, whatever its value type.
type addressed interface{ target() screen }

// load runs f off the UI goroutine under ctx with a timeout and addresses
// the outcome to the screen to, stamped with seq.
func load[T any](ctx context.Context, to screen, seq int, f func(context.Context) (T, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, requestTimeout)
		defer cancel()
		v, err := f(ctx)
		return result[T]{to: to, seq: seq, val: v, err: err}
	}
}

// resizeMsg is the space a screen has, without App's header and footer.
type resizeMsg struct{ width, height int }

// tickMsg comes every second so durations of running things keep counting.
type tickMsg time.Time

type pushMsg struct{ s screen }

type errMsg struct{ err error }

type envChosenMsg struct{ name string }

func push(s screen) tea.Cmd { return func() tea.Msg { return pushMsg{s} } }

// reportErr shows err in the status bar. Cancellation is not reported: it
// only happens to requests of screens the user has already left.
func reportErr(err error) tea.Cmd {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return func() tea.Msg { return errMsg{err} }
}

func isEnter(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "enter", "l", "right":
		return true
	}
	return false
}
