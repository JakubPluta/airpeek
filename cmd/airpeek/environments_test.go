package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFileEnvironmentNeedsItsSecretOnlyWhenOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(path, []byte(`
environments:
  - name: local
    http: {url: "http://localhost:8080", auth: basic, username_env: TEST_AIRFLOW_USERNAME, password_env: TEST_AIRFLOW_PASSWORD}
  - name: astro
    highlight: true
    http: {url: "https://example.astronomer.run/d1b2c3", auth: token, token_env: TEST_ASTRO_API_TOKEN}
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_AIRFLOW_USERNAME", "viewer")
	t.Setenv("TEST_AIRFLOW_PASSWORD", "viewer")
	t.Setenv("TEST_ASTRO_API_TOKEN", "")

	cfg, err := fromConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := cfg.Connect("astro"); err == nil || err.Error() != "environment variable TEST_ASTRO_API_TOKEN is not set" {
		t.Errorf("astro: err = %v", err)
	}
	if local, err := cfg.Connect("local"); err != nil || local.Getter == nil || local.Highlight {
		t.Errorf("local = %+v, err = %v", local, err)
	}
}

func TestMissingConfigFileSaysWhatToDo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	_, err := fromConfigFile(path)

	want := "no config file at " + path + "; run `airpeek init` to add an environment"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}
