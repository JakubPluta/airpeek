package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noMWAA(context.Context, string, string) ([]string, error) {
	return nil, errors.New("not expected")
}

func TestInitDockerTakesTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	// Docker, then enter on every question with a default, then no.
	answers := "1\n\n\n\n\n\n"
	var out strings.Builder

	err := runInit(strings.NewReader(answers), &out, path, noMWAA)

	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := `environments:
  - name: local
    http:
      url: http://localhost:8080
      auth: basic
      username_env: AIRFLOW_USERNAME
      password_env: AIRFLOW_PASSWORD
`
	if string(got) != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
	if !strings.HasSuffix(out.String(), "Saved. Open it with: airpeek --env local\nSet AIRFLOW_USERNAME and AIRFLOW_PASSWORD first.\n") {
		t.Errorf("output ends with:\n%s", out.String())
	}
}

func TestInitMWAAPicksFromTheListedEnvironments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	list := func(_ context.Context, profile, region string) ([]string, error) {
		if profile != "prod" || region != "" {
			t.Errorf("listed with profile %q, region %q", profile, region)
		}
		return []string{"analytics", "ingest"}, nil
	}
	// MWAA, profile prod, default region, a wrong pick then ingest, default name, production.
	answers := "4\nprod\n\n7\n2\n\ny\n"

	err := runInit(strings.NewReader(answers), &strings.Builder{}, path, list)

	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := `environments:
  - name: ingest
    highlight: true
    mwaa:
      profile: prod
      environment: ingest
`
	if string(got) != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestInitStopsWhenInputEnds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	err := runInit(strings.NewReader("2\n"), &strings.Builder{}, path, noMWAA)

	if err == nil {
		t.Fatal("want an error when the answers run out")
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("nothing should be written, stat = %v", statErr)
	}
}
