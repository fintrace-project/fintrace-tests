# fintrace-tests

Suites that run against a running fintrace stack. A test that needs only one service stays in that
service's repository; one that needs the stack lives here.

## Smoke

`fintrace-smoke` runs one application's smoke suite against its base URL, with a real token from
the `fintrace-test` client's service account.

```bash
cd smoke
go build ./cmd/fintrace-smoke

./fintrace-smoke --app fintrace-core --url http://localhost:8080 --env-file ../../infra/.env \
  --db postgres://fintrace@127.0.0.1:5432/fintrace
```

| Flag          | Default                 | Meaning                                                                    |
| ------------- | ----------------------- | -------------------------------------------------------------------------- |
| `--app`       | —                       | `fintrace-core`, `fintrace-mok-importer` or `fintrace-bff`                 |
| `--url`       | the app's local port    | its base URL: Core 8080, the importer 8090, the BFF 8100                   |
| `--core-url`  | `http://localhost:8080` | Core, for a suite that sets up or reads back through it                    |
| `--dumps`     | `../dumps`              | the synthetic dumps, as `<format>/<size>/`                                 |
| `--env-file`  | none                    | `KEY=VALUE` file; the process environment wins over it                     |
| `--db`        | none                    | Postgres DSN for the checks behind the API; without it they are skipped    |
| `--tz`        | `$TZ`, `Europe/Nicosia` | zone the business timestamps are written in                                |
| `--log-level` | `warn`                  | `info` logs every call, `debug` its headers and bodies, secrets masked     |

The suites:

- **`fintrace-core`** — the data path (accounts, a category, an operation, a transfer, an anchor,
  balances) and a workspace's whole lifecycle.
- **`fintrace-mok-importer`** — S uploaded to the importer, every balance compared in Core.
- **`fintrace-bff`** — the edge on `fintrace-test`'s token: health and the security headers,
  authentication, routing (the BFF's 404, the login redirect to the realm, the web UI), Core through
  the edge with `Location` naming the BFF, the CSRF refusal; then S end to end through the BFF —
  uploaded, every balance read back, a first expense moving only its account.

The workspace's `./run-smoke-tests.sh` runs all three against `./run-local.sh -d`.

Configuration it reads: `FINTRACE_OAUTH2_ISSUER_URI`, `FINTRACE_TEST_CLIENT_SECRET`, and
`POSTGRES_PASSWORD` when the `--db` DSN carries no password. It prints no secret and no token, and
deletes every workspace it creates, even when a check fails or the run is interrupted.

Exit status: 0 all passed, 1 a check failed or the token could not be obtained, 2 bad usage.

## Browser

`browser/` holds the flows through the running stack in a real browser (design-decisions §3.6):
Playwright (`@playwright/test`) in TypeScript, on the phone profile, run in Playwright's own image, so
the host needs neither Node nor a browser. The workspace's `./run-web-ui-tests.sh` runs them against
`./run-local.sh -d`; it is kept out of `./run-smoke-tests.sh`, which runs unattended.

```bash
./run-web-ui-tests.sh                       # from the workspace root
./run-web-ui-tests.sh -- --grep placeholder # flags after -- go to `playwright test`
```

**Signing in is done by hand, so no password is stored anywhere.** The run opens a visible browser on this
machine's display (WSLg's on WSL), at the realm's form; sign in as the test user — a realm user holding
`fintrace:user` and nothing else — within five minutes, and the flows then run headless on that session.
With `FINTRACE_TEST_USERNAME` and `FINTRACE_TEST_PASSWORD` set in the shell — from a secret manager,
e.g. `FINTRACE_TEST_PASSWORD="$(pass show fintrace/test)"`, never a file — the form is filled in
unattended instead.

- **`sign-in.setup.ts`** — from the welcome page through Keycloak's form back to `/workspaces`. The
  session it saves in `.auth/` starts every flow.
- **`operations.spec.ts`** — on an account's page: an expense added back-dated, edited into an income,
  deleted, the balance following each; and an operation made elsewhere whose category the page has
  not read yet, shown with a placeholder and then its name, never its id.

Each flow sets up a workspace of its own through the BFF's `/api` on the browser's session, and
deletes it afterwards, even when the flow failed. A failed flow keeps its trace in
`browser/test-results/`: `npx playwright show-trace <trace.zip>`.

For an editor or a type-check, install and check inside the same image:

```bash
docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -v "$PWD/browser":/w -w /w \
  mcr.microsoft.com/playwright:v1.64.1-noble bash -c 'npm ci && npm run format:check && npm run typecheck'
```

The image's version follows `@playwright/test` in `browser/package.json`.
