# Telemetry Guardian status

## Current phase

- Phase: 7 — Release-candidate review
- Owner: Codex review pass
- Branch: `phase/7-release-candidate`
- State: review pass complete; `docs/reviews/release-candidate.md` records
  P0/P1/P2 findings and awaits implementation review
- Scope: review only; no product-code fixes, stretch work, live/demo reruns,
  or `demo-freeze` tag changes

## Authority

- Product: `Telemetry_Guardian_PRODUCT_SPEC.md`
- Delivery: `Telemetry_Guardian_BUILD_PLAN.md`
- Phase architecture: `docs/ARCHITECTURE.md` (non-authoritative)

## Phase 0 outputs

- Public repository bootstrap
- Architecture and language decision
- Thin agent/reviewer instructions
- Make targets and Phase 0 acceptance harness
- Assigned external unknowns

## Critical unknowns

| ID | Critical unknown | Owner | Resolution gate |
|---|---|---|---|
| FD-01 | Foundry casting, lock, readiness, and teardown behavior | Phase 1 environment owner | Phase 1 acceptance |
| OT-01 | OTLP transport, resource fields, correlation, and ingestion timing | Phase 1 demo owner | Phase 1 acceptance |
| DM-01 | Healthy/broken functional equivalence and deterministic fault behavior | Phase 1 demo owner | Phase 1 acceptance |
| SG-01 | Dashboard and alert resource shapes and supported query nodes | Phase 2 adapter owner | Phase 2 acceptance |
| SG-02 | Builder, trace, log, and alert-history request/result behavior | Phase 2 adapter owner | Phase 2 acceptance |
| SG-03 | Authentication, not-found, malformed-response, timeout, and redaction behavior | Phase 2 adapter owner | Phase 2 acceptance |
| CT-01 | Exact fixture-proven extraction forms and YAML dependency | Phase 3 miner owner | Phase 3 acceptance |
| VR-01 | Minimum sample counts, polling intervals, and stale-event rejection | Phase 4 verifier owner | Phase 4 acceptance |
| GH-01 | Actions exit propagation and artifact retention on failure | Phase 5 CI owner | Phase 5 acceptance |
| UI-01 | Offline graph implementation and accessible deterministic layout | Phase 5 UI owner | Phase 5 acceptance |

## Phase 0 empirical findings

On 2026-07-23, the configured SigNoz MCP identity successfully executed
read-only dashboard, alert, and service list operations. Each returned content
plus structured data and pagination; all data arrays were empty. No credentials
or resource values were recorded.

## Phase 1 empirical findings

- Foundry `v0.2.16` generates an isolated Docker Compose deployment from the
  committed casting and lock files.
- SigNoz `v0.133.0` accepts OTLP/HTTP JSON trace and log IDs as hexadecimal
  strings; base64 IDs are rejected.
- Dashboard, channel, and alert creation use the observed authenticated
  `/api/v1/dashboards`, `/api/v1/channels`, and `/api/v2/rules` resource shapes.
- The test environment sets the ruler evaluation delay to zero and uses a
  90-second alert window so SigNoz's notification group wait can complete.
- Foundry's OpAMP-managed default config intermittently replaced ingestion
  pipelines with `nop`; the isolated demo runs the generated static collector
  config directly because OpAMP fleet management is outside MVP scope.
- `scripts/accept/phase1.sh` and `make accept-phase1` each passed three
  consecutive healthy/broken scenarios with clean teardown.

## Phase 2 empirical findings

- Authenticated dashboard retrieval uses `GET /api/v1/dashboards/{id}` and
  alert retrieval uses `GET /api/v2/rules/{id}` with a bearer token.
- Builder, trace, and log queries use `POST /api/v5/query_range` with the
  observed Unix-millisecond `time_series` request and `compositeQuery` builder
  shape. A valid empty `results` or `aggregations: null` response is not an
  adapter error.
- Alert history uses
  `GET /api/v2/rules/{id}/history/timeline` with `start`, `end`, `limit`,
  `order`, `state`, `filterExpression`, and `cursor`; returned cursors are
  preserved for explicit page follow-up.
- Missing bearer, missing resources, and invalid query fields were observed as
  401, 404, and 400 responses respectively. Forbidden, malformed, timeout, and
  cancellation behavior is covered by offline transport tests.
- `internal/signoz/API.md`, fixture-backed fake tests, and the focused
  `scripts/accept/phase2.sh` acceptance path passed against the Phase 1
  instance without recording credentials or raw telemetry.

## Phase 3 implementation findings

