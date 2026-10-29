package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

// fakeAirflow answers GETs from a path → JSON body map, and fails like a
// real request would once its context is cancelled.
type fakeAirflow map[string]string

func (f fakeAirflow) Get(ctx context.Context, path string, _ url.Values) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, ok := f[path]
	if !ok {
		return nil, fmt.Errorf("unexpected path %s", path)
	}
	return []byte(body), nil
}

var qaAirflow = fakeAirflow{
	"/dags": `{"total_entries": 2, "dags": [
		{"dag_id": "finance_daily", "tags": [{"name": "finance"}]},
		{"dag_id": "people_sync", "is_paused": true, "tags": [{"name": "hr"}]}
	]}`,
	"/dags/finance_daily/dagRuns": `{"dag_runs": [
		{"dag_run_id": "manual__1", "dag_id": "finance_daily", "state": "failed"}
	]}`,
	"/dags/people_sync/dagRuns": `{"dag_runs": []}`,
	"/dags/finance_daily/dagRuns/manual__1/taskInstances": `{"total_entries": 1, "task_instances": [
		{"task_id": "load", "map_index": -1, "state": "failed", "try_number": 2}
	]}`,
	"/dags/finance_daily/dagRuns/manual__1/taskInstances/load/logs/2": `"step 1 ok\nboom: connection refused"`,
}

func newTestApp(t *testing.T, g airflow.Getter) *App {
	t.Helper()
	a := New(Config{Env: "local", Connect: func(string) (Environment, error) { return Environment{Getter: g}, nil }})
	_, cmd := a.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	drain(a, cmd)
	drain(a, a.top().init())
	return a
}

// drain runs cmd and feeds what it produces back into a, as the Bubble Tea
// runtime would, until nothing is left. Tests never start the clock tick.
func drain(a *App, cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		_, next := a.Update(msg)
		queue = append(queue, next)
	}
}

func press(a *App, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := a.Update(msg)
		drain(a, cmd)
	}
}

func breadcrumbs(a *App) string {
	titles := make([]string, len(a.stack))
	for i, s := range a.stack {
		titles[i] = s.title()
	}
	return strings.Join(titles, " › ")
}

func TestBrowsesFromDAGsToLogAndBack(t *testing.T) {
	a := newTestApp(t, qaAirflow)

	press(a, "enter", "enter", "enter")

	if got := breadcrumbs(a); got != "DAGs › finance_daily › manual__1 › load · try 2/2" {
		t.Errorf("breadcrumbs = %q", got)
	}
	if !strings.Contains(a.View(), "boom: connection refused") {
		t.Errorf("log not shown:\n%s", a.View())
	}

	press(a, "esc", "esc", "esc")

	if got := breadcrumbs(a); got != "DAGs" {
		t.Errorf("breadcrumbs after esc = %q", got)
	}
	if a.err != nil {
		t.Errorf("unexpected error: %v", a.err)
	}
}

func TestDAGListShowsRecentRunsOfEveryVisibleDAG(t *testing.T) {
	a := newTestApp(t, qaAirflow)
	dags := a.top().(*dagsScreen)

	if runs := dags.recent["finance_daily"]; len(runs) != 1 || runs[0].State != "failed" {
		t.Errorf("finance_daily recent runs = %+v", runs)
	}
	if runs, ok := dags.recent["people_sync"]; !ok || len(runs) != 0 {
		t.Errorf("people_sync recent runs = %+v, fetched = %v; want fetched and none", runs, ok)
	}
	if a.busy() {
		t.Error("app still busy after every load finished")
	}
}

func TestFilterMatchesDAGIDOrTag(t *testing.T) {
	a := newTestApp(t, qaAirflow)
	dags := a.top().(*dagsScreen)

	press(a, "/", "h", "r", "enter")

	if len(dags.shown) != 1 || dags.shown[0].DAGID != "people_sync" {
		t.Errorf("filter by tag shows %v", dags.shown)
	}

	press(a, "/", "esc")

	if len(dags.shown) != 2 {
		t.Errorf("cleared filter shows %d DAGs, want 2", len(dags.shown))
	}
}

