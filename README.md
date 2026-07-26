# Telemetry Guardian

Telemetry Guardian mines a telemetry contract from the dashboards and alerts
that actually consume your telemetry, then verifies a candidate release against
that contract before it ships.

The protected demo proves one story end to end against a live, isolated SigNoz
instance: a healthy release passes, a functionally identical release with
renamed telemetry silently breaks a dashboard panel and disables a critical
alert, Guardian names every affected consumer and blocks the release, and the
repaired release passes and fires the alert again.

## Prerequisites

| Requirement | Notes |
|---|---|
| Docker | Running daemon. The demo starts an isolated SigNoz stack and one demo service. |
| `foundryctl` | Casts the isolated SigNoz deployment from `foundry/casting.yaml`. |
| Go 1.22+ | Builds the `guardian` CLI and the demo checkout service. |
| `jq`, `curl`, `openssl` | Used by the seed, load, and assertion scripts. |
| Free local ports | `18080` (SigNoz), `19090` (checkout), `14318` (OTLP), `13134` (collector health). |

No environment variables are required for the demo. The demo creates its own
isolated SigNoz organisation and access token at runtime and never prints them.

Optional overrides, all defaulted for local use: `SIGNOZ_URL`, `CHECKOUT_URL`,
`OTLP_URL`, `COLLECTOR_HEALTH_URL`, `GOCACHE`.

`SIGNOZ_URL` and `SIGNOZ_TOKEN` are only needed when Guardian runs in GitHub
Actions against an existing SigNoz instance — see Troubleshooting.

## Run the demo

```sh
make demo
```

One command. It starts the isolated environment, runs the whole healthy →
broken → repaired narrative, preserves every artifact, and prints a final
summary. It never asks for input.

For the non-interactive smoke path used by CI and acceptance:

```sh
make demo-smoke
```

Both modes execute the same orchestration and the same assertions; smoke mode
routes sub-step output to the log instead of the terminal and fails immediately
on the first unexpected result.

The full acceptance gate — smoke run plus independent re-verification of the
preserved artifacts — is:

```sh
make accept-phase6
```

### Expected duration

The demo runs live against a real SigNoz instance and waits for real ingestion
and real alert evaluation. Expect several minutes: a cold SigNoz start, a
schema warmup, and three release stages that each align to a one-minute alert
evaluation bucket. It is not instant, and it is not intended to be.

### Expected outcome

| Stage | Guardian verdict | Alert |
|---|---|---|
| Healthy release | `PASS` — 4 passing checks | fires |
| Broken release | `FAIL` — `required-field-cart-value`, `required-field-error-type`, `alert-must-fire-payment-timeout` fail; `required-operation-payment-authorize` still passes | misses |
| Repaired release | `PASS` — 4 passing checks | fires |

Functional tests and functional responses are byte-identical across all three
releases. That is the point: nothing functional broke.

### Artifacts

Everything is written to `.run/demo/` (the previous run is moved to
`.run/demo.prev/` at startup, so a failed run's evidence survives):

- `contract-healthy.yaml`, `contract-broken.yaml`, `contract-repaired.yaml` — mined consumer contracts
- `verdict-healthy.json`, `verdict-broken.json`, `verdict-repaired.json` — verification verdicts
- `blast-graph-broken.html`, `blast-graph-broken.md` — consumer blast graph for the broken release
- `alert-healthy.jsonl`, `alert-broken.jsonl`, `alert-repaired.jsonl` — alert notification evidence
- `functional-*.json`, `functional-tests-*.log`, `fault-*.json` — functional-test evidence
- `demo.log` — full stage log
- `summary.md` — final summary

Open `.run/demo/blast-graph-broken.html` in a browser. It renders offline with
no network access.

Generated runtime artifacts are not committed.

## Troubleshooting

**Cold SigNoz schema warmup.** A freshly cast SigNoz discovers trace fields
lazily, so the first candidate verification against a cold instance can report
missing fields. The demo deploys a dedicated warmup release with its own run ID
before any candidate stage, and asserts that the warmup run ID never appears in
candidate evidence. If you run `guardian verify` by hand against a cold
instance, warm it the same way first.

**Minute-bucket alert alignment.** Live SigNoz trace-alert queries only expose a
fault once the current minute bucket closes. The demo waits for a deterministic
position inside the minute before injecting the payment timeout and stops if it
misses that window. This is why each release stage takes about a minute longer
than the work it performs.

**GitHub Actions repository configuration.** The `guardian` workflow verifies a
candidate against an existing SigNoz instance and needs repository variable
`SIGNOZ_URL` and repository secret `SIGNOZ_TOKEN`. Without them the workflow
fails at its connectivity step with `URL rejected: No host part in the URL`.
This is a repository configuration prerequisite, not a code failure; the local
demo does not use either value.

## Repository layout

- `cmd/guardian` — `mine`, `verify`, `report`
- `internal/` — SigNoz adapter, contracts, miner, verifier, evidence, report
- `demo/checkout` — demo service with healthy and broken telemetry variants
- `foundry/` — isolated SigNoz casting
- `scripts/` — environment, seed, load, CI, demo, and acceptance scripts
- `Telemetry_Guardian_PRODUCT_SPEC.md`, `Telemetry_Guardian_BUILD_PLAN.md` — the authoritative product documents
