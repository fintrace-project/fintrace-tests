package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type Response struct {
	Code int
	Body []byte
}

// Client calls one application. A transport failure is answered as code 0 with the error as the
// body, so a check fails with the reason instead of the run aborting.
type Client struct {
	base  string
	token string
	hc    *http.Client
}

func New(base, token string, hc *http.Client) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, hc: hc}
}

func (c *Client) Anonymous() *Client {
	return &Client{base: c.base, hc: c.hc}
}

func (c *Client) WithToken(token string) *Client {
	return &Client{base: c.base, token: token, hc: c.hc}
}

func (c *Client) Do(ctx context.Context, method, path string, body any) Response {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return Response{Body: []byte(err.Error())}
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return Response{Body: []byte(err.Error())}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return Response{Body: []byte(err.Error())}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{Code: resp.StatusCode, Body: []byte(err.Error())}
	}
	return Response{Code: resp.StatusCode, Body: b}
}
