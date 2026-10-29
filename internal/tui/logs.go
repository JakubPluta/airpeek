package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

// logScreen shows one try of a task instance; [ and ] switch between tries.
type logScreen struct {
	base
	g       airflow.Getter
	dagID   string
	runID   string
	ti      airflow.TaskInstance
	try     int
	loading bool
	text    textView
}

func newLogScreen(parent context.Context, g airflow.Getter, dagID, runID string, ti airflow.TaskInstance) *logScreen {
	return &logScreen{
		base:  newBase(parent),
		g:     g,
		dagID: dagID,
		runID: runID,
		ti:    ti,
		try:   lastTry(ti),
		text:  newTextView(),
	}
}

// lastTry is the newest try of ti; tasks that never ran (try_number 0) show try 1.
func lastTry(ti airflow.TaskInstance) int { return max(ti.TryNumber, 1) }

func (s *logScreen) init() tea.Cmd {
	s.loading = true
	g, dagID, runID, ti, try := s.g, s.dagID, s.runID, s.ti, s.try
	return load(s.ctx, s, s.nextSeq(), func(ctx context.Context) (string, error) {
		return airflow.GetTaskLog(ctx, g, dagID, runID, ti.TaskID, ti.MapIndex, try)
	})
}

func (s *logScreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case result[string]:
		if msg.seq != s.seq {
			return nil
		}
		s.loading = false
		if msg.err != nil {
			s.text.setText("")
			return reportErr(msg.err)
		}
		s.text.setText(msg.val)
		// Failures are at the end of a log, so open there.
		s.text.vp.GotoBottom()
	case resizeMsg:
		s.text.setSize(msg.width, msg.height)
	case tea.MouseMsg:
		return s.text.mouse(msg)
	case tea.KeyMsg:
		if !s.text.capturing() {
			switch msg.String() {
			case "[":
				return s.switchTry(-1)
			case "]":
				return s.switchTry(1)
			case "r":
				return s.init()
			}
		}
		return s.text.update(msg)
	}
	return nil
}

func (s *logScreen) switchTry(dir int) tea.Cmd {
	next := s.try + dir
	if next < 1 || next > lastTry(s.ti) {
		return nil
	}
	s.try = next
	return s.init()
}

func (s *logScreen) view() string {
	if s.loading {
		return dimStyle.Render("loading log…")
	}
	return s.text.view()
}

func (s *logScreen) title() string {
	name := s.ti.TaskID
	if s.ti.MapIndex >= 0 {
		name = fmt.Sprintf("%s[%d]", name, s.ti.MapIndex)
	}
	return fmt.Sprintf("%s · try %d/%d", name, s.try, lastTry(s.ti))
}

func (s *logScreen) help() string {
	if s.text.capturing() {
		return s.text.help()
	}
	return s.text.help() + " · [/] try · r reload"
}

func (s *logScreen) capturing() bool { return s.text.capturing() }
func (s *logScreen) busy() bool      { return s.loading }