func TestLeavingAScreenCancelsItsRequestAndDropsTheAnswer(t *testing.T) {
	a := newTestApp(t, qaAirflow)
	_, pushCmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, loadRuns := a.Update(pushCmd())
	runs := a.top().(*runsScreen)

	press(a, "esc")
	drain(a, loadRuns)

	if !errors.Is(runs.ctx.Err(), context.Canceled) {
		t.Errorf("closed screen context err = %v, want canceled", runs.ctx.Err())
	}
	if runs.runs != nil {
		t.Errorf("closed screen received runs %v", runs.runs)
	}
	if got := breadcrumbs(a); got != "DAGs" || a.err != nil {
		t.Errorf("breadcrumbs = %q, err = %v", got, a.err)
	}
}

func TestRunningDurationKeepsCountingOnTick(t *testing.T) {
	a := newTestApp(t, fakeAirflow{
		"/dags":                       `{"total_entries": 1, "dags": [{"dag_id": "finance_daily"}]}`,
		"/dags/finance_daily/dagRuns": `{"dag_runs": [{"dag_run_id": "manual__1", "state": "running", "start_date": "2026-11-01T10:00:00+00:00", "end_date": null}]}`,
	})
	press(a, "enter")
	runs := a.top().(*runsScreen)

	a.Update(tickMsg(time.Date(2026, 11, 1, 10, 0, 5, 0, time.UTC)))
	first := runs.table.rows[0][5].text
	a.Update(tickMsg(time.Date(2026, 11, 1, 10, 1, 0, 0, time.UTC)))
	second := runs.table.rows[0][5].text

	if first != "5s" || second != "1m00s" {
		t.Errorf("durations = %q, %q; want 5s, 1m00s", first, second)
	}
}

func TestHelpCoversTheScreenAndClosesWithEsc(t *testing.T) {
	a := newTestApp(t, qaAirflow)

	press(a, "?")
	if !strings.Contains(a.View(), "filter by dag_id or tag") {
		t.Errorf("help not shown:\n%s", a.View())
	}

	press(a, "esc")
	if a.showHelp || breadcrumbs(a) != "DAGs" {
		t.Errorf("showHelp = %v, breadcrumbs = %q", a.showHelp, breadcrumbs(a))
	}
}

// ignoresCancel answers even after its request was cancelled, like a
// response that was already on the wire.
type ignoresCancel struct{ fakeAirflow }

func (f ignoresCancel) Get(_ context.Context, path string, q url.Values) ([]byte, error) {
	return f.fakeAirflow.Get(context.Background(), path, q)
}

func TestAnswerArrivingAfterTheScreenClosedIsDropped(t *testing.T) {
	a := newTestApp(t, ignoresCancel{qaAirflow})
	_, pushCmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, loadRuns := a.Update(pushCmd())
	runs := a.top().(*runsScreen)

	press(a, "esc")
	drain(a, loadRuns)

	if runs.runs != nil {
		t.Errorf("closed screen received runs %v", runs.runs)
	}
}

func TestRecentRunsAreSquaresNewestOnTheRight(t *testing.T) {
	c := recentCell([]airflow.DAGRun{{State: "failed"}, {State: "success"}, {State: "running"}})

	var text strings.Builder
	for _, s := range c.spans {
		text.WriteString(s.text)
	}

	if text.String() != "       ■■■" {
		t.Errorf("squares = %q", text.String())
	}
	if last := c.spans[len(c.spans)-1]; last.style.GetForeground() != stateColors["failed"] {
		t.Errorf("newest square color = %v, want failed", last.style.GetForeground())
	}
}

