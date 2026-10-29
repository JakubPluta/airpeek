package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// wheelStep is how many rows one mouse wheel notch moves the cursor.
const wheelStep = 3

// column width 0 means "take the remaining width"; at most one column may
// use it.
type column struct {
	title string
	width int
}

// cell carries its own style so states can be colored; bubbles/table
// truncates by bytes and breaks on ANSI sequences, hence this small table.
// A cell with spans is drawn from differently styled pieces instead of text,
// e.g. the colored squares of recent runs.
type cell struct {
	text  string
	style lipgloss.Style
	spans []span
}

type span struct {
	text  string
	style lipgloss.Style
}

func plain(s string) cell { return cell{text: s, style: lipgloss.NewStyle()} }

type table struct {
	cols   []column
	rows   [][]cell
	cursor int
	offset int
	width  int
	height int
}

var tableKeys = struct {
	up, down, pgUp, pgDown, top, bottom key.Binding
}{
	up:     key.NewBinding(key.WithKeys("up", "k")),
	down:   key.NewBinding(key.WithKeys("down", "j")),
	pgUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u")),
	pgDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+d")),
	top:    key.NewBinding(key.WithKeys("home", "g")),
	bottom: key.NewBinding(key.WithKeys("end", "G")),
}

func (t *table) setRows(rows [][]cell) {
	t.rows = rows
	t.cursor = min(t.cursor, max(len(rows)-1, 0))
	t.clampOffset()
}

func (t *table) setSize(w, h int) {
	t.width, t.height = w, max(h, 2)
	t.clampOffset()
}

// bodyHeight is the number of rows that fit below the header.
func (t *table) bodyHeight() int { return t.height - 1 }

// visible returns the [from, to) range of rows on screen.
func (t *table) visible() (int, int) {
	return t.offset, min(t.offset+t.bodyHeight(), len(t.rows))
}

// update moves the cursor and reports whether the key was handled.
func (t *table) update(msg tea.KeyMsg) bool {
	switch {
	case key.Matches(msg, tableKeys.up):
		t.move(-1)
	case key.Matches(msg, tableKeys.down):
		t.move(1)
	case key.Matches(msg, tableKeys.pgUp):
		t.move(-t.bodyHeight())
	case key.Matches(msg, tableKeys.pgDown):
		t.move(t.bodyHeight())
	case key.Matches(msg, tableKeys.top):
		t.move(-len(t.rows))
	case key.Matches(msg, tableKeys.bottom):
		t.move(len(t.rows))
	default:
		return false
	}
	return true
}

// mouse scrolls on the wheel and selects the clicked row. y is relative to
// the table's header line. It reports whether the click landed on the row
// that was already selected, which callers treat as "open".
func (t *table) mouse(msg tea.MouseMsg, y int) (open bool) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		t.move(-wheelStep)
	case msg.Button == tea.MouseButtonWheelDown:
		t.move(wheelStep)
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		row := t.offset + y - 1
		if y < 1 || row >= len(t.rows) {
			return false
		}
		if row == t.cursor {
			return true
		}
		t.cursor = row
		t.clampOffset()
	}
	return false
}

func (t *table) move(n int) {
	t.cursor = max(min(t.cursor+n, len(t.rows)-1), 0)
	t.clampOffset()
}

func (t *table) clampOffset() {
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+t.bodyHeight() {
		t.offset = t.cursor - t.bodyHeight() + 1
	}
	t.offset = max(min(t.offset, len(t.rows)-t.bodyHeight()), 0)
}

func (t *table) widths() []int {
	ws := make([]int, len(t.cols))
	used := len(t.cols) - 1 // one space between columns
	for i, c := range t.cols {
		ws[i] = c.width
		used += c.width
	}
	for i, c := range t.cols {
		if c.width == 0 {
			ws[i] = max(t.width-used, 10)
		}
	}
	return ws
}

func (t *table) view() string {
	ws := t.widths()
	var b strings.Builder
	header := make([]string, len(t.cols))
	for i, c := range t.cols {
		header[i] = fit(c.title, ws[i])
	}
	b.WriteString(headerStyle.Render(strings.Join(header, " ")))
	from, to := t.visible()
	for r := from; r < to; r++ {
		b.WriteByte('\n')
		selected := r == t.cursor
		parts := make([]string, len(t.cols))
		for i := range t.cols {
			c := plain("")
			if i < len(t.rows[r]) {
				c = t.rows[r][i]
			}
			parts[i] = renderCell(c, ws[i], selected)
		}
		sep := " "
		if selected {
			sep = selectedStyle.Render(" ")
		}
		b.WriteString(strings.Join(parts, sep))
	}
	if len(t.rows) == 0 {
		b.WriteString("\n" + dimStyle.Render("  (empty)"))
	}
	return b.String()
}

// renderCell draws c in exactly w cells. Spans are styled one by one so the
// selection background survives between them.
func renderCell(c cell, w int, selected bool) string {
	sel := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return s.Inherit(selectedStyle)
		}
		return s
	}
	if c.spans == nil {
		return sel(c.style).Render(fit(c.text, w))
	}
	var b strings.Builder
	width := 0
	for _, s := range c.spans {
		width += ansi.StringWidth(s.text)
	}
	if width > w {
		var text strings.Builder
		for _, s := range c.spans {
			text.WriteString(s.text)
		}
		return sel(c.style).Render(fit(text.String(), w))
	}
	for _, s := range c.spans {
		b.WriteString(sel(s.style).Render(s.text))
	}
	b.WriteString(sel(c.style).Render(strings.Repeat(" ", w-width)))
	return b.String()
}

// fit truncates or pads plain text to exactly w cells.
func fit(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
