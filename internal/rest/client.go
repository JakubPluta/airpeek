// Package rest reaches the Airflow REST API of a webserver over HTTP(S):
// Airflow in Docker or Kubernetes, Astronomer, Google Cloud Composer. It can
// only issue GET requests.
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

// Auth adds credentials to a request.
type Auth func(r *http.Request) error

// Basic authenticates with a username and password; the webserver needs
// Airflow's basic_auth API backend enabled.
func Basic(username, password string) Auth {
	return func(r *http.Request) error {
		r.SetBasicAuth(username, password)
		return nil
	}
}

// Bearer sends a fixed token, such as an Astronomer API token.
func Bearer(token string) Auth {
	return func(r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// TokenSource sends an OAuth2 access token from ts, such as Google's for
// Cloud Composer. ts is expected to cache and refresh tokens itself, as the
// sources from golang.org/x/oauth2/google do.
func TokenSource(ts oauth2.TokenSource) Auth {
	return func(r *http.Request) error {
		t, err := ts.Token()
		if err != nil {
			return fmt.Errorf("get access token: %w", err)
		}
		t.SetAuthHeader(r)
		return nil
	}
}

// Client reads the Airflow REST API of one webserver. It satisfies
// airflow.Getter.
type Client struct {
	api  string
	auth Auth
}

// NewClient returns a Client for the webserver at baseURL, the address of
// the Airflow UI without /api/v1. auth may be nil.
func NewClient(baseURL string, auth Auth) *Client {
	return &Client{api: strings.TrimRight(baseURL, "/") + "/api/v1", auth: auth}
}

// Get fetches path (relative to /api/v1) and returns the JSON response body.
// The method is fixed to GET here and nowhere else.
//
// Task logs are asked for as plain text, because as JSON Airflow 2 renders
// them as a Python tuple list. A plain text answer comes back as a JSON
// string, the shape MWAA uses, so callers always get JSON.
func (c *Client) Get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.api + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain")
	if c.auth != nil {
		if err := c.auth(req); err != nil {
			return nil, err
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &airflow.APIError{Status: resp.StatusCode, Body: string(body)}
	}
	switch mediaType(resp) {
	case "text/plain":
		return json.Marshal(string(body))
	case "text/html":
		// The login page of the UI: the URL is not the webserver root, or a
		// proxy in front of it did not accept the credentials.
		return nil, fmt.Errorf("%s answered with an HTML page instead of the REST API; check the URL and the credentials", req.URL.Host)
	}
	return body, nil
}

func mediaType(resp *http.Response) string {
	t, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return t
}