func TestSummaryAndHidingPausedDAGs(t *testing.T) {
	a := newTestApp(t, qaAirflow)
	dags := a.top().(*dagsScreen)

	if got := ansi.Strip(dags.summary()); got != "2 of 2 DAGs · 1 paused  ·  last run of 2 loaded: ■ 1 failed ■ 1 none" {
		t.Errorf("summary = %q", got)
	}

	press(a, "p")

	if len(dags.shown) != 1 || dags.shown[0].DAGID != "finance_daily" {
		t.Errorf("with paused hidden shows %v", dags.shown)
	}
	if got := ansi.Strip(dags.summary()); !strings.HasPrefix(got, "1 of 2 DAGs · 1 paused (hidden)") {
		t.Errorf("summary = %q", got)
	}
}

func TestTimelineDrawsTasksAcrossTheRun(t *testing.T) {
	a := newTestApp(t, fakeAirflow{
		"/dags":                       `{"total_entries": 1, "dags": [{"dag_id": "finance_daily"}]}`,
		"/dags/finance_daily/dagRuns": `{"dag_runs": [{"dag_run_id": "manual__1", "state": "success"}]}`,
		"/dags/finance_daily/dagRuns/manual__1/taskInstances": `{"total_entries": 2, "task_instances": [
			{"task_id": "extract", "map_index": -1, "state": "success", "try_number": 1,
			 "start_date": "2026-11-01T06:00:00+00:00", "end_date": "2026-11-01T06:00:04+00:00"},
			{"task_id": "load", "map_index": -1, "state": "success", "try_number": 1,
			 "start_date": "2026-11-01T06:00:04+00:00", "end_date": "2026-11-01T06:00:08+00:00"}
		]}`,
	})
	press(a, "enter", "enter", "t")
	tasks := a.top().(*tasksScreen)

	// 160 columns minus STATE, TASK, DURATION and separators leave 98 cells.
	extract, load := tasks.table.rows[0][3].text, tasks.table.rows[1][3].text
	if extract != strings.Repeat("█", 49) {
		t.Errorf("extract bar = %q", extract)
	}
	if load != strings.Repeat(" ", 49)+strings.Repeat("█", 49) {
		t.Errorf("load bar = %q", load)
	}
	if got := tasks.table.cols[3].title; got != "TIMELINE 06:00:00 → 06:00:08 UTC" {
		t.Errorf("timeline title = %q", got)
	}
}

func TestMouseClickSelectsThenOpens(t *testing.T) {
	a := newTestApp(t, qaAirflow)
	// Screen lines: 0 header, 1 summary, 2 table header, 3 first row, 4 second row.
	click := tea.MouseMsg{X: 5, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}

	_, cmd := a.Update(click)
	drain(a, cmd)
	selected := a.top().(*dagsScreen).table.cursor
	_, cmd = a.Update(click)
	drain(a, cmd)

	if selected != 1 {
		t.Errorf("cursor after first click = %d, want 1", selected)
	}
	if got := breadcrumbs(a); got != "DAGs › people_sync" {
		t.Errorf("breadcrumbs after second click = %q", got)
	}
}

func TestEnvironmentThatCannotBeOpenedLeavesThePickerWithAnError(t *testing.T) {
	a := New(Config{
		ListEnvironments: func(context.Context) ([]string, error) { return []string{"local", "astro"}, nil },
		Connect: func(env string) (Environment, error) {
			if env == "astro" {
				return Environment{}, errors.New("environment variable ASTRO_API_TOKEN is not set")
			}
			return Environment{Getter: qaAirflow}, nil
		},
	})
	_, cmd := a.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	drain(a, cmd)
	drain(a, a.top().init())

	press(a, "j", "enter")

	if got := breadcrumbs(a); got != "environments" {
		t.Errorf("breadcrumbs = %q, want to stay on the picker", got)
	}
	if !strings.Contains(a.View(), "error: open astro: environment variable ASTRO_API_TOKEN is not set") {
		t.Errorf("error not shown:\n%s", a.View())
	}

	press(a, "k", "enter")

	if got := breadcrumbs(a); got != "DAGs" {
		t.Errorf("breadcrumbs after opening local = %q", got)
	}
}
