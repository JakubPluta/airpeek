package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"golang.org/x/oauth2/google"

	"github.com/JakubPluta/airpeek/internal/airflow"
	"github.com/JakubPluta/airpeek/internal/config"
	"github.com/JakubPluta/airpeek/internal/mwaa"
	"github.com/JakubPluta/airpeek/internal/rest"
	"github.com/JakubPluta/airpeek/internal/tui"
)

// fromConfigFile offers the environments of the config file at path. Secrets
// are read from the environment when an environment is opened, so a missing
// token for one does not stop the others.
func fromConfigFile(path string) (tui.Config, error) {
	envs, err := config.Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		return tui.Config{}, fmt.Errorf("no config file at %s; run `airpeek init` to add an environment", path)
	}
	if err != nil {
		return tui.Config{}, err
	}

	names := make([]string, len(envs))
	byName := make(map[string]config.Environment, len(envs))
	for i, e := range envs {
		names[i] = e.Name
		byName[e.Name] = e
	}
	return tui.Config{
		ListEnvironments: func(context.Context) ([]string, error) { return names, nil },
		Connect: func(name string) (tui.Environment, error) {
			e, ok := byName[name]
			if !ok {
				return tui.Environment{}, fmt.Errorf("no environment named %q in %s", name, path)
			}
			g, err := getter(e)
			return tui.Environment{Getter: g, Highlight: e.Highlight}, err
		},
	}, nil
}

func listMWAAEnvironments(ctx context.Context, profile, region string) ([]string, error) {
	cfg, err := awsConfig(profile, region)
	if err != nil {
		return nil, err
	}
	return mwaa.ListEnvironments(ctx, cfg)
}

func getter(e config.Environment) (airflow.Getter, error) {
	if e.MWAA != nil {
		cfg, err := awsConfig(e.MWAA.Profile, e.MWAA.Region)
		if err != nil {
			return nil, err
		}
		return mwaa.NewClient(cfg, e.MWAA.Environment), nil
	}
	auth, err := httpAuth(*e.HTTP)
	if err != nil {
		return nil, err
	}
	return rest.NewClient(e.HTTP.URL, auth), nil
}

func httpAuth(h config.HTTP) (rest.Auth, error) {
	switch h.Auth {
	case config.AuthNone:
		return nil, nil
	case config.AuthBasic:
		username, err := secret(h.UsernameEnv)
		if err != nil {
			return nil, err
		}
		password, err := secret(h.PasswordEnv)
		if err != nil {
			return nil, err
		}
		return rest.Basic(username, password), nil
	case config.AuthToken:
		token, err := secret(h.TokenEnv)
		if err != nil {
			return nil, err
		}
		return rest.Bearer(token), nil
	case config.AuthGoogle:
		ts, err := google.DefaultTokenSource(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, fmt.Errorf("google credentials: %w", err)
		}
		return rest.TokenSource(ts), nil
	}
	return nil, fmt.Errorf("unknown auth %q", h.Auth)
}

func secret(name string) (string, error) {
	if v := os.Getenv(name); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("environment variable %s is not set", name)
}

// awsConfig loads the AWS SDK config; an empty profile or region falls back
// to the SDK's usual lookup.
func awsConfig(profile, region string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load AWS config: %w", err)
	}
	return cfg, nil
}
