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

The workspace's `./run-smoke.sh` runs all three against `./run-local.sh -d`.

Configuration it reads: `FINTRACE_OAUTH2_ISSUER_URI`, `FINTRACE_TEST_CLIENT_SECRET`, and
`POSTGRES_PASSWORD` when the `--db` DSN carries no password. It prints no secret and no token, and
deletes every workspace it creates, even when a check fails or the run is interrupted.

Exit status: 0 all passed, 1 a check failed or the token could not be obtained, 2 bad usage.
