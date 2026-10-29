package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func tableWithRows(n, height int) table {
	t := table{cols: []column{{title: "NAME"}}}
	rows := make([][]cell, n)
	for i := range rows {
		rows[i] = []cell{plain("row")}
	}
	t.setRows(rows)
	t.setSize(40, height)
	return t
}

func TestTableScrollsToKeepCursorVisible(t *testing.T) {
	tbl := tableWithRows(10, 4) // header + 3 rows

	for range 5 {
		tbl.update(tea.KeyMsg{Type: tea.KeyDown})
	}

	if from, to := tbl.visible(); tbl.cursor != 5 || from != 3 || to != 6 {
		t.Errorf("cursor=%d visible=[%d,%d), want cursor=5 visible=[3,6)", tbl.cursor, from, to)
	}
}

func TestTableClampsCursorWhenRowsShrink(t *testing.T) {
	tbl := tableWithRows(10, 4)
	tbl.update(tea.KeyMsg{Type: tea.KeyEnd})

	tbl.setRows([][]cell{{plain("a")}, {plain("b")}})

	if from, to := tbl.visible(); tbl.cursor != 1 || from != 0 || to != 2 {
		t.Errorf("cursor=%d visible=[%d,%d), want cursor=1 visible=[0,2)", tbl.cursor, from, to)
	}
}

func TestFitPadsAndTruncatesToWidth(t *testing.T) {
	if got := fit("abc", 5); got != "abc  " {
		t.Errorf("fit pad = %q", got)
	}
	if got := fit("abcdef", 4); got != "abc…" {
		t.Errorf("fit truncate = %q", got)
	}
}
