package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

type envsScreen struct {
	base
	list    func(ctx context.Context) ([]string, error)
	loading bool
	names   []string
	table   table
}

func newEnvsScreen(parent context.Context, list func(ctx context.Context) ([]string, error)) *envsScreen {
	return &envsScreen{base: newBase(parent), list: list, table: table{cols: []column{{title: "ENVIRONMENT"}}}}
}

func (s *envsScreen) init() tea.Cmd {
	s.loading = true
	return load(s.ctx, s, s.nextSeq(), s.list)
}

func (s *envsScreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case result[[]string]:
		if msg.seq != s.seq {
			return nil
		}
		s.loading = false
		if msg.err != nil {
			return reportErr(msg.err)
		}
		s.names = msg.val
		rows := make([][]cell, len(s.names))
		for i, n := range s.names {
			rows[i] = []cell{plain(n)}
		}
		s.table.setRows(rows)
		if len(s.names) == 1 {
			return s.open()
		}
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
		if isEnter(msg) {
			return s.open()
		}
	}
	return nil
}

func (s *envsScreen) open() tea.Cmd {
	if len(s.names) == 0 {
		return nil
	}
	name := s.names[s.table.cursor]
	return func() tea.Msg { return envChosenMsg{name} }
}

func (s *envsScreen) view() string {
	if s.loading {
		return dimStyle.Render("loading environments…")
	}
	return s.table.view()
}

func (s *envsScreen) title() string   { return "environments" }
func (s *envsScreen) help() string    { return "↑↓ move · enter open" }
func (s *envsScreen) capturing() bool { return false }
func (s *envsScreen) busy() bool      { return s.loading }
