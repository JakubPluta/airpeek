package airflow

import (
	"context"
	"net/url"
	"reflect"
	"testing"
)

type call struct {
	path  string
	query url.Values
}

// fakeGetter answers calls in order with the given bodies and records them.
type fakeGetter struct {
	bodies []string
	calls  []call
}

func (f *fakeGetter) Get(_ context.Context, path string, query url.Values) ([]byte, error) {
	f.calls = append(f.calls, call{path, query})
	body := f.bodies[0]
	f.bodies = f.bodies[1:]
	return []byte(body), nil
}

func TestListDAGsLoadsEveryPage(t *testing.T) {
	g := &fakeGetter{bodies: []string{
		`{"dags": [{"dag_id": "a"}, {"dag_id": "b"}], "total_entries": 3}`,
		`{"dags": [{"dag_id": "c"}], "total_entries": 3}`,
	}}

	dags, err := ListDAGs(context.Background(), g)

	if err != nil {
		t.Fatal(err)
	}
	if got := []string{dags[0].DAGID, dags[1].DAGID, dags[2].DAGID}; !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("dag ids = %v", got)
	}
	if got := []string{g.calls[0].query.Get("offset"), g.calls[1].query.Get("offset")}; !reflect.DeepEqual(got, []string{"0", "2"}) {
		t.Errorf("offsets = %v", got)
	}
	if g.calls[0].path != "/dags" || g.calls[0].query.Get("only_active") != "true" {
		t.Errorf("first call = %+v", g.calls[0])
	}
}

func TestScheduleTextPrefersCronOverTimetableDescription(t *testing.T) {
	g := &fakeGetter{bodies: []string{`{"total_entries": 3, "dags": [
		{"dag_id": "cron", "schedule_interval": {"__type": "CronExpression", "value": "0 6 * * *"}, "timetable_description": "At 06:00"},
		{"dag_id": "delta", "schedule_interval": {"__type": "TimeDelta", "days": 1, "seconds": 0, "microseconds": 0}, "timetable_description": "Every day"},
		{"dag_id": "manual", "schedule_interval": null, "timetable_description": null}
	]}`}}

	dags, err := ListDAGs(context.Background(), g)

	if err != nil {
		t.Fatal(err)
	}
	got := []string{dags[0].ScheduleText(), dags[1].ScheduleText(), dags[2].ScheduleText()}
	if !reflect.DeepEqual(got, []string{"0 6 * * *", "Every day", "None"}) {
		t.Errorf("schedules = %v", got)
	}
}

func TestListTaskInstancesLoadsEveryPage(t *testing.T) {
	g := &fakeGetter{bodies: []string{
		`{"task_instances": [{"task_id": "extract", "map_index": -1, "state": "success", "try_number": 1}], "total_entries": 2}`,
		`{"task_instances": [{"task_id": "load", "map_index": -1, "state": null, "try_number": 0}], "total_entries": 2}`,
	}}

	tis, err := ListTaskInstances(context.Background(), g, "etl_users", "manual__2026-10-31T06:00:00+00:00")

	if err != nil {
		t.Fatal(err)
	}
	if got := []string{tis[0].TaskID + "=" + tis[0].StateText(), tis[1].TaskID + "=" + tis[1].StateText()}; !reflect.DeepEqual(got, []string{"extract=success", "load=none"}) {
		t.Errorf("task states = %v", got)
	}
	if want := "/dags/etl_users/dagRuns/manual__2026-10-31T06%3A00%3A00%2B00%3A00/taskInstances"; g.calls[0].path != want {
		t.Errorf("path = %s, want %s", g.calls[0].path, want)
	}
}

func TestGetTaskLogEscapesRunIDAndSendsMapIndexOnlyForMappedTasks(t *testing.T) {
	g := &fakeGetter{bodies: []string{`"unmapped log"`, `"mapped log"`}}
	runID := "scheduled__2026-10-31T06:00:00+00:00"

	unmapped, err := GetTaskLog(context.Background(), g, "etl_users", runID, "load", -1, 2)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := GetTaskLog(context.Background(), g, "etl_users", runID, "load", 7, 1)
	if err != nil {
		t.Fatal(err)
	}

	if unmapped != "unmapped log" || mapped != "mapped log" {
		t.Errorf("logs = %q, %q", unmapped, mapped)
	}
	if want := "/dags/etl_users/dagRuns/scheduled__2026-10-31T06%3A00%3A00%2B00%3A00/taskInstances/load/logs/2"; g.calls[0].path != want {
		t.Errorf("path = %s, want %s", g.calls[0].path, want)
	}
	if g.calls[0].query.Has("map_index") || g.calls[1].query.Get("map_index") != "7" {
		t.Errorf("map_index queries = %v, %v", g.calls[0].query, g.calls[1].query)
	}
	if g.calls[0].query.Get("full_content") != "true" {
		t.Errorf("full_content = %q", g.calls[0].query.Get("full_content"))
	}
}

func TestDecodeLogAcceptsTextAndJSONContent(t *testing.T) {
	text, err := decodeLog([]byte(`"line 1\nline 2"`))
	if err != nil || text != "line 1\nline 2" {
		t.Errorf("text body: %q, %v", text, err)
	}
	content, err := decodeLog([]byte(`{"content": "line 1", "continuation_token": "abc"}`))
	if err != nil || content != "line 1" {
		t.Errorf("json body: %q, %v", content, err)
	}
	if _, err := decodeLog([]byte(`{"detail": "nope"}`)); err == nil {
		t.Error("expected an error for a body without content")
	}
}
