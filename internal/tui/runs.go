package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

// runsLimit is how many of the newest runs the runs screen shows.
const runsLimit = 50

type runsScreen struct {
	base
	g       airflow.Getter
	dagID   string
	loading bool
	runs    []airflow.DAGRun
	table   table
}

func newRunsScreen(parent context.Context, g airflow.Getter, dagID string) *runsScreen {
	return &runsScreen{base: newBase(parent), g: g, dagID: dagID, table: table{cols: []column{
		{title: "STATE", width: 15},
		{title: "RUN ID"},
		{title: "TYPE", width: 12},
		{title: "LOGICAL DATE", width: 19},
		{title: "START", width: 19},
		{title: "DURATION", width: 10},
	}}}
}

func (s *runsScreen) init() tea.Cmd {
	s.loading = true
	g, dagID := s.g, s.dagID
	return load(s.ctx, s, s.nextSeq(), func(ctx context.Context) ([]airflow.DAGRun, error) {
		return airflow.ListDAGRuns(ctx, g, dagID, runsLimit)
	})
}

func (s *runsScreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case result[[]airflow.DAGRun]:
		if msg.seq != s.seq {
			return nil
		}
		s.loading = false
		if msg.err != nil {
			return reportErr(msg.err)
		}
		s.runs = msg.val
		s.refreshRows(time.Now())
	case tickMsg:
		if s.anyRunning() {
			s.refreshRows(time.Time(msg))
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
		switch {
		case msg.String() == "r":
			return s.init()
		case isEnter(msg):
			return s.open()
		}
	}
	return nil
}

func (s *runsScreen) open() tea.Cmd {
	if len(s.runs) == 0 {
		return nil
	}
	return push(newTasksScreen(s.ctx, s.g, s.dagID, s.runs[s.table.cursor].DAGRunID))
}

func (s *runsScreen) refreshRows(now time.Time) {
	rows := make([][]cell, len(s.runs))
	for i, r := range s.runs {
		rows[i] = []cell{
			stateCell(r.State),
			plain(r.DAGRunID),
			plain(r.RunType),
			plain(timeText(r.LogicalDate)),
			plain(timeText(r.StartDate)),
			plain(durationText(r.StartDate, r.EndDate, now)),
		}
	}
	s.table.setRows(rows)
}

func (s *runsScreen) anyRunning() bool {
	for _, r := range s.runs {
		if r.StartDate != nil && r.EndDate == nil {
			return true
		}
	}
	return false
}

func (s *runsScreen) view() string {
	if s.loading && s.runs == nil {
		return dimStyle.Render("loading runs…")
	}
	return s.table.view()
}

func (s *runsScreen) title() string   { return s.dagID }
func (s *runsScreen) help() string    { return "↑↓ move · enter tasks · r refresh" }
func (s *runsScreen) capturing() bool { return false }
func (s *runsScreen) busy() bool      { return s.loading }
