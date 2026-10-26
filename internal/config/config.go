// Package config reads the Airflow environments airpeek can open from a
// YAML file. The file names environment variables for secrets and never
// holds a secret itself.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Environment is one Airflow to browse. Exactly one of HTTP and MWAA is set.
type Environment struct {
	Name string `yaml:"name"`
	// Highlight paints the header red, to make production unmistakable.
	Highlight bool  `yaml:"highlight,omitempty"`
	HTTP      *HTTP `yaml:"http,omitempty"`
	MWAA      *MWAA `yaml:"mwaa,omitempty"`
}

// HTTP reaches the Airflow webserver directly.
type HTTP struct {
	// URL is the address of the Airflow UI, without /api/v1.
	URL         string `yaml:"url"`
	Auth        Auth   `yaml:"auth"`
	UsernameEnv string `yaml:"username_env,omitempty"`
	PasswordEnv string `yaml:"password_env,omitempty"`
	TokenEnv    string `yaml:"token_env,omitempty"`
}

// Auth is how an HTTP environment authenticates.
type Auth string

// The ways to authenticate an HTTP environment.
const (
	// AuthNone sends no credentials.
	AuthNone Auth = "none"
	// AuthBasic sends the username and password from UsernameEnv and
	// PasswordEnv.
	AuthBasic Auth = "basic"
	// AuthToken sends the token from TokenEnv as a bearer token.
	AuthToken Auth = "token"
	// AuthGoogle sends a Google access token from Application Default
	// Credentials.
	AuthGoogle Auth = "google"
)

// MWAA reaches an Amazon MWAA environment through the AWS API. Profile and
// Region are optional and default to the AWS SDK's usual lookup.
type MWAA struct {
	Profile     string `yaml:"profile,omitempty"`
	Region      string `yaml:"region,omitempty"`
	Environment string `yaml:"environment"`
}

// DefaultPath is $AIRPEEK_CONFIG if set, else airpeek/config.yaml
// under $XDG_CONFIG_HOME, else under ~/.config.
func DefaultPath() (string, error) {
	if p := os.Getenv("AIRPEEK_CONFIG"); p != "" {
		return p, nil
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "airpeek", "config.yaml"), nil
}

// Load reads and validates the environments in the file at path.
func Load(path string) ([]Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	envs, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return envs, nil
}

func parse(data []byte) ([]Environment, error) {
	var file struct {
		Environments []Environment `yaml:"environments"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(file.Environments) == 0 {
		return nil, errors.New("no environments")
	}
	seen := make(map[string]bool)
	for i, e := range file.Environments {
		if e.Name == "" {
			return nil, fmt.Errorf("environment %d has no name", i+1)
		}
		if seen[e.Name] {
			return nil, fmt.Errorf("environment %q is defined twice", e.Name)
		}
		seen[e.Name] = true
		if err := e.validate(); err != nil {
			return nil, fmt.Errorf("environment %q: %w", e.Name, err)
		}
	}
	return file.Environments, nil
}

func (e Environment) validate() error {
	switch {
	case (e.HTTP == nil) == (e.MWAA == nil):
		return errors.New("set exactly one of http and mwaa")
	case e.MWAA != nil:
		if e.MWAA.Environment == "" {
			return errors.New("mwaa.environment is required")
		}
		return nil
	}
	return e.HTTP.validate()
}

func (h HTTP) validate() error {
	u, err := url.Parse(h.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("http.url must be an http(s) URL, got %q", h.URL)
	}
	if strings.Contains(u.Path, "/api/") {
		return fmt.Errorf("http.url is the address of the Airflow UI, without the /api/... part, got %q", h.URL)
	}
	switch h.Auth {
	case AuthNone, AuthGoogle:
		return nil
	case AuthBasic:
		if h.UsernameEnv == "" || h.PasswordEnv == "" {
			return errors.New("http.auth basic needs username_env and password_env")
		}
		return nil
	case AuthToken:
		if h.TokenEnv == "" {
			return errors.New("http.auth token needs token_env")
		}
		return nil
	}
	return fmt.Errorf("http.auth must be none, basic, token or google, got %q", h.Auth)
}

// Add appends e to the file at path, creating the file and its directory
// when needed. Comments in an existing file are kept.
func Add(path string, e Environment) error {
	if e.Name == "" {
		return errors.New("the environment has no name")
	}
	if err := e.validate(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("environments: []\n")
	} else {
		existing, err := parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, x := range existing {
			if x.Name == e.Name {
				return fmt.Errorf("%s already has an environment named %q", path, e.Name)
			}
		}
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	list := environmentsNode(&doc)
	if list == nil {
		return fmt.Errorf("%s: no environments list to add to", path)
	}
	var item yaml.Node
	if err := item.Encode(e); err != nil {
		return err
	}
	list.Style = 0 // block style, also when the list was written as []
	list.Content = append(list.Content, &item)

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o600)
}

// environmentsNode finds the environments list in a parsed document, or
// returns nil.
func environmentsNode(doc *yaml.Node) *yaml.Node {
	if len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "environments" && root.Content[i+1].Kind == yaml.SequenceNode {
			return root.Content[i+1]
		}
	}
	return nil
}
