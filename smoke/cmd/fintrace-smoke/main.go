// Command fintrace-smoke runs one application's smoke suite against a running stack.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/fintrace-project/fintrace-tests/smoke/internal/apiclient"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/auth"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/calllog"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/db"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/envfile"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/report"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/suite"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/suite/core"
)

const testClient = "fintrace-test"

var suites = map[string]suite.Func{
	"fintrace-core": core.Run,
}

func main() {
	os.Exit(run())
}

func run() int {
	apps := slices.Sorted(maps.Keys(suites))
	app := flag.String("app", "", "application to smoke-test: "+strings.Join(apps, ", "))
	url := flag.String("url", "http://localhost:8080", "the application's base URL")
	envFile := flag.String("env-file", "", "KEY=VALUE file for FINTRACE_OAUTH2_ISSUER_URI, FINTRACE_TEST_CLIENT_SECRET and POSTGRES_PASSWORD; the environment wins over it")
	dsn := flag.String("db", "", "Postgres DSN for the checks behind the API, e.g. postgres://fintrace@127.0.0.1:5432/fintrace; without a password it takes POSTGRES_PASSWORD. Omitted, those checks are skipped")
	zone := flag.String("tz", envOr("TZ", "Europe/Nicosia"), "zone the business timestamps are written in")
	logLevel := flag.String("log-level", "warn", "debug shows every call's headers and bodies, info every call")
	flag.Parse()

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return usage("bad --log-level: %v", err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	runSuite, ok := suites[*app]
	if !ok {
		return usage("--app must be one of: %s", strings.Join(apps, ", "))
	}
	loc, err := time.LoadLocation(*zone)
	if err != nil {
		return usage("bad --tz: %v", err)
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
		fmt.Fprintf(os.Stderr, "check the issuer and FINTRACE_TEST_CLIENT_SECRET, and that %s has service accounts enabled with the fintrace:user realm role\n", testClient)
		return 1
	}

	var database *db.DB
	if *dsn != "" {
		if database, err = db.Connect(ctx, *dsn, env.Get("POSTGRES_PASSWORD")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer database.Close(context.WithoutCancel(ctx))
	}

	rep := report.New(os.Stdout)
	runSuite(ctx, suite.Target{
		Client: apiclient.New(*url, token.Access, hc),
		Token:  token,
		DB:     database,
		Report: rep,
		Zone:   loc,
	})
	if !rep.Summary() {
		return 1
	}
	return 0
}

func usage(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "fintrace-smoke: "+format+"\n\n", args...)
	flag.Usage()
	return 2
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
