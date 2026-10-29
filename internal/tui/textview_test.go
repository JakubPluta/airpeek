package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func typeInto(tv *textView, keys ...string) {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == "enter" {
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		}
		tv.update(msg)
	}
}

func TestSearchFindsMatchesCaseInsensitivelyAndCycles(t *testing.T) {
	tv := newTextView()
	tv.setSize(80, 3)
	tv.setText("start\nerror one\nok\nERROR two\nend")

	typeInto(&tv, "/", "e", "r", "r", "o", "r", "enter")
	first := tv.matchPos
	typeInto(&tv, "n")
	second := tv.matchPos
	typeInto(&tv, "n")
	wrapped := tv.matchPos

	if len(tv.matches) != 2 || tv.matches[0] != 1 || tv.matches[1] != 3 {
		t.Fatalf("matches = %v, want [1 3]", tv.matches)
	}
	if first != 0 || second != 1 || wrapped != 0 {
		t.Errorf("match positions = %d, %d, %d; want 0, 1, 0", first, second, wrapped)
	}
}

func TestDurationText(t *testing.T) {
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 1, 10, 0, 42, 0, time.UTC)
	now := time.Date(2026, 11, 1, 11, 2, 3, 0, time.UTC)

	if got := durationText(&start, &end, now); got != "42s" {
		t.Errorf("finished = %q, want 42s", got)
	}
	if got := durationText(&start, nil, now); got != "1h02m03s" {
		t.Errorf("running = %q, want 1h02m03s", got)
	}
	if got := durationText(nil, nil, now); got != "-" {
		t.Errorf("not started = %q, want -", got)
	}
}
