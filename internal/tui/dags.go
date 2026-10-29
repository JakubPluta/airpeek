package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

const (
	// recentRunsShown is how many of the newest runs each DAG row shows as
	// squares. They come in the same request as the last run.
	recentRunsShown = 10
	// recentConcurrency caps parallel recent-runs requests so scrolling a
	// long DAG list does not flood the webserver.
	recentConcurrency = 8
)

// recentRuns are the newest runs of a DAG, newest first; empty when the
// DAG never ran.
type recentRuns struct {
	dagID string
	runs  []airflow.DAGRun
}

type dagsScreen struct {
	base
	g       airflow.Getter
	sem     chan struct{}
	loading bool
	all     []airflow.DAG
	shown   []airflow.DAG
	// recent holds the runs of every DAG fetched so far; a missing key
	// means not fetched yet. requested guards against fetching twice.
	// Both belong to the list load numbered seq and are reset with it.
	recent     map[string][]airflow.DAGRun
	requested  map[string]bool
	hidePaused bool
	filter     textinput.Model
	table      table
}

func newDAGsScreen(parent context.Context, g airflow.Getter) *dagsScreen {
	return &dagsScreen{
		base:   newBase(parent),
		g:      g,
		sem:    make(chan struct{}, recentConcurrency),
		filter: newInput("filter by dag_id or tag"),
		table: table{cols: []column{
			{title: "DAG"},
			{title: "PAUSED", width: 6},
			{title: "SCHEDULE", width: 16},
			{title: "RECENT RUNS", width: 11},
			{title: "LAST RUN", width: 15},
			{title: "LAST LOGICAL DATE", width: 19},
			{title: "NEXT RUN", width: 19},
			{title: "TAGS", width: 24},
		}},
	}
}

// newInput is a "/" prompt with a steady cursor; a blinking one would keep
// the UI redrawing for nothing.
func newInput(placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = "/"
	in.Placeholder = placeholder
	in.Cursor.SetMode(cursor.CursorStatic)
	return in
}

func (s *dagsScreen) init() tea.Cmd {
	s.loading = true
	s.recent = map[string][]airflow.DAGRun{}
	s.requested = map[string]bool{}
	g := s.g
	return load(s.ctx, s, s.nextSeq(), func(ctx context.Context) ([]airflow.DAG, error) {
		return airflow.ListDAGs(ctx, g)
	})
}

func (s *dagsScreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case result[[]airflow.DAG]:
		if msg.seq != s.seq {
			return nil
		}
		s.loading = false
		if msg.err != nil {
			return reportErr(msg.err)
		}
		s.all = msg.val
		s.applyFilter()
		return s.fetchVisible()
	case result[recentRuns]:
		if msg.seq != s.seq {
			return nil
		}
		if msg.err != nil {
			// Retried when the row is scrolled into view again.
			delete(s.requested, msg.val.dagID)
			return reportErr(fmt.Errorf("recent runs of %s: %w", msg.val.dagID, msg.err))
		}
		s.recent[msg.val.dagID] = msg.val.runs
		s.refreshRows()
	case resizeMsg:
		s.table.setSize(msg.width, msg.height-1) // one line for the summary / filter
		return s.fetchVisible()
	case tea.MouseMsg:
		if s.table.mouse(msg, msg.Y-1) {
			return s.open()
		}
		return s.fetchVisible()
	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return nil
}

func (s *dagsScreen) handleKey(msg tea.KeyMsg) tea.Cmd {
	if s.filter.Focused() {
		switch msg.String() {
		case "enter":
			s.filter.Blur()
		case "esc":
			s.filter.Blur()
			s.filter.SetValue("")
		default:
			s.filter, _ = s.filter.Update(msg)
		}
		s.applyFilter()
		return s.fetchVisible()
	}
	if s.table.update(msg) {
		return s.fetchVisible()
	}
	switch {
	case msg.String() == "/":
		return s.filter.Focus()
	case msg.String() == "p":
		s.hidePaused = !s.hidePaused
		s.applyFilter()
		return s.fetchVisible()
	case msg.String() == "r":
		return s.init()
	case msg.String() == "i":
		return push(newImportErrorsScreen(s.ctx, s.g))
	case isEnter(msg):
		return s.open()
	}
	return nil
}

func (s *dagsScreen) open() tea.Cmd {
	if len(s.shown) == 0 {
		return nil
	}
	return push(newRunsScreen(s.ctx, s.g, s.shown[s.table.cursor].DAGID))
}

func (s *dagsScreen) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(s.filter.Value()))
	s.shown = nil
	for _, d := range s.all {
		if s.hidePaused && d.IsPaused {
			continue
		}
		if q == "" || strings.Contains(strings.ToLower(d.DAGID), q) || hasTag(d, q) {
			s.shown = append(s.shown, d)
		}
	}
	s.refreshRows()
}

