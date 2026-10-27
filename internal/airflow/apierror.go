package airflow

import (
	"cmp"
	"encoding/json"
	"fmt"
)

// APIError is a non-2xx answer from the Airflow REST API, whatever the
// transport that carried it.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	// Airflow answers with RFC 7807 problem details; detail is null on 401.
	var problem struct {
		Detail string `json:"detail"`
		Title  string `json:"title"`
	}
	if json.Unmarshal([]byte(e.Body), &problem) == nil {
		if msg := cmp.Or(problem.Detail, problem.Title); msg != "" {
			return fmt.Sprintf("airflow API returned %d: %s", e.Status, msg)
		}
	}
	return fmt.Sprintf("airflow API returned %d: %s", e.Status, e.Body)
}