- The miner consumes only `internal/signoz.SigNozClient`; no SigNoz HTTP calls
  were added outside the typed adapter.
- The supported shape is one traces Builder query per dashboard panel and one
  traces Builder query per alert, with conjunction filters and the proven
  `sum(cart.value)` / `count()` aggregations.
- Source JSON paths are retained by the typed boundary and emitted on every
  derived requirement and consumer mapping.
- Unsupported query nodes, unknown fields, missing filters or identities, bad
  field types, empty resources, and malformed paths fail explicitly.
- Normalized requirements retain every dashboard-panel and alert consumer;
  run IDs are canonicalized to `__RUN_ID__` so generated output is stable.
- `make accept-phase3` passed the offline suite, fixture golden/mutation tests,
  secret scan, and focused live mining smoke against the seeded Phase 1
  dashboard and alert.

## Phase 4 empirical findings

- SigNoz Builder requests reject response-only `orderBy` nodes; the typed
  adapter now uses a minimal request-only wire.
- Query warnings may be strings or structured objects, and alert-history
  responses use a `status`/`data` envelope with `unixMilli` timestamps.
- Alert history requires `state=firing`; the unsupported literal `state=all`
  returns an empty result.
- Clean Foundry needs a distinct schema-warmup run, and alert injection is
  aligned to the observed minute-bucket boundary. Candidate evidence remains
  isolated by run ID and explicit time windows.
- `scripts/accept/phase4.sh` passed repeated healthy, broken, and no-load
  verdicts with exit codes 0, 1, and 2; invalid contracts exit 3.

## Deferred work

Phase 7 hardening, Phase 8 submission packaging, and every stretch goal remain
deferred. Deferred ideas are recorded in `docs/ROADMAP.md`.

## Phase 5 findings

- CI classification is explicit: exit 0 is `PASS`, exit 1 is
  `TELEMETRY_CONTRACT_VIOLATION`, exit 2 is
  `VERIFICATION_INCONCLUSIVE`, and exit 3 is
  `INVALID_GUARDIAN_CONFIGURATION`. Exit 2 is never rendered as healthy.
- `scripts/ci/guardian.sh` preserves `verdict.json`, a deterministic HTML
  report when the verdict is valid, and a Markdown summary on every exit.
- The report is native HTML/CSS/JavaScript with fixed 1280×720 coordinates,
  no network dependency, and graph relationships only; it does not imply
  incident causality.
- Selected UI direction: dense high-contrast developer evidence console;
  dark slate semantic tokens; JetBrains Mono technical text with IBM Plex
  Sans body text; text/icon/border state cues; native keyboard-accessible
  evidence drawer; visible focus rings; and reduced-motion support.
- Offline fixtures cover healthy, broken, and inconclusive verdicts. Broken
  output names the failed requirement, dashboard panel, and alert, and
  preserves `BREAKS`, `REQUIRED_BY`, and `PART_OF` mappings.

## Phase 6 findings

- `scripts/demo.sh` orchestrates the protected narrative in 22 numbered stages
  and owns no verification logic of its own; it reuses the Phase 1 environment,
  deploy, seed, load, and alert scripts and the Phase 3–5 `guardian mine`,
  `verify`, and `report` commands.
- The repaired release is the restored telemetry contract, so it deploys the
  healthy checkout variant under its own run ID. No new release variant was
  added.
- Fault injection precedes each verification because `alert_must_fire` can only
  observe an alert that already had a fault to react to. All 22 protected
  outcomes are still proven live.
- Alert notification evidence is truncated and captured per stage, so a firing
  notification from an earlier stage can never satisfy a later one.
- `scripts/env/down.sh` now removes only runtime files from `.run`, so preserved
  demo evidence survives teardown and a second invocation starts predictably.
- `make demo` and `make demo-smoke` share one orchestration; smoke mode only
  suppresses sub-step output and is the path `scripts/accept/phase6.sh` drives.

## Phase 7 review findings

- The frozen tree is content-identical to `origin/main`; the Phase 6 merge is
  present at `a883e72`, and the `demo-freeze` tag was not changed.
- Offline `make fmt-check`, `make lint`, `make test`, shell syntax validation,
  `git diff --check`, and the committed-content high-signal secret scan passed.
- The review artifact identifies four P0 correctness blockers around unsupported
  input, canonical contract semantics, returned query timestamps, and
  malformed verdict aggregation.
- The hosted Guardian healthy-workflow criterion remains partial because the
  externally reachable `SIGNOZ_URL` variable and `SIGNOZ_TOKEN` secret are not
  configured; this is not classified as a product-code P0.
