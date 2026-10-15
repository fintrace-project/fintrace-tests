package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func jwt(claims string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(claims)) + ".sig"
}

func TestClientCredentialsReadsTheClaims(t *testing.T) {
	access := jwt(`{"sub":"u-1","preferred_username":"service-account-fintrace-test","realm_access":{"roles":["fintrace:user"]}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realms/x/protocol/openid-connect/token" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "fintrace-test" || r.Form.Get("client_secret") != "s" {
			t.Errorf("form = %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"access_token":"` + access + `"}`))
	}))
	defer srv.Close()

	tok, err := ClientCredentials(context.Background(), srv.Client(), srv.URL+"/realms/x/", "fintrace-test", "s")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != access || tok.Subject != "u-1" || tok.PreferredUsername != "service-account-fintrace-test" {
		t.Errorf("token = %+v", tok)
	}
	if !tok.HasRole("fintrace:user") || tok.HasRole("fintrace:admin") {
		t.Errorf("roles = %v", tok.Roles)
	}
}

func TestClientCredentialsReportsTheEndpointsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized_client","error_description":"Invalid client secret"}`))
	}))
	defer srv.Close()

	_, err := ClientCredentials(context.Background(), srv.Client(), srv.URL, "fintrace-test", "wrong")
	if err == nil || !strings.Contains(err.Error(), "401 unauthorized_client Invalid client secret") {
		t.Errorf("err = %v", err)
	}
}
