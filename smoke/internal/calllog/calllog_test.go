package calllog

import (
	"net/http"
	"strings"
	"testing"
)

func TestMaskHidesSecretsInFormAndJSONBodies(t *testing.T) {
	cases := map[string]string{
		"grant_type=client_credentials&client_id=fintrace-test&client_secret=s3cret": "grant_type=client_credentials&client_id=fintrace-test&client_secret=***",
		"subject_token=abc.def.ghi&audience=fintrace-core":                           "subject_token=***&audience=fintrace-core",
		`{"access_token":"abc.def.ghi","expires_in":300,"refresh_token": "r"}`:       `{"access_token":"***","expires_in":300,"refresh_token": "***"}`,
	}
	for in, want := range cases {
		if got := mask([]byte(in)); got != want {
			t.Errorf("mask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestBodyLogsAnUploadBySizeOnly(t *testing.T) {
	h := http.Header{"Content-Type": {"multipart/form-data; boundary=x"}}
	if got := requestBody(h, []byte("SQLite format 3\x00binary")); got != "<multipart, 22 bytes>" {
		t.Errorf("requestBody = %q", got)
	}
	form := http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}
	if got := requestBody(form, []byte("client_secret=s3cret")); got != "client_secret=***" {
		t.Errorf("a form body is still masked, not hidden: %q", got)
	}
}

func TestMaskHeadersHidesAuthorizationWithoutTouchingTheRequest(t *testing.T) {
	h := http.Header{"Authorization": {"Bearer abc"}, "Accept": {"application/json"}}
	got := maskHeaders(h)
	if got.Get("Authorization") != "***" || got.Get("Accept") != "application/json" {
		t.Errorf("maskHeaders = %v", got)
	}
	if !strings.HasPrefix(h.Get("Authorization"), "Bearer ") {
		t.Errorf("the original header was modified: %v", h)
	}
}