func hasTag(d airflow.DAG, q string) bool {
	for _, t := range d.Tags {
		if strings.Contains(strings.ToLower(t.Name), q) {
			return true
		}
	}
	return false
}

func (s *dagsScreen) refreshRows() {
	rows := make([][]cell, len(s.shown))
	for i, d := range s.shown {
		paused := plain("no")
		if d.IsPaused {
			paused = cell{text: "yes", style: dimStyle}
		}
		squares, state, logical := plain(""), plain(""), plain("")
		if runs, ok := s.recent[d.DAGID]; ok {
			squares, state, logical = recentCell(runs), plain("-"), plain("-")
			if len(runs) > 0 {
				state, logical = stateCell(runs[0].State), plain(timeText(runs[0].LogicalDate))
			}
		}
		next := "-"
		if !d.IsPaused {
			next = timeText(d.NextDagrun)
		}
		name := d.DAGID
		if d.HasImportErrors {
			name = "⚠ " + name
		}
		rows[i] = []cell{plain(name), paused, plain(d.ScheduleText()), squares, state, logical, plain(next), plain(tagsText(d.Tags))}
	}
	s.table.setRows(rows)
}

// recentCell draws runs as squares in their state's color, oldest on the
// left like Airflow's grid, so the newest run lines up on the right.
func recentCell(runs []airflow.DAGRun) cell {
	spans := []span{{text: strings.Repeat(" ", recentRunsShown-len(runs)), style: lipgloss.NewStyle()}}
	for i := len(runs) - 1; i >= 0; i-- {
		spans = append(spans, span{text: "■", style: stateStyle(runs[i].State)})
	}
	return cell{spans: spans, style: lipgloss.NewStyle()}
}

func tagsText(tags []airflow.Tag) string {
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	return strings.Join(names, ",")
}

// fetchVisible requests the recent runs of every DAG on screen not asked
// for yet. The answers carry the current seq, so a reload discards them.
func (s *dagsScreen) fetchVisible() tea.Cmd {
	if s.loading {
		return nil
	}
	from, to := s.table.visible()
	var cmds []tea.Cmd
	for _, d := range s.shown[from:to] {
		if s.requested[d.DAGID] {
			continue
		}
		s.requested[d.DAGID] = true
		dagID, g, sem := d.DAGID, s.g, s.sem
		cmds = append(cmds, load(s.ctx, s, s.seq, func(ctx context.Context) (recentRuns, error) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return recentRuns{dagID: dagID}, ctx.Err()
			}
			runs, err := airflow.ListDAGRuns(ctx, g, dagID, recentRunsShown)
			return recentRuns{dagID: dagID, runs: runs}, err
		}))
	}
	return tea.Batch(cmds...)
}

func (s *dagsScreen) view() string {
	if s.loading {
		return dimStyle.Render("loading DAGs…")
	}
	top := s.summary()
	if s.filter.Focused() || s.filter.Value() != "" {
		top = s.filter.View() + "  " + top
	}
	return top + "\n" + s.table.view()
}

// summary counts what is known without asking Airflow for more: DAGs from
// the list, last-run states only of DAGs whose runs were loaded (the rows
// scrolled past so far).
func (s *dagsScreen) summary() string {
	paused := 0
	for _, d := range s.all {
		if d.IsPaused {
			paused++
		}
	}
	pausedText := fmt.Sprintf("%d paused", paused)
	if s.hidePaused {
		pausedText += " (hidden)"
	}
	parts := []string{dimStyle.Render(fmt.Sprintf("%d of %d DAGs · %s", len(s.shown), len(s.all), pausedText))}

	counts := map[string]int{}
	loaded := 0
	for _, runs := range s.recent {
		loaded++
		state := "none"
		if len(runs) > 0 {
			state = stateName(runs[0].State)
		}
		counts[state]++
	}
	if loaded > 0 {
		states := []string{dimStyle.Render(fmt.Sprintf("last run of %d loaded:", loaded))}
		for _, st := range stateOrder {
			if n := counts[st]; n > 0 {
				states = append(states, stateStyle(st).Render("■")+dimStyle.Render(fmt.Sprintf(" %d %s", n, st)))
			}
		}
		parts = append(parts, strings.Join(states, " "))
	}
	return strings.Join(parts, dimStyle.Render("  ·  "))
}

func (s *dagsScreen) title() string { return "DAGs" }

func (s *dagsScreen) help() string {
	if s.filter.Focused() {
		return "type to filter · enter keep · esc clear"
	}
	return "↑↓ move · enter runs · / filter · p hide paused · r refresh · i import errors"
}

func (s *dagsScreen) capturing() bool { return s.filter.Focused() }

// busy is true while the list or any row's runs are still on the way.
func (s *dagsScreen) busy() bool {
	if s.loading {
		return true
	}
	for id := range s.requested {
		if _, ok := s.recent[id]; !ok {
			return true
		}
	}
	return false
}
