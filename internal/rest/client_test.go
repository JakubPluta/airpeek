package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/oauth2"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

func TestGetSendsEscapedPathAndQueryUnderAPIv1(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_instances": []}`))
	}))
	defer srv.Close()

	body, err := NewClient(srv.URL+"/", nil).Get(context.Background(),
		"/dags/etl/dagRuns/manual__2026-11-01T06%3A00%3A00%2B00%3A00/taskInstances", url.Values{"limit": {"100"}})

	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"task_instances": []}` {
		t.Errorf("body = %s", body)
	}
	if got.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", got.Method)
	}
	if p := got.URL.EscapedPath(); p != "/api/v1/dags/etl/dagRuns/manual__2026-11-01T06%3A00%3A00%2B00%3A00/taskInstances" {
		t.Errorf("path = %s", p)
	}
	if q := got.URL.RawQuery; q != "limit=100" {
		t.Errorf("query = %s", q)
	}
}

func TestGetSendsCredentials(t *testing.T) {
	cases := []struct {
		name string
		auth Auth
		want string
	}{
		{"basic", Basic("viewer", "secret"), "Basic dmlld2VyOnNlY3JldA=="},
		{"bearer", Bearer("astro-token"), "Bearer astro-token"},
		{"token source", TokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "ya29.token"})), "Bearer ya29.token"},
	}
	for _, c := range cases {
		var header string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{}`))
		}))

		_, err := NewClient(srv.URL, c.auth).Get(context.Background(), "/dags", nil)
		srv.Close()

		if err != nil || header != c.want {
			t.Errorf("%s: Authorization = %q (err %v), want %q", c.name, header, err, c.want)
		}
	}
}

func TestGetReturnsPlainTextLogAsJSONString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("step 1 ok\n\"boom\""))
	}))
	defer srv.Close()

	body, err := NewClient(srv.URL, nil).Get(context.Background(), "/dags/etl/dagRuns/r/taskInstances/load/logs/1", nil)

	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `"step 1 ok\n\"boom\""` {
		t.Errorf("body = %s", body)
	}
}

func TestGetReturnsAirflowErrorWithItsTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail": null, "status": 401, "title": "Unauthorized"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, Basic("viewer", "wrong")).Get(context.Background(), "/dags", nil)

	var apiErr *airflow.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("err = %v, want an APIError with status 401", err)
	}
	if err.Error() != "airflow API returned 401: Unauthorized" {
		t.Errorf("message = %q", err.Error())
	}
}

func TestGetRejectsHTMLPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>Sign in</html>"))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, nil).Get(context.Background(), "/dags", nil)

	if err == nil {
		t.Fatal("an HTML page should be an error, not a body to decode")
	}
}
