package tui

import (
	"testing"
	"time"
)

func TestGanttBarPlacesTasksOnTheAxis(t *testing.T) {
	from := time.Date(2026, 11, 1, 6, 0, 0, 0, time.UTC)
	to := from.Add(8 * time.Second)
	at := func(sec int) time.Time { return from.Add(time.Duration(sec) * time.Second) }

	cases := []struct {
		name       string
		start, end time.Time
		want       string
	}{
		{"first half", at(0), at(4), "██"},
		{"second half", at(4), at(8), "  ██"},
		{"one second", at(2), at(3), " ▌"},
		{"starts mid-cell", at(1), at(4), "▐█"},
		{"zero length still shows", at(0), at(0), "▏"},
	}
	for _, c := range cases {
		if got := ganttBar(c.start, c.end, from, to, 4); got != c.want {
			t.Errorf("%s: ganttBar = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTimelineSpanCoversStartedTasksAndRunsToNow(t *testing.T) {
	start1 := time.Date(2026, 11, 1, 6, 0, 0, 0, time.UTC)
	end1 := time.Date(2026, 11, 1, 6, 1, 0, 0, time.UTC)
	start2 := time.Date(2026, 11, 1, 6, 0, 30, 0, time.UTC)
	now := time.Date(2026, 11, 1, 6, 5, 0, 0, time.UTC)

	from, to, ok := timelineSpan([]*time.Time{&start1, &start2, nil}, []*time.Time{&end1, nil, nil}, now)

	if !ok || !from.Equal(start1) || !to.Equal(now) {
		t.Errorf("span = %v → %v (ok=%v), want %v → %v", from, to, ok, start1, now)
	}
	if _, _, ok := timelineSpan([]*time.Time{nil}, []*time.Time{nil}, now); ok {
		t.Error("span of tasks that never started should not be ok")
	}
}
