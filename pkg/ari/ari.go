package ari

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

var ErrNotFound = errors.New("resource not found")

type Client struct {
	Scheme      string
	Host        string
	App         string
	Credentials string
	client      *http.Client
}

func New(scheme, host, app, credentials string) *Client {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	return &Client{
		Scheme:      scheme,
		Host:        host,
		App:         app,
		Credentials: credentials,
		client:      client,
	}
}

func (c *Client) Post(path, contentType string, params *url.Values, body io.Reader) (*http.Response, error) {
	if params == nil {
		params = &url.Values{}
	}
	params.Set("api_key", c.Credentials)
	target := url.URL{
		Scheme:   "http",
		Host:     c.Host,
		Path:     path,
		RawQuery: params.Encode(),
	}

	return c.client.Post(target.String(), contentType, body)
}

func (c *Client) Do(method, path, contentType string, params *url.Values, body io.Reader) (*http.Response, error) {
	if params == nil {
		params = &url.Values{}
	}
	params.Set("api_key", c.Credentials)
	target := url.URL{
		Scheme:   "http",
		Host:     c.Host,
		Path:     path,
		RawQuery: params.Encode(),
	}

	req, err := http.NewRequest(method, target.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	return c.client.Do(req)
}
