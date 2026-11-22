// Command fintrace-compare compares the old MOK bot's balance report with Core's balances for the same dump:
// every currency and account, by day, week, month and year (task 3.13).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/fintrace-project/fintrace-tests/smoke/internal/apiclient"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/auth"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/botcompare"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/botreport"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/calllog"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/dump"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/envfile"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/report"
)

const (
	api        = "/api/v1"
	testClient = "fintrace-test"
)

type account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Archived bool   `json:"archived"`
}

type series struct {
	Buckets []struct {
		Start   string `json:"start"`
		Balance struct {
			ByCurrency []struct {
				Currency string  `json:"currency"`
				Total    float64 `json:"total"`
			} `json:"byCurrency"`
		} `json:"balance"`
	} `json:"buckets"`
}

func main() {
	os.Exit(run())
}

func run() int {
	envFile := flag.String("env-file", "", "KEY=VALUE file for FINTRACE_OAUTH2_ISSUER_URI and FINTRACE_TEST_CLIENT_SECRET; the environment wins over it")
	coreURL := flag.String("core-url", "http://localhost:8080", "fintrace-core's base URL")
	importerURL := flag.String("importer-url", "http://localhost:8090", "fintrace-mok-importer's base URL, for --mok and --test-dump")
	dumps := flag.String("dumps", "../dumps", "directory of synthetic dumps, laid out as <format>/<size>/")
	testDump := flag.String("test-dump", "", "a synthetic dump's size, e.g. S: imports dumps/mok/<size>/dump.sql and compares with its references/mok-bot-report-<size>.xlsx")
	mokPath := flag.String("mok", "", "a .mok file to import into a new workspace")
	workspace := flag.String("workspace", "", "an existing workspace's id to compare, unchanged")
	xlsxPath := flag.String("xlsx", "", "the bot's report; defaults to the test dump's reference with --test-dump")
	keep := flag.Bool("keep", false, "keep the workspace an import created, rather than deleting it at the end")
	all := flag.Bool("all", false, "list every mismatching period, not only each row's first")
	tolerance := flag.Float64("tolerance", 0.01, "how far apart two figures may be and still agree: the bot's doubles against Core's exact decimals")
	logLevel := flag.String("log-level", "warn", "debug shows every call's headers and bodies, info every call")
	flag.Parse()

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return usage("bad --log-level: %v", err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if sources := count(*testDump != "", *mokPath != "", *workspace != ""); sources != 1 {
		return usage("give exactly one of --test-dump, --mok and --workspace")
	}
	var mok []byte
	label := ""
	switch {
	case *testDump != "":
		dir := filepath.Join(*dumps, "mok", *testDump)
		d, err := dump.Load(dir)
		if err != nil {
			return usage("--test-dump %s: %v", *testDump, err)
		}
		mok, label = d.SQLite, "test dump "+*testDump
		if *xlsxPath == "" {
			*xlsxPath = filepath.Join(dir, "references", "mok-bot-report-"+*testDump+".xlsx")
		}
	case *mokPath != "":
		raw, err := os.ReadFile(*mokPath)
		if err != nil {
			return usage("--mok: %v", err)
		}
		mok, label = raw, filepath.Base(*mokPath)
	}
	if *xlsxPath == "" {
		return usage("--xlsx is required with --mok and --workspace")
	}
	bot, err := botreport.Read(*xlsxPath)
	if err != nil {
		return usage("%v", err)
	}

	env, err := envfile.Load(*envFile)
	if err != nil {
		return usage("%v", err)
	}
	issuer, secret := env.Get("FINTRACE_OAUTH2_ISSUER_URI"), env.Get("FINTRACE_TEST_CLIENT_SECRET")
	if issuer == "" || secret == "" {
		return usage("FINTRACE_OAUTH2_ISSUER_URI and FINTRACE_TEST_CLIENT_SECRET are required — set them or pass --env-file")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	hc := calllog.NewClient()
	token, err := auth.ClientCredentials(ctx, hc, issuer, testClient, secret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not obtain a token: %v\n", err)
		return 1
	}
	core := apiclient.New(*coreURL, token.Access, hc)
	rep := report.New(os.Stdout)

	ws := *workspace
	if mok != nil {
		ws, err = importInto(ctx, core, apiclient.New(*importerURL, token.Access, hc), label, mok)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		rep.Info("imported %s into workspace %s", label, ws)
		if *keep {
			rep.Info("kept: --workspace %s compares it again", ws)
		} else {
			// Deleted even when the run fails or is interrupted
			defer deleteWorkspace(context.WithoutCancel(ctx), core, ws, rep)
		}
	}

	if err := compare(ctx, core, ws, bot, *tolerance, *all, rep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !rep.Summary() {
		return 1
	}
	return 0
}

func compare(ctx context.Context, core *apiclient.Client, ws string, bot botreport.Report, tolerance float64, all bool, rep *report.Report) error {
	var accounts []account
	if err := get(ctx, core, "/workspaces/"+ws+"/accounts?includeArchived=true", &struct {
		Items *[]account `json:"items"`
	}{&accounts}); err != nil {
		return err
	}
	seen := map[string]bool{}

	for _, sheet := range bot.Sheets {
		rep.Section(sheet.Name)
		rep.Note("expected is the bot's figure, got Core's; within %.2f they agree", tolerance)
		var shared map[string]map[string]float64
		if sheet.Scope == botreport.Currencies {
			var err error
			if shared, err = balances(ctx, core, ws, sheet, ""); err != nil {
				return err
			}
		}
		for _, row := range sheet.Rows {
			currency, figures := row.Label, shared
			if sheet.Scope == botreport.Accounts {
				name, cur, ok := botreport.SplitAccount(row.Label)
				if !ok {
					rep.Equal(row.Label, "not an account label", "Name (CUR)", nil)
					continue
				}
				matches := matching(accounts, name, cur)
				if len(matches) != 1 {
					rep.Equal(row.Label, fmt.Sprintf("%d accounts named so in Core", len(matches)), "exactly one", nil)
					continue
				}
				seen[matches[0].ID] = true
				var err error
				if figures, err = balances(ctx, core, ws, sheet, matches[0].ID); err != nil {
					return err
				}
				currency = cur
			}
			result := botcompare.Row(sheet.Starts, row.Values, column(figures, currency), tolerance)
			say(rep, row.Label, result, all)
		}
	}

	rep.Section("Accounts only Core has")
	for _, a := range accounts {
		if !seen[a.ID] {
			rep.Info("%s (%s)%s, in no account sheet of the report", a.Name, a.Currency, map[bool]string{true: ", archived"}[a.Archived])
		}
	}
	return nil
}

func say(rep *report.Report, label string, r botcompare.Result, all bool) {
	periods := fmt.Sprintf("%d periods", r.Periods)
	if r.Skipped > 0 {
		periods += fmt.Sprintf(", %d empty in the report", r.Skipped)
	}
	if len(r.Mismatches) == 0 {
		rep.Equal(fmt.Sprintf("%s — %s", label, periods), "agree", "agree", nil)
		return
	}
	first := r.Mismatches[0]
	rep.Equal(fmt.Sprintf("%s — %d of %s differ, the first %s", label, len(r.Mismatches), periods, first.Start),
		coreFigure(first.Core), fmt.Sprintf("%.2f", first.Bot), nil)
	if all {
		for _, m := range r.Mismatches[1:] {
			rep.Note("%s: the bot %.2f, Core %s", m.Start, m.Bot, coreFigure(m.Core))
		}
	}
}

func coreFigure(v *float64) string {
	if v == nil {
		return "no bucket"
	}
	return fmt.Sprintf("%.2f", *v)
}

// balances reads Core's series over the sheet's periods, a figure per bucket start and currency. `to` is the
// last period's last day: Core reads a bucket's balance no later than `to` plus a day.
func balances(ctx context.Context, core *apiclient.Client, ws string, sheet botreport.Sheet, accountID string) (map[string]map[string]float64, error) {
	q := url.Values{
		"granularity":     {string(sheet.Granularity)},
		"from":            {sheet.Starts[0]},
		"to":              {botreport.LastDay(sheet.Granularity, sheet.Starts[len(sheet.Starts)-1])},
		"includeArchived": {"true"},
	}
	if accountID != "" {
		q.Set("accountIds", accountID)
	}
	var s series
	if err := get(ctx, core, "/workspaces/"+ws+"/statistics/balances/series?"+q.Encode(), &s); err != nil {
		return nil, err
	}
	out := map[string]map[string]float64{}
	for _, b := range s.Buckets {
		out[b.Start] = map[string]float64{}
		for _, a := range b.Balance.ByCurrency {
			out[b.Start][a.Currency] = a.Total
		}
	}
	return out, nil
}

func column(figures map[string]map[string]float64, currency string) map[string]float64 {
	out := map[string]float64{}
	for start, byCurrency := range figures {
		if v, ok := byCurrency[currency]; ok {
			out[start] = v
		}
	}
	return out
}

// matching finds Core's accounts the bot's `Name (CUR)` can mean: the report carries no source uid, so two
// accounts of one name and currency are ambiguous and reported, never guessed between.
func matching(accounts []account, name, currency string) []account {
	var out []account
	for _, a := range accounts {
		if a.Name == name && a.Currency == currency {
			out = append(out, a)
		}
	}
	return out
}

func importInto(ctx context.Context, core, importer *apiclient.Client, label string, mok []byte) (string, error) {
	resp := core.Do(ctx, http.MethodPost, api+"/workspaces", map[string]any{
		"workspaceName":   workspaceName(label),
		"defaultCurrency": "EUR",
	})
	if resp.Code != http.StatusCreated {
		return "", fmt.Errorf("creating a workspace: %d %s", resp.Code, resp.Body)
	}
	var w struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Body, &w); err != nil {
		return "", err
	}
	resp = importer.Upload(ctx, api+"/workspaces/"+w.ID+"/import", "file", "dump.mok", mok)
	var job struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(resp.Body, &job)
	if resp.Code != http.StatusOK || job.Status != "SUCCEEDED" {
		deleteWorkspace(context.WithoutCancel(ctx), core, w.ID, nil)
		return "", fmt.Errorf("importing %s: %d %s", label, resp.Code, resp.Body)
	}
	return w.ID, nil
}

// workspaceName names the workspace after what was imported, within Core's name rule: no full stop (§4.13.2),
// so a file's extension goes and any other dot becomes a hyphen.
func workspaceName(label string) string {
	label = strings.TrimSuffix(label, filepath.Ext(label))
	return "compare " + strings.ReplaceAll(label, ".", "-")
}

func deleteWorkspace(ctx context.Context, core *apiclient.Client, ws string, rep *report.Report) {
	var w struct {
		Version int64 `json:"version"`
	}
	if err := get(ctx, core, "/workspaces/"+ws, &w); err == nil {
		resp := core.Do(ctx, http.MethodDelete, fmt.Sprintf("%s/workspaces/%s?version=%d", api, ws, w.Version), nil)
		if resp.Code == http.StatusNoContent {
			return
		}
	}
	if rep != nil {
		rep.Info("could not delete workspace %s; delete it by hand", ws)
	}
}

func get(ctx context.Context, core *apiclient.Client, path string, into any) error {
	resp := core.Do(ctx, http.MethodGet, api+path, nil)
	if resp.Code != http.StatusOK {
		return fmt.Errorf("GET %s: %d %s", path, resp.Code, strings.TrimSpace(string(resp.Body)))
	}
	return json.Unmarshal(resp.Body, into)
}

func count(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

func usage(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "fintrace-compare: "+format+"\n\n", args...)
	flag.Usage()
	return 2
}
