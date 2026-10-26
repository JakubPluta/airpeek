package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeConfig(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadReadsEveryKindOfEnvironment(t *testing.T) {
	envs, err := Load(writeConfig(t, `
environments:
  - name: local
    http:
      url: http://localhost:8080
      auth: basic
      username_env: AIRFLOW_USERNAME
      password_env: AIRFLOW_PASSWORD
  - name: astro
    http: {url: "https://example.astronomer.run/d1b2c3", auth: token, token_env: ASTRO_API_TOKEN}
  - name: composer
    http: {url: "https://abc-dot-europe-west1.composer.googleusercontent.com", auth: google}
  - name: aws
    highlight: true
    mwaa: {profile: default, region: us-east-1, environment: my-env}
`))

	if err != nil {
		t.Fatal(err)
	}
	want := []Environment{
		{Name: "local", HTTP: &HTTP{URL: "http://localhost:8080", Auth: AuthBasic, UsernameEnv: "AIRFLOW_USERNAME", PasswordEnv: "AIRFLOW_PASSWORD"}},
		{Name: "astro", HTTP: &HTTP{URL: "https://example.astronomer.run/d1b2c3", Auth: AuthToken, TokenEnv: "ASTRO_API_TOKEN"}},
		{Name: "composer", HTTP: &HTTP{URL: "https://abc-dot-europe-west1.composer.googleusercontent.com", Auth: AuthGoogle}},
		{Name: "aws", Highlight: true, MWAA: &MWAA{Profile: "default", Region: "us-east-1", Environment: "my-env"}},
	}
	if !reflect.DeepEqual(envs, want) {
		t.Errorf("Load =\n%+v\nwant\n%+v", envs, want)
	}
}

func TestLoadRejectsInvalidEnvironments(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{"empty file", ``, "no environments"},
		{"no name", `environments: [{http: {url: "http://localhost:8080", auth: none}}]`,
			"environment 1 has no name"},
		{"duplicate", `environments: [{name: a, http: {url: "http://h", auth: none}}, {name: a, http: {url: "http://h", auth: none}}]`,
			`environment "a" is defined twice`},
		{"both transports", `environments: [{name: a, http: {url: "http://h", auth: none}, mwaa: {environment: e}}]`,
			`environment "a": set exactly one of http and mwaa`},
		{"mwaa without environment", `environments: [{name: a, mwaa: {profile: default}}]`,
			`environment "a": mwaa.environment is required`},
		{"url with api path", `environments: [{name: a, http: {url: "http://localhost:8080/api/v1", auth: none}}]`,
			`environment "a": http.url is the address of the Airflow UI, without the /api/... part, got "http://localhost:8080/api/v1"`},
		{"no scheme", `environments: [{name: a, http: {url: "localhost:8080", auth: none}}]`,
			`environment "a": http.url must be an http(s) URL, got "localhost:8080"`},
		{"basic without password", `environments: [{name: a, http: {url: "http://h", auth: basic, username_env: U}}]`,
			`environment "a": http.auth basic needs username_env and password_env`},
		{"unknown auth", `environments: [{name: a, http: {url: "http://h", auth: oauth}}]`,
			`environment "a": http.auth must be none, basic, token or google, got "oauth"`},
		{"missing auth", `environments: [{name: a, http: {url: "http://h"}}]`,
			`environment "a": http.auth must be none, basic, token or google, got ""`},
		{"secret in the file", `environments: [{name: a, http: {url: "http://h", auth: token, token: abc}}]`,
			"yaml: unmarshal errors:\n  line 1: field token not found in type config.HTTP"},
	}
	for _, c := range cases {
		path := writeConfig(t, c.yaml)

		_, err := Load(path)

		if err == nil || err.Error() != path+": "+c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, path+": "+c.want)
		}
	}
}

func TestDefaultPathFollowsEnvironment(t *testing.T) {
	t.Setenv("AIRPEEK_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if p, _ := DefaultPath(); p != "/xdg/airpeek/config.yaml" {
		t.Errorf("with XDG_CONFIG_HOME: %s", p)
	}

	t.Setenv("AIRPEEK_CONFIG", "/etc/airpeek.yaml")
	if p, _ := DefaultPath(); p != "/etc/airpeek.yaml" {
		t.Errorf("with AIRPEEK_CONFIG: %s", p)
	}
}

func TestAddCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "airpeek", "config.yaml")

	err := Add(path, Environment{Name: "local", HTTP: &HTTP{URL: "http://localhost:8080", Auth: AuthBasic, UsernameEnv: "AIRFLOW_USERNAME", PasswordEnv: "AIRFLOW_PASSWORD"}})

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
}

func TestAddAppendsAndKeepsComments(t *testing.T) {
	path := writeConfig(t, `# my Airflows
environments:
  - name: local # docker compose
    http: {url: "http://localhost:8080", auth: none}
`)

	err := Add(path, Environment{Name: "aws", Highlight: true, MWAA: &MWAA{Profile: "prod", Environment: "my-environment"}})

	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := `# my Airflows
environments:
  - name: local # docker compose
    http: {url: "http://localhost:8080", auth: none}
  - name: aws
    highlight: true
    mwaa:
      profile: prod
      environment: my-environment
`
	if string(got) != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestAddRefusesADuplicateName(t *testing.T) {
	path := writeConfig(t, `environments: [{name: local, http: {url: "http://localhost:8080", auth: none}}]`)

	err := Add(path, Environment{Name: "local", HTTP: &HTTP{URL: "http://localhost:9090", Auth: AuthNone}})

	if err == nil || err.Error() != path+` already has an environment named "local"` {
		t.Errorf("err = %v", err)
	}
}
