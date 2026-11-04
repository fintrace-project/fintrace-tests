package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/fintrace-project/fintrace-tests/smoke/internal/apiclient"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/suite"
)

const api = "/api/v1"

type workspace struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

type entity struct {
	ID string `json:"id"`
}

type userID struct {
	UserID string `json:"userId"`
}

type profile struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type category struct {
	ID         string  `json:"id"`
	SystemCode *string `json:"systemCode"`
}

type transfer struct {
	ID     string `json:"id"`
	Source struct {
		OperationID string `json:"operationId"`
	} `json:"source"`
}

type anchor struct {
	Difference *float64 `json:"difference"`
}

type balance struct {
	AccountID string  `json:"accountId"`
	Balance   float64 `json:"balance"`
}

type wholeList[T any] struct {
	Items []T             `json:"items"`
	Page  json.RawMessage `json:"page"`
}

type feedItem struct {
	Type   string `json:"type"`
	Cursor string `json:"cursor"`
	Side   string `json:"side"`
}

type feedPage struct {
	Items []feedItem `json:"items"`
	Page  struct {
		Older *string `json:"older"`
		Newer *string `json:"newer"`
	} `json:"page"`
}

type problem struct {
	Code    string `json:"code"`
	TraceID string `json:"traceId"`
	Errors  []struct {
		Field string `json:"field"`
	} `json:"errors"`
}

type run struct {
	suite.Target
	ctx     context.Context
	last    apiclient.Response
	created []string
	deleted map[string]bool
}

// Run walks the data path (sections 1-10) and a workspace's whole lifecycle (section 11). Every
// workspace it creates is deleted before it returns, whether or not the checks passed.
func Run(ctx context.Context, t suite.Target) {
	r := &run{Target: t, ctx: ctx, deleted: map[string]bool{}}
	defer r.cleanUp()

	r.authentication()
	ws := r.createWorkspace()
	cash, card := r.accounts(ws)
	food := r.category(ws)
	r.operation(ws, cash, food)
	r.transfer(ws, cash, card, food)
	r.anchor(ws, card)
	r.balances(ws, cash, card)
	r.replay(ws, cash)
	r.recorder(ws)
	r.feeds(ws, cash, card)
	r.lifecycle()

	r.Report.Section("12. Clean up")
	r.code(r.deleteWorkspace(ws, r.versionOf(ws)), 204, "the data workspace is deleted too")
}

func (r *run) authentication() {
	r.Report.Section("0. Authentication and the caller")
	r.code(r.Client.Anonymous().Do(r.ctx, http.MethodGet, api+"/workspaces", nil), 401, "no token")
	r.code(r.Client.WithToken("not.a.token").Do(r.ctx, http.MethodGet, api+"/workspaces", nil), 401, "a forged token")
	r.dbCheck("the V0004 stub user is gone (V0012)", int64(0),
		"SELECT count(*) FROM t_users WHERE external_id = 'stub:testuser'")

	r.code(r.call(http.MethodGet, "/users/current/id", nil), 200, "the caller's id resolved")
	id := decode[userID](r).UserID
	r.dbCheck("it is the id of the token's t_users row", id,
		"SELECT id::text FROM t_users WHERE external_id = $1", r.Token.Subject)
	r.code(r.call(http.MethodGet, "/users/current", nil), 200, "the caller's profile read")
	p := decode[profile](r)
	r.check("the profile names the same user", p.ID, id)
	r.check("its username is the token's preferred_username", p.Username, r.Token.PreferredUsername)
}

func (r *run) createWorkspace() string {
	r.Report.Section("1. Create workspace")
	r.code(r.call(http.MethodPost, "/workspaces", map[string]any{"workspaceName": "smoke", "defaultCurrency": "EUR"}),
		201, "workspace created")
	w := decode[workspace](r)
	r.track(w.ID)
	r.check("starts NEW", w.Status, "NEW")
	r.dbCheck("caller's user created on first request, named from preferred_username", r.Token.PreferredUsername,
		"SELECT username FROM t_users WHERE external_id = $1", r.Token.Subject)

	// ZZZ passes the request's pattern, so only Core's validation service refuses it.
	r.code(r.call(http.MethodPost, "/workspaces", map[string]any{"workspaceName": "smoke-refused", "defaultCurrency": "ZZZ"}),
		400, "an unknown currency is refused")
	r.problem("VALIDATION_FAILED")
	p := decode[problem](r)
	r.check("the refusal names the field", len(p.Errors) > 0 && p.Errors[0].Field == "defaultCurrency", true)
	return w.ID
}

