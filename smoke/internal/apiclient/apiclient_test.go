package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadSendsOneFilePartWithTheToken(t *testing.T) {
	var field, filename, content, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		f, h, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		field, filename, content = "file", h.Filename, string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp := New(srv.URL, "t0ken", srv.Client()).Upload(context.Background(), "/import", "file", "dump.mok", []byte("SQLite format 3\x00"))

	if resp.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", resp.Code, resp.Body)
	}
	if field != "file" || filename != "dump.mok" || content != "SQLite format 3\x00" || auth != "Bearer t0ken" {
		t.Errorf("got field %q, filename %q, content %q, auth %q", field, filename, content, auth)
	}
}

func TestWithHeaderSendsItAndLeavesTheOriginalClientAlone(t *testing.T) {
	var origins []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins = append(origins, r.Header.Get("Origin"))
		w.Header().Set("X-Seen", "yes")
	}))
	defer srv.Close()
	base := New(srv.URL, "", srv.Client())

	resp := base.WithHeader("Origin", "https://evil.example").Do(context.Background(), http.MethodGet, "/", nil)
	base.Do(context.Background(), http.MethodGet, "/", nil)

	if len(origins) != 2 || origins[0] != "https://evil.example" || origins[1] != "" {
		t.Errorf("Origins sent = %q, want the header on the copy only", origins)
	}
	if resp.Header.Get("X-Seen") != "yes" {
		t.Errorf("response headers = %v, want the server's", resp.Header)
	}
}

func TestNoRedirectsAnswersTheRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	client := New(srv.URL, "", srv.Client())

	if got := client.NoRedirects().Do(context.Background(), http.MethodGet, "/login", nil); got.Code != http.StatusFound ||
		got.Header.Get("Location") != "/elsewhere" {
		t.Errorf("NoRedirects() = %d, Location %q; want the 302 itself", got.Code, got.Header.Get("Location"))
	}
	if got := client.Do(context.Background(), http.MethodGet, "/login", nil); got.Code != http.StatusTeapot {
		t.Errorf("the original client = %d, want it still following redirects", got.Code)
	}
}
