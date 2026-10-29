package tui

import (
	"context"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

type importErrorsScreen struct {
	base
	g       airflow.Getter
	loading bool
	errs    []airflow.ImportError
	table   table
}

func newImportErrorsScreen(parent context.Context, g airflow.Getter) *importErrorsScreen {
	return &importErrorsScreen{base: newBase(parent), g: g, table: table{cols: []column{
		{title: "FILE"},
		{title: "SINCE", width: 19},
	}}}
}

func (s *importErrorsScreen) init() tea.Cmd {
	s.loading = true
	g := s.g
	return load(s.ctx, s, s.nextSeq(), func(ctx context.Context) ([]airflow.ImportError, error) {
		return airflow.ListImportErrors(ctx, g)
	})
}

func (s *importErrorsScreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case result[[]airflow.ImportError]:
		if msg.seq != s.seq {
			return nil
		}
		s.loading = false
		if msg.err != nil {
			return reportErr(msg.err)
		}
		s.errs = msg.val
		rows := make([][]cell, len(s.errs))
		for i, e := range s.errs {
			rows[i] = []cell{plain(e.Filename), plain(timeText(e.Timestamp))}
		}
		s.table.setRows(rows)
	case resizeMsg:
		s.table.setSize(msg.width, msg.height)
	case tea.MouseMsg:
		if s.table.mouse(msg, msg.Y) {
			return s.open()
		}
	case tea.KeyMsg:
		if s.table.update(msg) {
			return nil
		}
		switch {
		case msg.String() == "r":
			return s.init()
		case isEnter(msg):
			return s.open()
		}
	}
	return nil
}

func (s *importErrorsScreen) open() tea.Cmd {
	if len(s.errs) == 0 {
		return nil
	}
	return push(newStackTraceScreen(s.ctx, s.errs[s.table.cursor]))
}

func (s *importErrorsScreen) view() string {
	if s.loading && s.errs == nil {
		return dimStyle.Render("loading import errors…")
	}
	return s.table.view()
}

func (s *importErrorsScreen) title() string   { return "import errors" }
func (s *importErrorsScreen) help() string    { return "↑↓ move · enter stack trace · r refresh" }
func (s *importErrorsScreen) capturing() bool { return false }
func (s *importErrorsScreen) busy() bool      { return s.loading }

// stackTraceScreen shows one import error; it has nothing to load.
type stackTraceScreen struct {
	base
	e    airflow.ImportError
	text textView
}

func newStackTraceScreen(parent context.Context, e airflow.ImportError) *stackTraceScreen {
	s := &stackTraceScreen{base: newBase(parent), e: e, text: newTextView()}
	s.text.setText(e.StackTrace)
	return s
}

func (s *stackTraceScreen) init() tea.Cmd { return nil }

func (s *stackTraceScreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case resizeMsg:
		s.text.setSize(msg.width, msg.height)
	case tea.MouseMsg:
		return s.text.mouse(msg)
	case tea.KeyMsg:
		return s.text.update(msg)
	}
	return nil
}

func (s *stackTraceScreen) view() string    { return s.text.view() }
func (s *stackTraceScreen) title() string   { return filepath.Base(s.e.Filename) }
func (s *stackTraceScreen) help() string    { return s.text.help() }
func (s *stackTraceScreen) capturing() bool { return s.text.capturing() }
func (s *stackTraceScreen) busy() bool      { return false }
