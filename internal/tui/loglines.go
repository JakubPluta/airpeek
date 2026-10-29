package tui

import (
	"regexp"
	"strings"
)

// lineKind is how a log line is colored.
type lineKind int

const (
	kindPlain lineKind = iota
	// kindMeta marks the "***" lines Airflow adds about where it read the
	// log from.
	kindMeta
	kindDebug
	kindWarn
	kindError
)

// logPrefix matches Airflow's task log line prefix,
// "[2026-11-01, 06:00:03 UTC] {taskinstance.py:1234} ERROR - ", capturing the level.
var logPrefix = regexp.MustCompile(`^\[[^\]]+\] \{[^}]*\} ([A-Z]+) - `)

type logLine struct {
	kind lineKind
	// prefix is the byte length of the line's log prefix, shown dimmed;
	// 0 when the line has none.
	prefix int
}

// classifyLog decides how each line of a task log is shown. A line without
// the log prefix continues the message above it (multi-line output,
// tracebacks) and takes its kind; a Python traceback is an error wherever
// it starts. Text that is not an Airflow log stays plain.
func classifyLog(lines []string) []logLine {
	out := make([]logLine, len(lines))
	current := kindPlain
	for i, l := range lines {
		if m := logPrefix.FindStringSubmatchIndex(l); m != nil {
			current = levelKind(l[m[2]:m[3]])
			out[i] = logLine{kind: current, prefix: m[1]}
			continue
		}
		if strings.HasPrefix(l, "***") {
			out[i] = logLine{kind: kindMeta}
			continue
		}
		if strings.HasPrefix(l, "Traceback (most recent call last):") {
			current = kindError
		}
		out[i] = logLine{kind: current}
	}
	return out
}

func levelKind(level string) lineKind {
	switch level {
	case "ERROR", "CRITICAL", "FATAL":
		return kindError
	case "WARNING", "WARN":
		return kindWarn
	case "DEBUG":
		return kindDebug
	}
	return kindPlain
}
