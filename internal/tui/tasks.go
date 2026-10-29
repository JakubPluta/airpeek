package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

var (
	taskColumns = []column{
		{title: "STATE", width: 17},
		{title: "TASK"},
		{title: "MAP", width: 4},
		{title: "TRY", width: 3},
		{title: "START", width: 19},
		{title: "DURATION", width: 10},
		{title: "OPERATOR", width: 24},
	}
	// timelineColumns replace the details with a Gantt chart of the run;
	// the last column takes the remaining width and its title the time span.
	timelineColumns = []column{
		{title: "STATE", width: 17},
		{title: "TASK", width: 32},
		{title: "DURATION", width: 10},
		{title: "TIMELINE"},
	}
)

type tasksScreen struct {
	base
	g        airflow.Getter
	dagID    string
	runID    string
	loading  bool
	tis      []airflow.TaskInstance
	timeline bool
	table    table
}

func newTasksScreen(parent context.Context, g airflow.Getter, dagID, runID string) *tasksScreen {
	return &tasksScreen{
		base:  newBase(parent),
		g:     g,
		dagID: dagID,
		runID: runID,
		table: table{cols: slices.Clone(taskColumns)},
	}
}

func (s *tasksScreen) init() tea.Cmd {
	s.loading = true
	g, dagID, runID := s.g, s.dagID, s.runID
	return load(s.ctx, s, s.nextSeq(), func(ctx context.Context) ([]airflow.TaskInstance, error) {
		return airflow.ListTaskInstances(ctx, g, dagID, runID)
	})
}

func (s *tasksScreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case result[[]airflow.TaskInstance]:
		if msg.seq != s.seq {
			return nil
		}
		s.loading = false
		if msg.err != nil {
			return reportErr(msg.err)
		}
		s.tis = msg.val
		sortByStart(s.tis)
		s.refreshRows(time.Now())
	case tickMsg:
		if s.anyRunning() {
			s.refreshRows(time.Time(msg))
		}
	case resizeMsg:
		s.table.setSize(msg.width, msg.height)
		s.refreshRows(time.Now()) // the timeline is drawn to the column width
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
		case msg.String() == "t":
			s.timeline = !s.timeline
			s.table.cols = slices.Clone(taskColumns)
			if s.timeline {
				s.table.cols = slices.Clone(timelineColumns)
			}
			s.refreshRows(time.Now())
		case isEnter(msg):
			return s.open()
		}
	}
	return nil
}

func (s *tasksScreen) open() tea.Cmd {
	if len(s.tis) == 0 {
		return nil
	}
	return push(newLogScreen(s.ctx, s.g, s.dagID, s.runID, s.tis[s.table.cursor]))
}

func (s *tasksScreen) refreshRows(now time.Time) {
	if s.timeline {
		s.refreshTimeline(now)
		return
	}
	rows := make([][]cell, len(s.tis))
	for i, ti := range s.tis {
		rows[i] = []cell{
			stateCell(ti.StateText()),
			plain(ti.TaskID),
			plain(mapIndexText(ti)),
			plain(strconv.Itoa(ti.TryNumber)),
			plain(timeText(ti.StartDate)),
			plain(durationText(ti.StartDate, ti.EndDate, now)),
			plain(ti.Operator),
		}
	}
	s.table.setRows(rows)
}

func (s *tasksScreen) refreshTimeline(now time.Time) {
	starts := make([]*time.Time, len(s.tis))
	ends := make([]*time.Time, len(s.tis))
	for i, ti := range s.tis {
		starts[i], ends[i] = ti.StartDate, ti.EndDate
	}
	from, to, ok := timelineSpan(starts, ends, now)
	last := len(s.table.cols) - 1
	s.table.cols[last].title = "TIMELINE"
	if ok {
		s.table.cols[last].title = fmt.Sprintf("TIMELINE %s → %s UTC", from.UTC().Format("15:04:05"), to.UTC().Format("15:04:05"))
	}
	width := s.table.widths()[last]
	rows := make([][]cell, len(s.tis))
	for i, ti := range s.tis {
		bar := ""
		if ok && ti.StartDate != nil {
			end := now
			if ti.EndDate != nil {
				end = *ti.EndDate
			}
			bar = ganttBar(*ti.StartDate, end, from, to, width)
		}
		name := ti.TaskID
		if ti.MapIndex >= 0 {
			name += "[" + mapIndexText(ti) + "]"
		}
		rows[i] = []cell{
			stateCell(ti.StateText()),
			plain(name),
			plain(durationText(ti.StartDate, ti.EndDate, now)),
			{text: bar, style: stateStyle(ti.StateText())},
		}
	}
	s.table.setRows(rows)
}

func mapIndexText(ti airflow.TaskInstance) string {
	if ti.MapIndex < 0 {
		return ""
	}
	return strconv.Itoa(ti.MapIndex)
}

func (s *tasksScreen) anyRunning() bool {
	for _, ti := range s.tis {
		if ti.StartDate != nil && ti.EndDate == nil {
			return true
		}
	}
	return false
}

// sortByStart orders tasks the way they ran; never-started tasks go last,
// ties keep task_id and map_index order.
func sortByStart(tis []airflow.TaskInstance) {
	sort.SliceStable(tis, func(i, j int) bool {
		a, b := tis[i], tis[j]
		if (a.StartDate == nil) != (b.StartDate == nil) {
			return a.StartDate != nil
		}
		if a.StartDate != nil && !a.StartDate.Equal(*b.StartDate) {
			return a.StartDate.Before(*b.StartDate)
		}
		if a.TaskID != b.TaskID {
			return a.TaskID < b.TaskID
		}
		return a.MapIndex < b.MapIndex
	})
}

func (s *tasksScreen) view() string {
	if s.loading && s.tis == nil {
		return dimStyle.Render("loading task instances…")
	}
	return s.table.view()
}

func (s *tasksScreen) title() string { return s.runID }

func (s *tasksScreen) help() string {
	if s.timeline {
		return "↑↓ move · enter log · t table · r refresh"
	}
	return "↑↓ move · enter log · t timeline · r refresh"
}

func (s *tasksScreen) capturing() bool { return false }
func (s *tasksScreen) busy() bool      { return s.loading }
