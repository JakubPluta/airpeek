package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// textView is a scrollable, wrapped, searchable block of text, used for
// task logs and import error stack traces. Lines are colored by classifyLog.
type textView struct {
	source   []string
	kinds    []logLine
	lines    []wrappedLine
	vp       viewport.Model
	search   textinput.Model
	query    *regexp.Regexp
	matches  []int // indexes into lines
	matchPos int
}

// wrappedLine is one screen line of a source line, kept as plain text so
// search runs on what the user reads, never on escape codes.
type wrappedLine struct {
	text   string
	kind   lineKind
	prefix int
}

func newTextView() textView {
	return textView{vp: viewport.New(0, 0), search: newInput("search (case-insensitive)")}
}

func (t *textView) setText(s string) {
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ").Replace(s)
	t.source = strings.Split(s, "\n")
	t.kinds = classifyLog(t.source)
	t.render()
}

func (t *textView) setSize(w, h int) {
	t.vp.Width, t.vp.Height = w, max(h-1, 1)
	t.render()
}

// render wraps the text to the viewport width, colors it and highlights
// search hits.
func (t *textView) render() {
	if t.vp.Width <= 0 {
		return
	}
	t.lines = t.lines[:0]
	for i, src := range t.source {
		for j, piece := range strings.Split(ansi.Hardwrap(src, t.vp.Width, true), "\n") {
			l := wrappedLine{text: piece, kind: t.kinds[i].kind}
			if j == 0 {
				l.prefix = min(t.kinds[i].prefix, len(piece))
			}
			t.lines = append(t.lines, l)
		}
	}
	t.matches = t.matches[:0]
	shown := make([]string, len(t.lines))
	for i, l := range t.lines {
		var hits [][]int
		if t.query != nil {
			if hits = t.query.FindAllStringIndex(l.text, -1); hits != nil {
				t.matches = append(t.matches, i)
			}
		}
		shown[i] = l.render(hits)
	}
	t.matchPos = min(t.matchPos, max(len(t.matches)-1, 0))
	t.vp.SetContent(strings.Join(shown, "\n"))
}

// render styles the line: the log prefix dimmed, the rest by kind, and
// search hits highlighted on top of either.
func (l wrappedLine) render(hits [][]int) string {
	if l.kind == kindPlain && l.prefix == 0 && hits == nil {
		return l.text
	}
	cuts := []int{0, l.prefix, len(l.text)}
	for _, h := range hits {
		cuts = append(cuts, h[0], h[1])
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	var b strings.Builder
	for i := 0; i+1 < len(cuts); i++ {
		from, to := cuts[i], cuts[i+1]
		style := logStyles[l.kind]
		if from < l.prefix {
			style = dimStyle
		}
		if inHit(hits, from) {
			style = matchStyle.Inherit(style)
		}
		b.WriteString(style.Render(l.text[from:to]))
	}
	return b.String()
}

func inHit(hits [][]int, pos int) bool {
	for _, h := range hits {
		if h[0] <= pos && pos < h[1] {
			return true
		}
	}
	return false
}

func (t *textView) update(msg tea.KeyMsg) tea.Cmd {
	if t.search.Focused() {
		switch msg.String() {
		case "enter":
			t.search.Blur()
			t.query = nil
			if q := t.search.Value(); q != "" {
				t.query = regexp.MustCompile("(?i)" + regexp.QuoteMeta(q))
			}
			t.render()
			t.jumpFrom(t.vp.YOffset, 1)
		case "esc":
			t.search.Blur()
		default:
			var cmd tea.Cmd
			t.search, cmd = t.search.Update(msg)
			return cmd
		}
		return nil
	}
	switch msg.String() {
	case "/":
		t.search.SetValue("")
		return t.search.Focus()
	case "n":
		t.step(1)
	case "N":
		t.step(-1)
	case "g", "home":
		t.vp.GotoTop()
	case "G", "end":
		t.vp.GotoBottom()
	default:
		var cmd tea.Cmd
		t.vp, cmd = t.vp.Update(msg)
		return cmd
	}
	return nil
}

// mouse scrolls the text with the wheel.
func (t *textView) mouse(msg tea.MouseMsg) tea.Cmd {
	var cmd tea.Cmd
	t.vp, cmd = t.vp.Update(msg)
	return cmd
}

// jumpFrom moves to the first match at or after line (dir 1) or at or before
// it (dir -1), wrapping around the ends.
func (t *textView) jumpFrom(line, dir int) {
	if len(t.matches) == 0 {
		return
	}
	pos := -1
	for i, m := range t.matches {
		if dir > 0 && m >= line {
			pos = i
			break
		}
		if dir < 0 && m <= line {
			pos = i
		}
	}
	if pos < 0 {
		pos = 0
		if dir < 0 {
			pos = len(t.matches) - 1
		}
	}
	t.matchPos = pos
	t.vp.SetYOffset(t.matches[pos])
}

func (t *textView) step(dir int) {
	if len(t.matches) == 0 {
		return
	}
	t.matchPos = (t.matchPos + dir + len(t.matches)) % len(t.matches)
	t.vp.SetYOffset(t.matches[t.matchPos])
}

func (t *textView) view() string {
	status := fmt.Sprintf("%d lines · %3.0f%%", len(t.source), t.vp.ScrollPercent()*100)
	switch {
	case t.query == nil:
	case len(t.matches) == 0:
		status = fmt.Sprintf("no matches for %q · %s", t.search.Value(), status)
	default:
		status = fmt.Sprintf("match %d of %d · %s", t.matchPos+1, len(t.matches), status)
	}
	top := dimStyle.Render(status)
	if t.search.Focused() {
		top = t.search.View()
	}
	return top + "\n" + t.vp.View()
}

func (t *textView) capturing() bool { return t.search.Focused() }

func (t *textView) help() string {
	if t.search.Focused() {
		return "type to search · enter find · esc cancel"
	}
	return "↑↓/pgup/pgdn/wheel scroll · g/G top/bottom · / search · n/N next/prev"
}
