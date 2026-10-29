package tui

import (
	"math"
	"strings"
	"time"
)

// eighths[n] is a block filling n eighths of a cell from the left.
var eighths = [...]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// ganttBar draws [start, end) on an axis of width cells spanning [from, to].
// It works in eighths of a cell, so even a task of a few seconds in an
// hour-long run stays visible.
func ganttBar(start, end, from, to time.Time, width int) string {
	if width <= 0 || !to.After(from) {
		return ""
	}
	scale := float64(width*8) / float64(to.Sub(from))
	s := int(float64(start.Sub(from)) * scale)
	e := max(int(math.Ceil(float64(end.Sub(from))*scale)), s+1)
	var b strings.Builder
	for c := range width {
		lo, hi := c*8, c*8+8
		fill := min(e, hi) - max(s, lo)
		switch {
		case fill <= 0:
			b.WriteByte(' ')
		case fill >= 8:
			b.WriteString("█")
		case s > lo:
			// Unicode has no blocks filled from the right but the half one.
			b.WriteString("▐")
		default:
			b.WriteString(eighths[fill])
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// timelineSpan is the stretch of time the started tasks cover; running
// tasks extend it to now. ok is false when nothing has started yet.
func timelineSpan(starts, ends []*time.Time, now time.Time) (from, to time.Time, ok bool) {
	for i, s := range starts {
		if s == nil {
			continue
		}
		end := now
		if ends[i] != nil {
			end = *ends[i]
		}
		if !ok || s.Before(from) {
			from = *s
		}
		if !ok || end.After(to) {
			to = end
		}
		ok = true
	}
	return from, to, ok
}
