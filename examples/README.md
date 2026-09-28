# Automation Examples

These POSIX shell workflows are executable documentation for the current
`chab` command surface. The repository checker runs each script against a fresh
local mock with disposable home, config, auth, trace, credential, and
idempotency state.

- [CI environment auth and errors](ci/env-auth-and-errors.sh) uses only
  `CHAB_API_KEY` and `CHAB_BASE_URL`, first reports the effective non-secret
  runtime, reads Credits in JSON and built-in jq modes, handles an exact
  not-found exit, and proves no local state was written.
- [Support profile metadata](support/profile-metadata.sh) creates and
  automatically selects the first isolated profile, inspects config and
  Credits, and extracts request and pagination context.
- [Idempotent raw mutation](agents/idempotent-raw-mutation.sh) previews a raw
  POST offline, submits one explicit idempotency key, then repeats it to show
  local recovery of the known remote action without sending a second mutation.

Run the complete non-live example check from the repository root:

```bash
go run ./scripts/check-examples
```

Repository CI runs this command in its Linux quality job. Cross-platform jobs
retain portable validation and build coverage; Windows does not execute the
POSIX-shell workflows.

The checker builds `chab`, the mock API, and a value-blind invocation shim. It
validates [check-examples.json](check-examples.json), runs each listed script,
compares observed command paths and flag names with the manifest in both
directions, enforces request budgets, and scans completed output, traces, and
mock logs for protected values. It never discovers or runs `examples/live`.

## Mock and isolated environment

To inspect the standalone deterministic mock:

```bash
go run ./examples/mockapi --help
go run ./examples/mockapi --listen 127.0.0.1:0
```

In normal manual use, the mock accepts any non-empty bearer token and
idempotency key. The checker starts a new verifying instance for each script
and supplies only fingerprints to it. Scripts receive an allowlisted
environment containing `PATH`, an isolated `HOME`, `LC_ALL=C`, disposable
`CHAB_CONFIG` and `CHAB_AUTH_FILE` paths, the fresh `CHAB_API_KEY` and
`CHAB_BASE_URL`, plus checker-private binary, trace, and idempotency values.

Every checked mutating request must pass the runner-supplied explicit
idempotency key; an automatically generated key is unknown to the verifying
mock and is rejected.

Examples should use `--json` when a stable document is the useful artifact and
the built-in `--jq` flag when a smaller derived value is sufficient. No
external jq executable is required. `--include-meta` is available only on
API-backed Credits and raw API leaves, requires JSON, jq, or template
output, and cannot be combined with an active `--dry-run`.

## GitHub Actions

Environment-only auth is suitable for CI because it does not provision config
or auth files. The job can report its effective non-secret context first;
`CHAB_API_KEY` still comes from the CI provider's secret store before the same
API and error checks run:

```yaml
jobs:
  inspect-credits:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go build -o "$RUNNER_TEMP/chab" ./cmd/chab
      - run: |
          "$CHAB_BIN" auth env --json
          "$CHAB_BIN" api get /credits --jq '.spendable_balance'
        env:
          CHAB_BIN: ${{ runner.temp }}/chab
          CHAB_API_KEY: ${{ secrets.CHAB_API_KEY }}
          CHAB_BASE_URL: https://api.example.test
```

The deterministic scripts above are mock-backed and safe for repository gates.
Real-service verification remains an explicit opt-in boundary in
[`scripts/smoke-release-live.sh`](../scripts/smoke-release-live.sh); the local
checker never invokes that live smoke.
