package tui

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestClassifyLogColorsByLevelAndCarriesItToContinuationLines(t *testing.T) {
	lines := []string{
		"*** Found logs in Cloudwatch",
		"[2026-11-01, 06:00:03 UTC] {taskinstance.py:1234} INFO - Starting attempt 1",
		"[2026-11-01, 06:00:04 UTC] {warnings.py:110} WARNING - deprecated",
		"  more of the warning",
		"[2026-11-01, 06:00:05 UTC] {task.py:7} INFO - calling the API",
		"Traceback (most recent call last):",
		`  File "task.py", line 7`,
		"ConnectionError: refused",
		"[2026-11-01, 06:00:06 UTC] {taskinstance.py:99} ERROR - Task failed",
	}

	got := classifyLog(lines)

	want := []logLine{
		{kind: kindMeta},
		{kind: kindPlain, prefix: 57},
		{kind: kindWarn, prefix: 55},
		{kind: kindWarn},
		{kind: kindPlain, prefix: 46},
		{kind: kindError},
		{kind: kindError},
		{kind: kindError},
		{kind: kindError, prefix: 56},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("classifyLog =\n%+v\nwant\n%+v", got, want)
	}
}

func TestStyledLogLineKeepsItsTextUnderSearchHighlight(t *testing.T) {
	l := wrappedLine{text: "[t] {f.py:1} ERROR - boom: error again", kind: kindError, prefix: 21}

	got := ansi.Strip(l.render([][]int{{15, 20}, {33, 38}}))

	if got != l.text {
		t.Errorf("rendered text = %q, want %q", got, l.text)
	}
}