func (r *run) accounts(ws string) (cash, card string) {
	r.Report.Section("2. Accounts: cash with an initial balance of 100, card without")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/accounts", account("cash", 100.00)), 201, "cash created")
	cash = decode[entity](r).ID
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/accounts", account("card", nil)), 201, "card created")
	card = decode[entity](r).ID
	r.check("first command activated the workspace", r.statusOf(ws), "ACTIVE")
	// The operation and transfer must come after cash's initial-balance anchor.
	r.pause()
	return cash, card
}

func (r *run) category(ws string) string {
	r.Report.Section("3. Category under the expense root")
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/categories?includeArchived=false", nil), 200, "categories listed")
	var root string
	for _, c := range decode[[]category](r) {
		if c.SystemCode != nil && *c.SystemCode == "EXPENSE_ROOT" {
			root = c.ID
		}
	}
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/categories",
		map[string]any{"name": "food", "parentId": root, "icon": nil}), 201, "category created")
	return decode[entity](r).ID
}

func (r *run) operation(ws, cash, food string) {
	r.Report.Section("4. Operation: 12.50 expense from cash")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/operations", map[string]any{
		"amount": 12.50, "occurredAt": r.now(), "kind": "EXPENSE",
		"accountId": cash, "categoryId": food, "comment": "lunch",
	}), 201, "operation created")
	op := decode[entity](r).ID
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/operations/"+op, nil), 200, "operation read back")
}

func (r *run) transfer(ws, cash, card, food string) {
	r.Report.Section("5. Transfer: 30 from cash to card")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/transfers", map[string]any{
		"occurredAt": r.now(),
		"source":     map[string]any{"accountId": cash, "amount": 30},
		"target":     map[string]any{"accountId": card, "amount": 30},
		"comment":    "to card",
	}), 201, "transfer created")
	tr := decode[transfer](r).ID
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/transfers/"+tr, nil), 200, "transfer read back")
	leg := decode[transfer](r).Source.OperationID
	r.code(r.call(http.MethodPut, "/workspaces/"+ws+"/operations/"+leg, map[string]any{
		"amount": 1, "occurredAt": r.now(), "kind": "EXPENSE",
		"accountId": cash, "categoryId": food, "comment": nil,
	}), 409, "a transfer leg refuses PUT through /operations")
	r.problem("TRANSFER_VIA_OPERATIONS")
	r.pause()
}

func (r *run) anchor(ws, card string) {
	r.Report.Section("6. Balance anchor on card: 30")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/accounts/"+card+"/balance-anchors", map[string]any{"value": 30}),
		201, "anchor created")
	r.check("card's first anchor answers a null difference", decode[anchor](r).Difference == nil, true)
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/accounts/"+card+"/balance-anchors", nil), 200, "card's anchors listed")
	anchors := decode[wholeList[anchor]](r)
	r.check("the list holds the one anchor", len(anchors.Items), 1)
	r.check("the list answers whole, with no page", anchors.Page == nil, true)
}

func (r *run) balances(ws, cash, card string) {
	r.Report.Section("7. Balances")
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/statistics/balances/accounts", nil), 200, "balances read")
	b := decode[[]balance](r)
	r.check("cash = 100 - 12.50 - 30", balanceOf(b, cash), 57.5)
	r.check("card = 30", balanceOf(b, card), 30.0)
}

