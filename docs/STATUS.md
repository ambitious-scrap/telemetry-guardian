# Telemetry Guardian status

## Current phase

- Phase: 0 — specification freeze and architecture
- Owner: Codex implementation
- Branch: `phase/0-bootstrap-architecture`
- State: Phase 0 acceptance passed; external review pending
- Scope: frozen to the protected MVP in the two root authority documents

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

## Deferred work

All product code, Foundry resources, fixtures, API adapters, contract mining,
verification, evidence, CI, and UI remain assigned to later phases.
