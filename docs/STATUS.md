# Telemetry Guardian status

## Current phase

- Phase: 1 — Foundry environment and deterministic demo application
- Owner: Codex implementation
- Branch: `phase/1-foundry-demo`
- State: acceptance passed; awaiting external review
- Scope: Foundry, demo variants, deterministic load/fault, seeded resources, and Phase 1 acceptance only

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

## Deferred work

SigNoz adapter, contract mining, verification, evidence, CI, blast graph, and
product UI remain assigned to later phases.