func (r *run) replay(ws, cash string) {
	r.Report.Section("8. Replay")
	admin := r.Token.HasRole("fintrace:admin")
	want := 403
	if admin {
		want = 200
	}
	r.code(r.Client.Do(r.ctx, http.MethodPost, "/admin"+api+"/workspaces/"+ws+"/replay", nil), want,
		fmt.Sprintf("replay (token has fintrace:admin: %t)", admin))
	r.call(http.MethodGet, "/workspaces/"+ws+"/statistics/balances/accounts", nil)
	r.check("cash unchanged after the replay attempt", balanceOf(decode[[]balance](r), cash), 57.5)
}

func (r *run) recorder(ws string) {
	r.Report.Section("9. Events record the caller")
	r.dbCheck("one recorder across the workspace's events", int64(1),
		"SELECT count(DISTINCT recorded_by) FROM t_events WHERE workspace_id = $1", ws)
	r.dbCheck("and it is the token's user", true,
		`SELECT bool_and(e.recorded_by = u.id) FROM t_events e, t_users u
		 WHERE e.workspace_id = $1 AND u.external_id = $2`, ws, r.Token.Subject)
	if r.DB != nil {
		if n, err := r.DB.Value(r.ctx, "SELECT count(*) FROM t_events WHERE workspace_id = $1", ws); err == nil {
			r.Report.Info("events in the workspace: %v", n)
		}
	}
}

func (r *run) feeds(ws, cash, card string) {
	r.Report.Section("10. Feeds")
	feed := "/workspaces/" + ws + "/feed"
	cashFeed := "/workspaces/" + ws + "/accounts/" + cash + "/feed"
	cardFeed := "/workspaces/" + ws + "/accounts/" + card + "/feed"

	r.code(r.call(http.MethodGet, feed, nil), 200, "the workspace's feed")
	r.check("the operation and the transfer, one item each", kinds(decode[feedPage](r)), "OPERATION,TRANSFER")
	r.code(r.call(http.MethodGet, cashFeed, nil), 200, "cash's feed")
	page := decode[feedPage](r)
	r.check("cash: its anchor, the operation and the transfer", kinds(page), "BALANCE_ANCHOR,OPERATION,TRANSFER")
	r.check("cash is the transfer's source", sideOf(page), "SOURCE")
	r.code(r.call(http.MethodGet, cardFeed, nil), 200, "card's feed")
	page = decode[feedPage](r)
	r.check("card: its anchor and the transfer, a first anchor having no difference", kinds(page), "BALANCE_ANCHOR,TRANSFER")
	r.check("card is the transfer's target", sideOf(page), "TARGET")
	r.code(r.call(http.MethodGet, feed+"?kinds=BALANCE_ANCHOR", nil), 400, "a kind the workspace's feed never has is refused")

	r.walk("the workspace's feed", feed)
	r.walk("cash's feed", cashFeed)
}

// walk pages one item older and back, so a cursor is proved by the request it comes back in.
func (r *run) walk(label, feed string) {
	r.code(r.call(http.MethodGet, feed+"?size=1", nil), 200, label+": a page of one")
	first := decode[feedPage](r)
	r.check(label+": one item", len(first.Items), 1)
	r.check(label+": nothing newer than the first page", first.Page.Newer == nil, true)
	if first.Page.Older == nil {
		r.check(label+": a cursor to older items", "none", "one")
		return
	}
	r.code(r.call(http.MethodGet, feed+"?size=1&older="+*first.Page.Older, nil), 200, label+": the next page")
	second := decode[feedPage](r)
	r.check(label+": another item", cursorOf(second) != cursorOf(first), true)
	if second.Page.Newer == nil {
		r.check(label+": a cursor back to newer items", "none", "one")
		return
	}
	r.code(r.call(http.MethodGet, feed+"?size=1&newer="+*second.Page.Newer, nil), 200, label+": back again")
	r.check(label+": the first item again", cursorOf(decode[feedPage](r)), cursorOf(first))
}

func kinds(p feedPage) string {
	var found []string
	for _, item := range p.Items {
		found = append(found, item.Type)
	}
	slices.Sort(found)
	return strings.Join(found, ",")
}

func sideOf(p feedPage) string {
	for _, item := range p.Items {
		if item.Type == "TRANSFER" {
			return item.Side
		}
	}
	return ""
}

