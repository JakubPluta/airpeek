package airflow

import "time"

// DAG is a row of GET /dags. Only the fields shown in the UI are decoded.
type DAG struct {
	DAGID            string     `json:"dag_id"`
	IsPaused         bool       `json:"is_paused"`
	ScheduleInterval *Schedule  `json:"schedule_interval"`
	TimetableDesc    string     `json:"timetable_description"`
	NextDagrun       *time.Time `json:"next_dagrun"`
	Tags             []Tag      `json:"tags"`
	HasImportErrors  bool       `json:"has_import_errors"`
}

// Schedule is Airflow's tagged schedule_interval, e.g.
// {"__type": "CronExpression", "value": "0 6 * * *"}.
type Schedule struct {
	Type  string `json:"__type"`
	Value string `json:"value"`
}

// Tag is a DAG tag; DAGs carry them as objects, not plain strings.
type Tag struct {
	Name string `json:"name"`
}

// ScheduleText is the cron expression when there is one, otherwise Airflow's
// own description of the timetable (timedelta, dataset, custom timetables).
func (d DAG) ScheduleText() string {
	if d.ScheduleInterval != nil && d.ScheduleInterval.Type == "CronExpression" {
		return d.ScheduleInterval.Value
	}
	if d.TimetableDesc != "" {
		return d.TimetableDesc
	}
	return "None"
}

// DAGRun is a row of GET /dags/{dag_id}/dagRuns.
type DAGRun struct {
	DAGRunID    string     `json:"dag_run_id"`
	DAGID       string     `json:"dag_id"`
	RunType     string     `json:"run_type"`
	State       string     `json:"state"`
	LogicalDate *time.Time `json:"logical_date"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
}

// TaskInstance is a row of GET /dags/{dag_id}/dagRuns/{run_id}/taskInstances.
type TaskInstance struct {
	TaskID    string     `json:"task_id"`
	MapIndex  int        `json:"map_index"`
	State     *string    `json:"state"`
	TryNumber int        `json:"try_number"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	Operator  string     `json:"operator"`
}

// StateText is the task state, or "none" for tasks that were never scheduled.
func (t TaskInstance) StateText() string {
	if t.State == nil {
		return "none"
	}
	return *t.State
}

// ImportError is a row of GET /importErrors.
type ImportError struct {
	Filename   string     `json:"filename"`
	StackTrace string     `json:"stack_trace"`
	Timestamp  *time.Time `json:"timestamp"`
}
