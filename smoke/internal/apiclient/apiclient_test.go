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