func cursorOf(p feedPage) string {
	if len(p.Items) == 0 {
		return ""
	}
	return p.Items[0].Cursor
}

func (r *run) lists(ws, query string) bool {
	r.code(r.call(http.MethodGet, "/workspaces"+query, nil), 200, "workspaces listed"+query)
	list := decode[wholeList[workspace]](r)
	r.check("the list answers whole, with no page", list.Page == nil, true)
	return slices.ContainsFunc(list.Items, func(w workspace) bool { return w.ID == ws })
}

func (r *run) lifecycle() {
	r.Report.Section("11. Workspace lifecycle: NEW -> ACTIVE <-> ARCHIVED -> DELETED")
	r.code(r.call(http.MethodPost, "/workspaces", map[string]any{"workspaceName": "smoke-lifecycle", "defaultCurrency": "EUR"}),
		201, "second workspace created")
	w := decode[workspace](r)
	ws := w.ID
	r.track(ws)
	r.check("starts NEW", w.Status, "NEW")
	r.check("starts at version 0", w.Version, 0)

	r.Report.Note("NEW is not archivable, and not yet active")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/archive", nil), 409, "archive from NEW is refused")
	r.problem("WORKSPACE_STATUS_CONFLICT")
	r.code(r.call(http.MethodDelete, "/workspaces/"+ws+"/archive", nil), 409, "unarchive from NEW is refused")
	r.check("still NEW after both refusals", r.statusOf(ws), "NEW")

	r.Report.Note("optimistic locking on the rename")
	r.code(r.call(http.MethodPut, "/workspaces/"+ws, map[string]any{"version": 99, "workspaceName": "stale", "defaultCurrency": "EUR"}),
		409, "a stale version is refused")
	r.problem("VERSION_CONFLICT")
	r.check("the refused rename changed nothing", r.statusOf(ws), "NEW")
	r.code(r.call(http.MethodPut, "/workspaces/"+ws, map[string]any{"version": 0, "workspaceName": "smoke-renamed", "defaultCurrency": "EUR"}),
		200, "rename with the current version")
	r.call(http.MethodGet, "/workspaces/"+ws, nil)
	renamed := decode[workspace](r)
	r.check("the new name is stored", renamed.Name, "smoke-renamed")
	r.check(fmt.Sprintf("the write bumped the version (%d)", renamed.Version), renamed.Version > 0, true)
	r.check("a rename is not a command, so the workspace is still NEW", r.statusOf(ws), "NEW")

	r.Report.Note("the first command activates it")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/accounts", account("wallet", nil)), 201, "account created")
	r.check("NEW -> ACTIVE at the first command, with no endpoint for it", r.statusOf(ws), "ACTIVE")

	r.Report.Note("ARCHIVED is read-only system-wide")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/archive", nil), 200, "archived")
	r.check("status is ARCHIVED", r.statusOf(ws), "ARCHIVED")
	r.check("hidden from the listing by default", r.lists(ws, ""), false)
	r.check("listed when asked for", r.lists(ws, "?includeArchived=true"), true)
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/archive", nil), 200, "archiving again is idempotent at the target")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/accounts", account("denied", nil)), 409,
		"a command into an ARCHIVED workspace is refused")
	r.problem("WORKSPACE_READ_ONLY")
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/accounts?includeArchived=true", nil), 200, "reads still answer while ARCHIVED")
	r.code(r.call(http.MethodGet, "/workspaces/"+ws+"/statistics/balances/accounts", nil), 200,
		"statistics still answer while ARCHIVED")

	r.Report.Note("back to ACTIVE, which is the recoverable path")
	r.code(r.call(http.MethodDelete, "/workspaces/"+ws+"/archive", nil), 200, "unarchived")
	r.check("status is ACTIVE again", r.statusOf(ws), "ACTIVE")
	r.code(r.call(http.MethodDelete, "/workspaces/"+ws+"/archive", nil), 200, "unarchiving again is idempotent at the target")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/accounts", account("allowed", nil)), 201, "commands are accepted again")

	r.Report.Note("deletion is guarded by the version, and terminal")
	r.code(r.call(http.MethodDelete, "/workspaces/"+ws, nil), 400, "delete without a version is refused")
	r.problem("MALFORMED_REQUEST")
	r.code(r.call(http.MethodDelete, "/workspaces/"+ws+"?version=99", nil), 409, "delete with a stale version is refused")
	r.problem("VERSION_CONFLICT")
	r.check("still ACTIVE after both refusals", r.statusOf(ws), "ACTIVE")
	v := r.versionOf(ws)
	r.code(r.deleteWorkspace(ws, v), 204, fmt.Sprintf("deleted with the current version (%d)", v))
	r.code(r.call(http.MethodGet, "/workspaces/"+ws, nil), 404, "a DELETED workspace is gone from the API")
	r.problem("NOT_FOUND")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/archive", nil), 404, "archiving it answers 404, not 409")
	r.code(r.call(http.MethodPost, "/workspaces/"+ws+"/accounts", account("gone", nil)), 404, "a command answers 404")
	r.check("it is absent from the caller's listing, archived ones included", r.lists(ws, "?includeArchived=true"), false)
	r.dbCheck("the row survives as DELETED with deleted_at set, for the retention job", true,
		"SELECT status = 'DELETED' AND deleted_at IS NOT NULL FROM t_workspaces WHERE id = $1", ws)
}

