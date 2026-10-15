package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type Token struct {
	Access            string
	Subject           string
	PreferredUsername string
	Roles             []string
}

func (t Token) HasRole(role string) bool {
	return slices.Contains(t.Roles, role)
}

// ClientCredentials obtains a token for a confidential client's service account.
func ClientCredentials(ctx context.Context, hc *http.Client, issuer, clientID, secret string) (Token, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {secret},
	}
	endpoint := strings.TrimRight(issuer, "/") + "/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hc.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &e)
		return Token{}, fmt.Errorf("token endpoint answered %d %s %s", resp.StatusCode, e.Error, e.Description)
	}

	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return Token{}, fmt.Errorf("token response carries no access_token")
	}
	return parse(tr.AccessToken)
}

// parse reads the claims without verifying the signature: Core verifies it, this only reports.
func parse(access string) (Token, error) {
	parts := strings.Split(access, ".")
	if len(parts) != 3 {
		return Token{}, fmt.Errorf("access token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Token{}, fmt.Errorf("decode token claims: %w", err)
	}
	var claims struct {
		Sub               string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		RealmAccess       struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Token{}, fmt.Errorf("decode token claims: %w", err)
	}
	return Token{
		Access:            access,
		Subject:           claims.Sub,
		PreferredUsername: claims.PreferredUsername,
		Roles:             claims.RealmAccess.Roles,
	}, nil
}
