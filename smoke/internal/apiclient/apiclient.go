package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"strings"
)

type Response struct {
	Code   int
	Body   []byte
	Header http.Header
}

// Client calls one application. A transport failure is answered as code 0 with the error as the
// body, so a check fails with the reason instead of the run aborting.
type Client struct {
	base   string
	token  string
	hc     *http.Client
	header http.Header
}

func New(base, token string, hc *http.Client) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, hc: hc}
}

func (c *Client) Base() string {
	return c.base
}

func (c *Client) Anonymous() *Client {
	return &Client{base: c.base, hc: c.hc, header: c.header}
}

func (c *Client) WithToken(token string) *Client {
	return &Client{base: c.base, token: token, hc: c.hc, header: c.header}
}

// WithHeader sends one more header on every request, e.g. an Origin for a CSRF check.
func (c *Client) WithHeader(name, value string) *Client {
	h := c.header.Clone()
	if h == nil {
		h = http.Header{}
	}
	h.Set(name, value)
	return &Client{base: c.base, token: c.token, hc: c.hc, header: h}
}

// NoRedirects answers a redirect instead of following it, the call still logged.
func (c *Client) NoRedirects() *Client {
	hc := *c.hc
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: c.base, token: c.token, hc: &hc, header: c.header}
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
	return c.send(req)
}

// Upload posts one file as a multipart form part.
func (c *Client) Upload(ctx context.Context, path, field, filename string, content []byte) Response {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, filename)
	if err == nil {
		_, err = part.Write(content)
	}
	if err == nil {
		err = w.Close()
	}
	if err != nil {
		return Response{Body: []byte(err.Error())}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, &buf)
	if err != nil {
		return Response{Body: []byte(err.Error())}
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return c.send(req)
}

func (c *Client) send(req *http.Request) Response {
	maps.Copy(req.Header, c.header)
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
		return Response{Code: resp.StatusCode, Body: []byte(err.Error()), Header: resp.Header}
	}
	return Response{Code: resp.StatusCode, Body: b, Header: resp.Header}
}
