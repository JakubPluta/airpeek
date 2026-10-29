package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	titleStyle    = lipgloss.NewStyle().Bold(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	envStyle      = lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	prodEnvStyle  = envStyle.Background(lipgloss.Color("124"))
	matchStyle    = lipgloss.NewStyle().Background(lipgloss.Color("58"))
	busyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

// logStyles color task log lines by kind; see classifyLog.
var logStyles = map[lineKind]lipgloss.Style{
	kindPlain: lipgloss.NewStyle(),
	kindMeta:  lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true),
	kindDebug: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	kindWarn:  lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
	kindError: lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
}

// stateOrder lists Airflow states from the most to the least interesting;
// summaries and the help legend follow it.
var stateOrder = []string{
	"failed", "upstream_failed", "up_for_retry", "running", "restarting", "up_for_reschedule",
	"deferred", "queued", "scheduled", "success", "skipped", "removed", "none",
}

// stateColors follow the Airflow UI palette closely enough to read at a glance.
var stateColors = map[string]lipgloss.Color{
	"success":           "34",
	"failed":            "196",
	"upstream_failed":   "208",
	"running":           "40",
	"queued":            "245",
	"scheduled":         "180",
	"up_for_retry":      "220",
	"up_for_reschedule": "45",
	"deferred":          "141",
	"restarting":        "135",
	"skipped":           "213",
	"removed":           "250",
	"none":              "240",
}

func stateStyle(state string) lipgloss.Style {
	if c, ok := stateColors[stateName(state)]; ok {
		return lipgloss.NewStyle().Foreground(c)
	}
	return lipgloss.NewStyle()
}

// stateName is Airflow's state, with "none" for runs and tasks that have none.
func stateName(state string) string {
	if state == "" {
		return "none"
	}
	return state
}

func stateCell(state string) cell {
	return cell{text: stateName(state), style: stateStyle(state)}
}

// timeText shows times in UTC, the zone Airflow schedules in.
func timeText(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// durationText is end - start, or now - start while there is no end yet.
func durationText(start, end *time.Time, now time.Time) string {
	if start == nil {
		return "-"
	}
	stop := now
	if end != nil {
		stop = *end
	}
	d := stop.Sub(*start).Round(time.Second)
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm%02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
