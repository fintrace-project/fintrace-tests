package calllog

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const userAgent = "fintrace-smoke"

var (
	formSecret = regexp.MustCompile(`((?:client_secret|subject_token|access_token|refresh_token)=)[^&]*`)
	jsonSecret = regexp.MustCompile(`("(?:access_token|refresh_token|id_token)"\s*:\s*")[^"]*`)
)

// Transport logs every outbound call: the call at INFO, headers and bodies at DEBUG, secrets masked.
type Transport struct {
	Next http.RoundTripper
}

func NewClient() *http.Client {
	return &http.Client{Transport: Transport{Next: http.DefaultTransport}, Timeout: 30 * time.Second}
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Keycloak sits behind Cloudflare, which blocks Go's default User-Agent.
	req.Header.Set("User-Agent", userAgent)
	debug := slog.Default().Enabled(req.Context(), slog.LevelDebug)

	var reqBody []byte
	if debug && req.Body != nil {
		reqBody, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	start := time.Now()
	resp, err := t.Next.RoundTrip(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		slog.Info("call failed", "method", req.Method, "url", req.URL.String(), "elapsed", elapsed, "error", err)
		return nil, err
	}
	slog.Info("call", "method", req.Method, "url", req.URL.String(), "status", resp.StatusCode, "elapsed", elapsed)

	if debug {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		slog.Debug("call detail",
			"method", req.Method, "url", req.URL.String(),
			"requestHeaders", maskHeaders(req.Header), "requestBody", requestBody(req.Header, reqBody),
			"responseHeaders", maskHeaders(resp.Header), "responseBody", mask(respBody))
	}
	return resp, nil
}

func maskHeaders(h http.Header) http.Header {
	out := h.Clone()
	for _, k := range []string{"Authorization", "Cookie", "Set-Cookie"} {
		if out.Get(k) != "" {
			out.Set(k, "***")
		}
	}
	return out
}

// requestBody keeps an uploaded file out of the log: a dump is binary and says nothing a size does not.
func requestBody(h http.Header, body []byte) string {
	if strings.HasPrefix(h.Get("Content-Type"), "multipart/") {
		return fmt.Sprintf("<multipart, %d bytes>", len(body))
	}
	return mask(body)
}

func mask(body []byte) string {
	b := formSecret.ReplaceAll(body, []byte("${1}***"))
	return string(jsonSecret.ReplaceAll(b, []byte("${1}***")))
}
