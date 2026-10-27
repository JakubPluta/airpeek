// Package airflow reads DAGs, runs, task instances and logs from the
// Airflow 2 REST API (/api/v1). Every function is a GET through Getter.
package airflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Getter fetches a path of the Airflow REST API (relative to /api/v1) and
// returns the JSON body. It has no way to express a method other than GET,
// which is what keeps this package read-only.
type Getter interface {
	Get(ctx context.Context, path string, query url.Values) ([]byte, error)
}

// pageSize stays at Airflow's default maximum_page_limit (100); asking for
// more is silently capped by the server.
const pageSize = 100

// ListDAGs returns every active DAG ordered by dag_id. All pages are loaded
// up front so the UI can filter locally without another round trip.
func ListDAGs(ctx context.Context, g Getter) ([]DAG, error) {
	var dags []DAG
	for {
		q := url.Values{}
		q.Set("only_active", "true")
		q.Set("order_by", "dag_id")
		q.Set("limit", strconv.Itoa(pageSize))
		q.Set("offset", strconv.Itoa(len(dags)))
		var page struct {
			DAGs         []DAG `json:"dags"`
			TotalEntries int   `json:"total_entries"`
		}
		if err := getJSON(ctx, g, "/dags", q, &page); err != nil {
			return nil, err
		}
		dags = append(dags, page.DAGs...)
		if len(page.DAGs) == 0 || len(dags) >= page.TotalEntries {
			return dags, nil
		}
	}
}

// ListDAGRuns returns the newest runs of a DAG, newest first.
func ListDAGRuns(ctx context.Context, g Getter, dagID string, limit int) ([]DAGRun, error) {
	q := url.Values{}
	q.Set("order_by", "-execution_date")
	q.Set("limit", strconv.Itoa(limit))
	var resp struct {
		DAGRuns []DAGRun `json:"dag_runs"`
	}
	err := getJSON(ctx, g, path("dags", dagID, "dagRuns"), q, &resp)
	return resp.DAGRuns, err
}

// ListTaskInstances returns all task instances of a run, mapped ones included.
func ListTaskInstances(ctx context.Context, g Getter, dagID, runID string) ([]TaskInstance, error) {
	var tis []TaskInstance
	for {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(pageSize))
		q.Set("offset", strconv.Itoa(len(tis)))
		var resp struct {
			TaskInstances []TaskInstance `json:"task_instances"`
			TotalEntries  int            `json:"total_entries"`
		}
		if err := getJSON(ctx, g, path("dags", dagID, "dagRuns", runID, "taskInstances"), q, &resp); err != nil {
			return nil, err
		}
		tis = append(tis, resp.TaskInstances...)
		if len(resp.TaskInstances) == 0 || len(tis) >= resp.TotalEntries {
			return tis, nil
		}
	}
}

// GetTaskLog returns the full log of one try of a task instance. mapIndex is
// -1 for unmapped tasks.
func GetTaskLog(ctx context.Context, g Getter, dagID, runID, taskID string, mapIndex, try int) (string, error) {
	q := url.Values{}
	q.Set("full_content", "true")
	if mapIndex >= 0 {
		q.Set("map_index", strconv.Itoa(mapIndex))
	}
	body, err := g.Get(ctx, path("dags", dagID, "dagRuns", runID, "taskInstances", taskID, "logs", strconv.Itoa(try)), q)
	if err != nil {
		return "", err
	}
	return decodeLog(body)
}

// decodeLog accepts both shapes MWAA can hand back for a log: a plain text
// body (a JSON string) or Airflow's JSON {"content": ..., "continuation_token": ...}.
func decodeLog(body []byte) (string, error) {
	var text string
	if err := json.Unmarshal(body, &text); err == nil {
		return text, nil
	}
	var obj struct {
		Content *string `json:"content"`
	}
	if err := json.Unmarshal(body, &obj); err != nil || obj.Content == nil {
		return "", fmt.Errorf("unexpected log response: %.200s", body)
	}
	return *obj.Content, nil
}

// ListImportErrors returns DAG files the scheduler failed to parse.
func ListImportErrors(ctx context.Context, g Getter) ([]ImportError, error) {
	q := url.Values{}
	q.Set("order_by", "filename")
	q.Set("limit", strconv.Itoa(pageSize))
	var resp struct {
		ImportErrors []ImportError `json:"import_errors"`
	}
	err := getJSON(ctx, g, "/importErrors", q, &resp)
	return resp.ImportErrors, err
}

func getJSON(ctx context.Context, g Getter, p string, q url.Values, out any) error {
	body, err := g.Get(ctx, p, q)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", p, err)
	}
	return nil
}

// path joins segments into an API path, percent-encoding everything but
// unreserved characters, because run ids carry ':' and '+'
// (scheduled__2026-10-31T06:00:00+00:00) and ids may in theory carry '/'.
func path(segments ...string) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteByte('/')
		for i := 0; i < len(s); i++ {
			c := s[i]
			if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
				b.WriteByte(c)
			} else {
				fmt.Fprintf(&b, "%%%02X", c)
			}
		}
	}
	return b.String()
}
