package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JakubPluta/airpeek/internal/config"
)

// listMWAA returns the MWAA environments an AWS profile can see.
type listMWAA func(ctx context.Context, profile, region string) ([]string, error)

// runInit asks about one Airflow environment and adds it to the config file
// at path.
func runInit(in io.Reader, out io.Writer, path string, list listMWAA) error {
	p := &prompter{in: bufio.NewScanner(in), out: out}
	fmt.Fprintf(out, "Adding an Airflow environment to %s\n\n", path)

	var e config.Environment
	var name string
	switch p.choose("Where does it run?", []string{
		"Docker or Kubernetes (address, username and password)",
		"Astronomer (address and API token)",
		"Google Cloud Composer (address and your gcloud login)",
		"Amazon MWAA (AWS profile)",
	}) {
	case 1:
		e.HTTP = &config.HTTP{
			URL:         p.ask("Address of the Airflow UI", "http://localhost:8080"),
			Auth:        config.AuthBasic,
			UsernameEnv: p.ask("Environment variable with the username", "AIRFLOW_USERNAME"),
			PasswordEnv: p.ask("Environment variable with the password", "AIRFLOW_PASSWORD"),
		}
		name = "local"
	case 2:
		e.HTTP = &config.HTTP{
			URL:      p.ask("Airflow API URL of the Deployment, without /api/v1", ""),
			Auth:     config.AuthToken,
			TokenEnv: p.ask("Environment variable with the API token", "ASTRO_API_TOKEN"),
		}
		name = "astro"
	case 3:
		e.HTTP = &config.HTTP{URL: p.ask("Address of the Airflow UI (config.airflowUri)", ""), Auth: config.AuthGoogle}
		name = "composer"
	case 4:
		m := &config.MWAA{Profile: p.ask("AWS profile", "default"), Region: p.optional("AWS region (empty: the profile's)")}
		m.Environment = p.mwaaEnvironment(list, m.Profile, m.Region)
		e.MWAA = m
		name = m.Environment
	}
	e.Name = p.ask("Name for it in airpeek", name)
	e.Highlight = p.yes("Is it production? The header turns red")
	if p.err != nil {
		return p.err
	}

	if err := config.Add(path, e); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nSaved. Open it with: airpeek --env %s\n", e.Name)
	if e.HTTP != nil && e.HTTP.Auth == config.AuthBasic {
		fmt.Fprintf(out, "Set %s and %s first.\n", e.HTTP.UsernameEnv, e.HTTP.PasswordEnv)
	}
	if e.HTTP != nil && e.HTTP.Auth == config.AuthToken {
		fmt.Fprintf(out, "Set %s first.\n", e.HTTP.TokenEnv)
	}
	return nil
}

// prompter asks questions on a terminal. After the input ends every
// question returns the zero value and err says why.
type prompter struct {
	in  *bufio.Scanner
	out io.Writer
	err error
}

func (p *prompter) line(question string) string {
	if p.err != nil {
		return ""
	}
	fmt.Fprint(p.out, question)
	if !p.in.Scan() {
		p.err = p.in.Err()
		if p.err == nil {
			p.err = io.ErrUnexpectedEOF
		}
		return ""
	}
	return strings.TrimSpace(p.in.Text())
}

// ask repeats the question until it gets an answer; an empty answer takes
// def when there is one.
func (p *prompter) ask(question, def string) string {
	if def != "" {
		question += " [" + def + "]"
	}
	for p.err == nil {
		if a := p.line(question + ": "); a != "" {
			return a
		}
		if def != "" {
			return def
		}
	}
	return ""
}

func (p *prompter) optional(question string) string {
	return p.line(question + ": ")
}

func (p *prompter) yes(question string) bool {
	a := strings.ToLower(p.line(question + " [y/N]: "))
	return a == "y" || a == "yes"
}

// choose returns the number, from 1, of the option picked.
func (p *prompter) choose(question string, options []string) int {
	fmt.Fprintln(p.out, question)
	for i, o := range options {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, o)
	}
	for p.err == nil {
		n, err := strconv.Atoi(p.line(fmt.Sprintf("Choose 1-%d: ", len(options))))
		if err == nil && n >= 1 && n <= len(options) {
			return n
		}
	}
	return 0
}

// mwaaEnvironment lets the user pick from the environments the profile can
// see, or type a name when they cannot be listed.
func (p *prompter) mwaaEnvironment(list listMWAA, profile, region string) string {
	if p.err != nil {
		return ""
	}
	names, err := list(context.Background(), profile, region)
	switch {
	case err != nil:
		fmt.Fprintf(p.out, "Could not list MWAA environments: %v\n", err)
	case len(names) == 1:
		fmt.Fprintf(p.out, "Found one MWAA environment: %s\n", names[0])
		return names[0]
	case len(names) > 1:
		if n := p.choose("Which MWAA environment?", names); n > 0 {
			return names[n-1]
		}
		return ""
	}
	return p.ask("MWAA environment name", "")
}
