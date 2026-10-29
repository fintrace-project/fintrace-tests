// Package bff is fintrace-bff's smoke suite: the edge's own answers, and S taken end to end through
// it — uploaded, every balance read back, and a first entry recorded — with fintrace-test's token on
// the Bearer path, the one way through the BFF without a browser.
package bff

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/fintrace-project/fintrace-tests/smoke/internal/apiclient"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/dump"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/suite"
)

const api = "/api/v1"

type workspace struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

type job struct {
	Status      string            `json:"status"`
	Accounts    int               `json:"accounts"`
	Categories  int               `json:"categories"`
	Operations  int               `json:"operations"`
	Transfers   int               `json:"transfers"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Problems    []json.RawMessage `json:"problems"`
}

type problem struct {
	Code string `json:"code"`
}

type profile struct {
	Username string `json:"username"`
}

type account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Archived bool   `json:"archived"`
}

type balance struct {
	AccountID string  `json:"accountId"`
	Currency  string  `json:"currency"`
	Balance   float64 `json:"balance"`
}

type run struct {
	suite.Target
	ctx     context.Context
	last    apiclient.Response
	created []string
	deleted map[string]bool
}

// Run checks the edge and takes S through it. Every workspace it creates is deleted before it
// returns, whether or not the checks passed.
func Run(ctx context.Context, t suite.Target) {
	r := &run{Target: t, ctx: ctx, deleted: map[string]bool{}}
	defer r.cleanUp()

	r.edge()
	r.authentication()
	r.routing()
	r.coreThroughTheEdge()
	r.csrf()
	s, ok := r.loadDump(filepath.Join(r.Dumps, "mok", "S"))
	if ok {
		if ws := r.importS(s); ws != "" {
			accounts := r.balances(ws, s.Expected)
			r.firstEntry(ws, accounts)
			r.reimport(ws, s)
			r.behindTheAPI(ws)
		}
	}

	r.Report.Section("9. Clean up")
	for _, ws := range r.created {
		r.code(r.deleteWorkspace(ws), 204, "workspace "+ws+" deleted through the BFF")
	}
}

func (r *run) edge() {
	r.Report.Section("0. The edge")
	r.code(r.Client.Anonymous().Do(r.ctx, http.MethodGet, "/healthz", nil), 200, "/healthz answers without a token")
	r.check("/healthz says UP", strings.TrimSpace(string(r.last.Body)), `{"status":"UP"}`)
	r.securityHeaders("/healthz")
}

func (r *run) authentication() {
	r.Report.Section("1. Authentication")
	r.problem(r.Client.Anonymous().Do(r.ctx, http.MethodGet, api+"/workspaces", nil), 401, "NO_SESSION",
		"no credentials: neither a session nor a token")
	r.problem(r.Client.WithToken("not.a.token").Do(r.ctx, http.MethodGet, api+"/workspaces", nil), 401, "INVALID_TOKEN",
		"a forged token")
	r.check("a forged token: WWW-Authenticate", r.last.Header.Get("WWW-Authenticate"), `Bearer error="invalid_token"`)
	r.code(r.call(http.MethodGet, "/workspaces", nil), 200, "fintrace-test's token")
}

func (r *run) routing() {
	r.Report.Section("2. Routing")
	r.problem(r.call(http.MethodGet, "/unknown", nil), 404, "NOT_FOUND", "an unlisted API route is the BFF's 404")
	// Same-origin, or the CSRF check — which exempts a token only under /api/ — refuses it first.
	r.code(r.Client.WithHeader("Origin", r.Client.Base()).
		Do(r.ctx, http.MethodPost, "/admin"+api+"/workspaces/"+randomID()+"/replay", nil), 405,
		"Core's admin replay is not reachable: it falls to the web UI, GET only")
	r.problem(r.Client.Anonymous().Do(r.ctx, http.MethodGet, "/auth/logout", nil), 404, "NOT_FOUND",
		"GET /auth/logout: a link logs no one out")

	login := r.Client.Anonymous().NoRedirects().Do(r.ctx, http.MethodGet, "/auth/login", nil)
	r.code(login, 302, "/auth/login redirects")
	location := login.Header.Get("Location")
	r.check("to the realm the BFF discovered", strings.HasPrefix(location, strings.TrimRight(r.Issuer, "/")+"/"), true)
	r.check("with PKCE", strings.Contains(location, "code_challenge_method=S256"), true)

	r.code(r.Client.Anonymous().Do(r.ctx, http.MethodGet, "/", nil), 200, "/ is the web UI")
	r.check("as HTML", strings.HasPrefix(r.last.Header.Get("Content-Type"), "text/html"), true)
	r.securityHeaders("/")
}

func (r *run) coreThroughTheEdge() {
	r.Report.Section("3. Core through the edge")
	r.code(r.call(http.MethodPost, "/workspaces", map[string]any{"workspaceName": "smoke-bff", "defaultCurrency": "EUR"}),
		201, "a workspace created with no Origin, on the token")
	w := decode[workspace](r)
	r.track(w.ID)
	r.check("its Location names the BFF, not Core's host", r.last.Header.Get("Location"),
		r.Client.Base()+api+"/workspaces/"+w.ID)
	r.securityHeaders("Core's 201")
	r.code(r.call(http.MethodGet, "/workspaces/"+w.ID, nil), 200, "read back")
	r.check("the same workspace", decode[workspace](r).ID, w.ID)

	r.code(r.call(http.MethodGet, "/users/current", nil), 200, "the caller's profile, through the edge")
	r.check("names the token's user", decode[profile](r).Username, r.Token.PreferredUsername)
	r.problem(r.call(http.MethodGet, "/users/current/id", nil), 404, "NOT_FOUND",
		"the caller's id is not listed: only the BFF asks Core for it")
}

func (r *run) csrf() {
	r.Report.Section("4. CSRF")
	resp := r.Client.Anonymous().WithHeader("Origin", "https://evil.example").
		Do(r.ctx, http.MethodPost, api+"/workspaces", map[string]any{"workspaceName": "smoke-csrf", "defaultCurrency": "EUR"})
	r.code(resp, 403, "a cross-origin POST on the session path is refused before Core")
}

func (r *run) loadDump(dir string) (dump.Dump, bool) {
	r.Report.Section("5. S end to end: uploaded through the BFF")
	s, err := dump.Load(dir)
	if err != nil {
		r.Report.Equal("S loads from "+dir, err, "loaded", nil)
		return dump.Dump{}, false
	}
	r.Report.Equal("S loads from "+dir, "loaded", "loaded", nil)
	r.Report.Info("seed %d, %d rows, %d bytes as SQLite", s.Expected.Seed, s.Expected.Rows, len(s.SQLite))
	return s, true
}

func (r *run) importS(s dump.Dump) string {
	r.code(r.call(http.MethodPost, "/workspaces", map[string]any{"workspaceName": "smoke-bff-import", "defaultCurrency": "EUR"}),
		201, "a NEW workspace created")
	w := decode[workspace](r)
	r.track(w.ID)
	r.check("starts NEW", w.Status, "NEW")

	r.code(r.upload(w.ID, s.SQLite), 200, "S uploaded through the BFF to the importer")
	j := decode[job](r)
	want := s.Expected
	r.check("the job SUCCEEDED", j.Status, "SUCCEEDED")
	r.check("accounts", j.Accounts, len(want.Accounts))
	r.check("categories", j.Categories, want.Categories)
	r.check("operations", j.Operations, want.Operations)
	r.check("transfers", j.Transfers, want.Transfers)
	r.check("diagnostics", len(j.Diagnostics), want.Diagnostics)
	r.check("problems", len(j.Problems), 0)
	r.check("the import activated the workspace", r.statusOf(w.ID), "ACTIVE")
	return w.ID
}

func (r *run) balances(ws string, want dump.Expected) []account {
	r.Report.Section("6. S end to end: every balance read back through the BFF")
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/accounts?includeArchived=true", nil), 200, "accounts listed")
	accounts := decode[[]account](r)
	r.check("as many accounts as S expects", len(accounts), len(want.Accounts))
	balances := r.readBalances(ws)

	// Matched by name: S reuses a source uid, and Core's ids are the importer's own.
	for _, w := range want.Accounts {
		a, ok := byName(accounts, w.Name)
		if !ok {
			r.check(w.Name, "missing", "present")
			continue
		}
		b := balanceOf(balances, a.ID)
		r.check(w.Name+": archived", a.Archived, w.Archived)
		r.check(w.Name+": currency", b.Currency, w.Currency)
		r.check(w.Name+": balance", cents(b.Balance), cents(w.Balance))
	}
	return accounts
}

// The user's first entry after the import: an expense moves its account's balance by exactly its
// amount, and no other.
func (r *run) firstEntry(ws string, accounts []account) {
	r.Report.Section("7. S end to end: a first entry after the import")
	var target account
	for _, a := range accounts {
		if !a.Archived {
			target = a
			break
		}
	}
	if target.ID == "" {
		r.check("an open account to record on", "none", "one")
		return
	}
	before := r.readBalances(ws)
	const amount = 12.50
	// Not before the import's final anchors, which carry today's balances.
	occurredAt := time.Now().In(r.Zone).Format("2006-01-02T15:04:05")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/operations", map[string]any{
		"amount": amount, "occurredAt": occurredAt, "kind": "EXPENSE",
		"accountId": target.ID, "categoryId": nil, "comment": "smoke: first entry",
	}), 201, fmt.Sprintf("a %.2f expense on %s", amount, target.Name))
	after := r.readBalances(ws)

	for _, b := range before {
		want := cents(b.Balance)
		label := "unchanged: " + nameOf(accounts, b.AccountID)
		if b.AccountID == target.ID {
			want = cents(b.Balance - amount)
			label = fmt.Sprintf("%s: down by %.2f", target.Name, amount)
		}
		r.check(label, cents(balanceOf(after, b.AccountID).Balance), want)
	}
}

func (r *run) reimport(ws string, s dump.Dump) {
	r.Report.Section("8. S end to end: the importer's refusal passes through unchanged")
	r.problem(r.upload(ws, s.SQLite), 409, "WORKSPACE_NOT_IMPORTABLE", "importing again into the now ACTIVE workspace")
}

func (r *run) behindTheAPI(ws string) {
	r.Report.Section("8b. Behind the API")
	r.dbCheck("one SUCCEEDED job, started by the token's user through the BFF", int64(1),
		`SELECT count(*) FROM t_import_jobs j JOIN t_users u ON u.id = j.started_by
		 WHERE j.workspace_id = $1 AND j.status = 'SUCCEEDED' AND u.external_id = $2`, ws, r.Token.Subject)
}

// securityHeaders checks the edge's headers on the last answer, each exactly once.
func (r *run) securityHeaders(label string) {
	h := r.last.Header
	once := func(name, want string) {
		got := h.Values(name)
		if want == "" {
			r.check(fmt.Sprintf("%s: %s, once", label, name), len(got) == 1 && got[0] != "", true)
			return
		}
		r.check(fmt.Sprintf("%s: %s", label, name), strings.Join(got, ", "), want)
	}
	once("X-Content-Type-Options", "nosniff")
	once("Referrer-Policy", "no-referrer")
	once("Content-Security-Policy", "")
	https := strings.HasPrefix(r.Client.Base(), "https://")
	r.check(fmt.Sprintf("%s: HSTS exactly when the origin is https (%t)", label, https),
		len(h.Values("Strict-Transport-Security")) == 1, https)
}

func (r *run) readBalances(ws string) []balance {
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/statistics/balances/accounts?includeArchived=true", nil), 200,
		"balances read")
	return decode[[]balance](r)
}

func (r *run) upload(ws string, content []byte) apiclient.Response {
	r.last = r.Client.Upload(r.ctx, api+"/workspaces/"+ws+"/import", "file", "dump.mok", content)
	return r.last
}

func (r *run) call(method, path string, body any) apiclient.Response {
	r.last = r.Client.Do(r.ctx, method, api+path, body)
	return r.last
}

func (r *run) problem(resp apiclient.Response, status int, code, label string) {
	r.code(resp, status, label)
	r.check(label+": code", decode[problem](r).Code, code)
}

func (r *run) check(label string, got, want any) {
	r.Report.Equal(label, got, want, r.last.Body)
}

func (r *run) code(resp apiclient.Response, want int, label string) {
	r.last = resp
	r.check(label, resp.Code, want)
}

func (r *run) dbCheck(label string, want any, query string, args ...any) {
	if r.DB == nil {
		r.Report.Skip(label, "no --db")
		return
	}
	got, err := r.DB.Value(r.ctx, query, args...)
	if err != nil {
		got = err
	}
	r.Report.Equal(label, got, want, nil)
}

// decode leaves the zero value on a body that does not parse; the checks that read it then fail
// with the body shown.
func decode[T any](r *run) T {
	var v T
	if err := json.Unmarshal(r.last.Body, &v); err != nil {
		r.Report.Info("could not read the %d response: %v", r.last.Code, err)
	}
	return v
}

func (r *run) statusOf(ws string) string {
	r.call(http.MethodGet, "/workspaces/"+ws, nil)
	return decode[workspace](r).Status
}

func (r *run) track(ws string) {
	if ws != "" {
		r.created = append(r.created, ws)
	}
}

func (r *run) deleteWorkspace(ws string) apiclient.Response {
	r.call(http.MethodGet, "/workspaces/"+ws, nil)
	version := decode[workspace](r).Version
	resp := r.call(http.MethodDelete, fmt.Sprintf("/workspaces/%s?version=%d", ws, version), nil)
	r.deleted[ws] = resp.Code == http.StatusNoContent
	return resp
}

func (r *run) cleanUp() {
	// An interrupted or failed run still deletes what it created.
	r.ctx = context.WithoutCancel(r.ctx)
	for _, ws := range r.created {
		if r.deleted[ws] {
			continue
		}
		if resp := r.deleteWorkspace(ws); resp.Code != http.StatusNoContent {
			r.Report.Info("could not delete workspace %s: %d", ws, resp.Code)
		}
	}
}

func cents(v float64) float64 {
	return math.Round(v*100) / 100
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func byName(accounts []account, name string) (account, bool) {
	for _, a := range accounts {
		if a.Name == name {
			return a, true
		}
	}
	return account{}, false
}

func nameOf(accounts []account, id string) string {
	for _, a := range accounts {
		if a.ID == id {
			return a.Name
		}
	}
	return id
}

func balanceOf(bs []balance, id string) balance {
	for _, b := range bs {
		if b.AccountID == id {
			return b
		}
	}
	return balance{Currency: "missing"}
}
