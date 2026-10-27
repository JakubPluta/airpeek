// Package mwaa reaches the Airflow REST API of an Amazon MWAA environment
// through the AWS InvokeRestApi operation. It can only issue GET requests.
package mwaa

import (
	"context"
	"errors"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	mwaasdk "github.com/aws/aws-sdk-go-v2/service/mwaa"
	"github.com/aws/aws-sdk-go-v2/service/mwaa/document"
	"github.com/aws/aws-sdk-go-v2/service/mwaa/types"

	"github.com/JakubPluta/airpeek/internal/airflow"
)

// Client reads the Airflow REST API of one MWAA environment. It satisfies
// airflow.Getter. InvokeRestApi goes through the AWS API, so it works with a
// private webserver and needs neither a VPN nor a web login token.
type Client struct {
	api *mwaasdk.Client
	env string
}

// NewClient returns a Client for the MWAA environment named env.
func NewClient(cfg aws.Config, env string) *Client {
	return &Client{api: mwaasdk.NewFromConfig(cfg), env: env}
}

// Get fetches path (relative to /api/v1) and returns the JSON response body.
// The method is fixed to GET here and nowhere else.
func (c *Client) Get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	params := make(map[string]string, len(query))
	for k := range query {
		params[k] = query.Get(k)
	}
	out, err := c.api.InvokeRestApi(ctx, &mwaasdk.InvokeRestApiInput{
		Name:            aws.String(c.env),
		Path:            aws.String(path),
		Method:          types.RestApiMethodGet,
		QueryParameters: document.NewLazyDocument(params),
	})
	if err != nil {
		return nil, apiError(err)
	}
	if out.RestApiResponse == nil {
		return []byte("null"), nil
	}
	return out.RestApiResponse.MarshalSmithyDocument()
}

// apiError unwraps the Airflow status and body that MWAA attaches to its
// client and server exceptions, so the UI can show what Airflow said.
func apiError(err error) error {
	var status *int32
	var body document.Interface
	var clientErr *types.RestApiClientException
	var serverErr *types.RestApiServerException
	switch {
	case errors.As(err, &clientErr):
		status, body = clientErr.RestApiStatusCode, clientErr.RestApiResponse
	case errors.As(err, &serverErr):
		status, body = serverErr.RestApiStatusCode, serverErr.RestApiResponse
	default:
		return err
	}
	text := ""
	if body != nil {
		if b, mErr := body.MarshalSmithyDocument(); mErr == nil {
			text = string(b)
		}
	}
	return &airflow.APIError{Status: int(aws.ToInt32(status)), Body: text}
}

// ListEnvironments returns the names of MWAA environments visible to cfg.
func ListEnvironments(ctx context.Context, cfg aws.Config) ([]string, error) {
	p := mwaasdk.NewListEnvironmentsPaginator(mwaasdk.NewFromConfig(cfg), &mwaasdk.ListEnvironmentsInput{})
	var names []string
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		names = append(names, page.Environments...)
	}
	return names, nil
}
