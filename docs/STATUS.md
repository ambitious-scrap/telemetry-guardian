# Telemetry Guardian status

## Current phase

- Phase: 4 — Verification engine and evidence semantics
- Owner: Codex implementation
- Branch: `phase/4-verification-engine`
- State: acceptance passed; awaiting external review
- Scope: four canonical checks, evidence-complete verdicts, stable exit codes,
  bounded polling, stale-state isolation, and Phase 4 acceptance only

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

CI, blast graph, reporting, and product UI remain assigned to later phases.