func (r *run) call(method, path string, body any) apiclient.Response {
	r.last = r.Client.Do(r.ctx, method, api+path, body)
	return r.last
}

func (r *run) check(label string, got, want any) {
	r.Report.Equal(label, got, want, r.last.Body)
}

func (r *run) code(resp apiclient.Response, want int, label string) {
	r.last = resp
	r.check(label, resp.Code, want)
}

// problem checks the last answer is Core's ProblemDetail: its code, and a trace id to quote.
func (r *run) problem(code string) {
	p := decode[problem](r)
	r.check("  a problem+json body", r.last.Header.Get("Content-Type"), "application/problem+json")
	r.check("  its code is "+code, p.Code, code)
	r.check("  it carries a traceId", p.TraceID != "", true)
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

// decode fails on a body that does not parse, since its zero value would pass a check that something
// is absent.
func decode[T any](r *run) T {
	var v T
	if err := json.Unmarshal(r.last.Body, &v); err != nil {
		r.check(fmt.Sprintf("the %d response is readable", r.last.Code), err, nil)
	}
	return v
}

func (r *run) statusOf(ws string) string {
	r.call(http.MethodGet, "/workspaces/"+ws, nil)
	return decode[workspace](r).Status
}

func (r *run) versionOf(ws string) int64 {
	r.call(http.MethodGet, "/workspaces/"+ws, nil)
	return decode[workspace](r).Version
}

func (r *run) deleteWorkspace(ws string, version int64) apiclient.Response {
	resp := r.call(http.MethodDelete, fmt.Sprintf("/workspaces/%s?version=%d", ws, version), nil)
	if resp.Code == http.StatusNoContent {
		r.deleted[ws] = true
	}
	return resp
}

func (r *run) track(ws string) {
	if ws != "" {
		r.created = append(r.created, ws)
	}
}

func (r *run) cleanUp() {
	// An interrupted run still deletes what it created.
	r.ctx = context.WithoutCancel(r.ctx)
	for _, ws := range r.created {
		if r.deleted[ws] {
			continue
		}
		if resp := r.deleteWorkspace(ws, r.versionOf(ws)); resp.Code != http.StatusNoContent {
			r.Report.Info("could not delete workspace %s: %d", ws, resp.Code)
		}
	}
}

func (r *run) now() string {
	return time.Now().In(r.Zone).Format("2006-01-02T15:04:05")
}

func (r *run) pause() {
	select {
	case <-time.After(2 * time.Second):
	case <-r.ctx.Done():
	}
}

func account(name string, initialBalance any) map[string]any {
	return map[string]any{"name": name, "currency": "EUR", "icon": nil, "initialBalance": initialBalance}
}

func balanceOf(bs []balance, account string) any {
	for _, b := range bs {
		if b.AccountID == account {
			return b.Balance
		}
	}
	return "missing"
}
