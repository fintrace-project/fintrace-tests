// Package mokimporter is fintrace-mok-importer's smoke suite: the synthetic S dump uploaded to the
// importer, and every account's balance read back from Core.
package mokimporter

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

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
	Anchors     int               `json:"anchors"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Problems    []json.RawMessage `json:"problems"`
}

type problem struct {
	Code string `json:"code"`
}

type account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

type balance struct {
	AccountID string  `json:"accountId"`
	Currency  string  `json:"currency"`
	Balance   float64 `json:"balance"`
}

type run struct {
	suite.Target
	ctx       context.Context
	last      apiclient.Response
	workspace string
	deleted   bool
}

// Run imports S into a fresh workspace and checks Core ends in the state S's expected.json describes.
// The workspace is deleted before it returns, whether or not the checks passed.
func Run(ctx context.Context, t suite.Target) {
	r := &run{Target: t, ctx: ctx}
	defer r.cleanUp()

	r.authentication()
	s, ok := r.loadDump(filepath.Join(r.Dumps, "mok", "S"))
	if !ok {
		return
	}
	ws := r.createWorkspace()
	if ws == "" {
		return
	}
	r.importDump(ws, s)
	r.balances(ws, s.Expected)
	r.refusals(ws, s)
	r.behindTheAPI(ws)

	r.Report.Section("7. Clean up")
	r.code(r.deleteWorkspace(), 204, "the workspace is deleted")
}

func (r *run) authentication() {
	r.Report.Section("0. Authentication")
	path := importPath(randomID())
	r.code(r.Client.Anonymous().Upload(r.ctx, path, "file", "dump.mok", []byte("x")), 401, "an upload without a token")
	r.code(r.Client.WithToken("not.a.token").Upload(r.ctx, path, "file", "dump.mok", []byte("x")), 401, "an upload with a forged token")
	r.code(r.Client.Anonymous().Do(r.ctx, http.MethodGet, "/actuator/health", nil), 200, "the health probe answers without a token")
}

func (r *run) loadDump(dir string) (dump.Dump, bool) {
	r.Report.Section("1. The S dump")
	s, err := dump.Load(dir)
	if err != nil {
		r.Report.Equal("S loads from "+dir, err, "loaded", nil)
		return dump.Dump{}, false
	}
	r.Report.Equal("S loads from "+dir, "loaded", "loaded", nil)
	r.Report.Info("seed %d, %d rows, %d bytes as SQLite", s.Expected.Seed, s.Expected.Rows, len(s.SQLite))
	return s, true
}

func (r *run) createWorkspace() string {
	r.Report.Section("2. A NEW workspace in Core")
	r.code(r.core(http.MethodPost, "/workspaces", map[string]any{"workspaceName": "smoke-import", "defaultCurrency": "EUR"}),
		201, "workspace created")
	w := decode[workspace](r)
	r.workspace = w.ID
	r.check("starts NEW", w.Status, "NEW")
	return w.ID
}

func (r *run) importDump(ws string, s dump.Dump) {
	r.Report.Section("3. Upload S to the importer")
	r.code(r.upload(ws, s.SQLite), 200, "S imported")
	j := decode[job](r)
	want := s.Expected
	r.check("the job SUCCEEDED", j.Status, "SUCCEEDED")
	r.check("accounts", j.Accounts, len(want.Accounts))
	r.check("categories", j.Categories, want.Categories)
	r.check("operations", j.Operations, want.Operations)
	r.check("transfers", j.Transfers, want.Transfers)
	r.check("diagnostics", len(j.Diagnostics), want.Diagnostics)
	r.check("problems", len(j.Problems), 0)
	r.Report.Info("anchors: %d", j.Anchors)

	r.Report.Section("4. Activation")
	r.check("the import activated the workspace", r.statusOf(ws), "ACTIVE")
}

func (r *run) balances(ws string, want dump.Expected) {
	r.Report.Section("5. Every account's balance equals S's expected one")
	r.code(r.core(http.MethodGet, "/workspaces/"+ws+"/accounts?includeArchived=true", nil), 200, "accounts listed")
	accounts := decode[[]account](r)
	r.check("as many accounts as S expects", len(accounts), len(want.Accounts))
	r.code(r.core(http.MethodGet, "/workspaces/"+ws+"/statistics/balances/accounts?includeArchived=true", nil), 200, "balances read")
	balances := decode[[]balance](r)

	// Matched by name: S reuses a source uid, and Core's ids are the importer's own.
	for _, w := range want.Accounts {
		a, ok := byName(accounts, w.Name)
		if !ok {
			r.check(w.Name, "missing", "present")
			continue
		}
		b := balanceOf(balances, a.ID)
		r.check(fmt.Sprintf("%s: archived", w.Name), a.Archived, w.Archived)
		r.check(fmt.Sprintf("%s: currency", w.Name), b.Currency, w.Currency)
		r.check(fmt.Sprintf("%s: balance", w.Name), b.Balance, w.Balance)
	}
}

func (r *run) refusals(ws string, s dump.Dump) {
	r.Report.Section("6. Refusals, each a ProblemDetail with its code")
	r.problem(r.upload(ws, s.SQLite), 409, "WORKSPACE_NOT_IMPORTABLE", "importing again into the now ACTIVE workspace")
	r.problem(r.upload(ws, []byte("not a dump at all")), 422, "INVALID_DUMP", "a file that is not a SQLite dump")
	r.problem(r.upload(randomID(), s.SQLite), 404, "WORKSPACE_NOT_FOUND", "a workspace that does not exist")
}

func (r *run) behindTheAPI(ws string) {
	r.Report.Section("6b. Behind the API")
	r.dbCheck("one SUCCEEDED job, started by the token's user", int64(1),
		`SELECT count(*) FROM t_import_jobs j JOIN t_users u ON u.id = j.started_by
		 WHERE j.workspace_id = $1 AND j.status = 'SUCCEEDED' AND u.external_id = $2`, ws, r.Token.Subject)
	r.dbCheck("every imported event is recorded by the token's user", true,
		`SELECT bool_and(e.recorded_by = u.id) FROM t_events e, t_users u
		 WHERE e.workspace_id = $1 AND u.external_id = $2`, ws, r.Token.Subject)
	if r.DB != nil {
		if n, err := r.DB.Value(r.ctx, "SELECT count(*) FROM t_events WHERE workspace_id = $1", ws); err == nil {
			r.Report.Info("events in the workspace: %v", n)
		}
	}
}

func (r *run) problem(resp apiclient.Response, status int, code, label string) {
	r.code(resp, status, label)
	r.check(label+": code", decode[problem](r).Code, code)
}

func (r *run) upload(ws string, content []byte) apiclient.Response {
	r.last = r.Client.Upload(r.ctx, importPath(ws), "file", "dump.mok", content)
	return r.last
}

func (r *run) core(method, path string, body any) apiclient.Response {
	r.last = r.Core.Do(r.ctx, method, api+path, body)
	return r.last
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
	r.core(http.MethodGet, "/workspaces/"+ws, nil)
	return decode[workspace](r).Status
}

func (r *run) deleteWorkspace() apiclient.Response {
	r.core(http.MethodGet, "/workspaces/"+r.workspace, nil)
	version := decode[workspace](r).Version
	resp := r.core(http.MethodDelete, fmt.Sprintf("/workspaces/%s?version=%d", r.workspace, version), nil)
	r.deleted = resp.Code == http.StatusNoContent
	return resp
}

func (r *run) cleanUp() {
	if r.workspace == "" || r.deleted {
		return
	}
	// An interrupted or failed run still deletes what it created.
	r.ctx = context.WithoutCancel(r.ctx)
	if resp := r.deleteWorkspace(); resp.Code != http.StatusNoContent {
		r.Report.Info("could not delete workspace %s: %d", r.workspace, resp.Code)
	}
}

func importPath(ws string) string {
	return api + "/workspaces/" + ws + "/import"
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

func balanceOf(bs []balance, id string) balance {
	for _, b := range bs {
		if b.AccountID == id {
			return b
		}
	}
	return balance{Currency: "missing"}
}
