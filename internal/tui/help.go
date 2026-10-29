package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var keyStyle = lipgloss.NewStyle().Bold(true).Width(14)

type helpSection struct {
	title string
	keys  [][2]string
}

var helpSections = []helpSection{
	{"Everywhere", [][2]string{
		{"↑↓ / j k", "move"},
		{"pgup pgdn", "page (also ctrl+u ctrl+d)"},
		{"g / G", "top / bottom"},
		{"enter / l", "open the selected row"},
		{"esc / h", "go back one level"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
		{"mouse", "wheel scrolls, click selects, click again opens"},
		{"⌥/⇧ + drag", "select text while the mouse is captured"},
	}},
	{"DAGs", [][2]string{
		{"/", "filter by dag_id or tag"},
		{"enter / esc", "keep / clear the filter"},
		{"p", "hide / show paused DAGs"},
		{"r", "reload the list and recent runs"},
		{"i", "DAG files that failed to import"},
		{"⚠ name", "this DAG has an import error"},
		{"RECENT RUNS", "last 10 runs, newest on the right"},
		{"LAST RUN", "newest run by logical date; blank = loading"},
		{"NEXT RUN", "next scheduled logical date; - when paused"},
		{"summary", "last-run states count only loaded rows"},
	}},
	{"Runs → Tasks", [][2]string{
		{"enter", "runs → task instances → log"},
		{"t", "tasks: table ⇄ timeline (Gantt) of the run"},
		{"r", "reload"},
		{"MAP", "map index of a mapped (expanded) task"},
		{"TRY", "attempt; the log opens on the last one"},
		{"DURATION", "still counting while running"},
	}},
	{"Log / stack trace", [][2]string{
		{"/", "search, case-insensitive"},
		{"n / N", "next / previous match"},
		{"[ / ]", "previous / next try"},
		{"r", "reload the log"},
		{"", "logs open at the end, where failures are"},
		{"colors", "errors and tracebacks red, warnings yellow"},
	}},
}

// helpView is the full help page shown in place of the current screen.
// Everything here is read-only: nothing in the app can trigger, pause,
// clear or mark anything in Airflow.
func helpView(width int) string {
	blocks := make([]string, 0, len(helpSections)+1)
	for _, s := range helpSections {
		lines := []string{titleStyle.Render(s.title)}
		for _, k := range s.keys {
			lines = append(lines, "  "+keyStyle.Render(k[0])+dimStyle.Render(k[1]))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	blocks = append(blocks, stateLegend())

	footer := dimStyle.Render("Times are UTC. Read-only: only GET requests are sent to Airflow. Press ? or esc to close.")

	// Two columns when there is room: navigation and DAGs on the left,
	// runs, logs and states on the right. One column otherwise.
	if width >= 110 {
		left := lipgloss.NewStyle().Width(width / 2).Render(strings.Join(blocks[:2], "\n\n"))
		right := strings.Join(blocks[2:], "\n\n")
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n\n" + footer
	}
	return strings.Join(blocks, "\n\n") + "\n\n" + footer
}

func stateLegend() string {
	lines := []string{titleStyle.Render("States")}
	for i := 0; i < len(stateOrder); i += 3 {
		var row []string
		for _, st := range stateOrder[i:min(i+3, len(stateOrder))] {
			row = append(row, stateStyle(st).Render(fit("■ "+st, 20)))
		}
		lines = append(lines, "  "+strings.Join(row, ""))
	}
	return strings.Join(lines, "\n")
}
