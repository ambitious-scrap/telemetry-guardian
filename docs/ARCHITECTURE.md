# Telemetry Guardian architecture

## Authority and phase

`Telemetry_Guardian_PRODUCT_SPEC.md` defines product scope and
`Telemetry_Guardian_BUILD_PLAN.md` defines delivery order. This document
records Phase 0 architecture; it does not add product requirements.

## Protected MVP

Guardian mines one SigNoz dashboard and one alert into four checks:
`cart.value`, `error.type`, `payment.authorize`, and the `payment-timeout`
alert. It verifies functionally equivalent healthy and telemetry-broken
releases, preserves evidence, identifies affected consumers, blocks broken CI,
demonstrates the alert miss, then proves the repaired alert fires.

## Explicit non-goals

- Root-cause analysis, Epicenter, causal inference, and change-point detection.
- Cardinality forecasting, production remediation, or generic policy engines.
- GitHub App development or SigNoz source modification.
- Universal SigNoz query support, an AI chatbot, or stretch checks before the
  documented stretch gate.

## Implementation language

Neither authority document mandates a language. Go 1.26 is available locally,
matches the recommended `cmd`/`internal` layout, provides typed boundaries,
`context` cancellation, explicit timeouts, a single-binary CLI, and needs no
Phase 0 dependency. Guardian's CLI and backend will therefore use Go. Phase 5
may add a self-contained static HTML report, but no second application runtime
is chosen unless that phase demonstrates a need.

## Package and directory boundaries

Directories are created by their owning phase rather than as empty placeholders.

| Boundary | Responsibility | First owner |
|---|---|---|
| `cmd/guardian` | CLI composition and stable exit codes | Phase 2 |
| `internal/contracts` | Contract model, validation, and YAML serialization | Phase 3 |
| `internal/signoz` | Only typed SigNoz boundary; no response types escape it | Phase 2 |
| `internal/miner` | Fixture-proven consumer dependency extraction | Phase 3 |
| `internal/verifier` | Check execution, state aggregation, and stale isolation | Phase 4 |
| `internal/evidence` | Inspectable query, window, count, summary, and link data | Phase 4 |
| `internal/impact` | Consumer mappings and deterministic graph data | Phase 5 |
| `internal/report` | Terminal, JSON, and Markdown rendering | Phase 5 |
| `foundry`, `demo`, `fixtures` | Reproducible environment, app variants, sanitized data | Phase 1 |
| `web` | Offline deterministic blast-graph report | Phase 5 |
| `scripts` | Phase-owned acceptance, environment, CI, and demo commands | Each phase |

Core packages depend on Guardian domain values, never raw SigNoz responses.
Environment, UI, and CI call the CLI or consume its stable artifacts; they do
not bypass the typed SigNoz adapter.

## Minimum domain model

- `Consumer`: stable ID, name, type, owner, criticality, source reference, and
  requirements. MVP types are dashboard panel and alert.
- `Requirement`: stable ID, check type, signal, optional field/operation/alert
  ID, source path, and bounded parameters.
- `Evidence`: retrieval/query description, explicit time window, sample count,
  result summary, optional returned deep link, and data-quality state.
- `CheckResult`: requirement, state, evidence, and optional violation.
- `Violation`: code, message, requirement, severity, affected consumers,
  evidence, and remediation hint.
- `VerificationResult`: run ID, service, release, start/end times, overall
  state, check results, violations, evidence, and affected consumers.

The model is implemented only when its owning phase needs executable behavior.

## External boundaries

### SigNoz and MCP

All SigNoz access must later pass through one typed, cancelable client. Required
capabilities are dashboard retrieval, alert retrieval, Builder query execution,
trace search, log search where supported, alert-history retrieval, and returned
deep links. Phase 2 must empirically establish operation names, authentication,
request/response shapes, errors, and timeout behavior; no endpoint or schema is
assumed here.

Phase 0 read-only MCP observations on 2026-07-23:

- `signoz_list_dashboards`: PASS; result contains content plus
  `structuredContent.data[]` and pagination.
- `signoz_list_alerts`: PASS; same high-level result shape.
- `signoz_list_services`: PASS; same high-level result shape.
- Each observed data array was empty. No resource values or credentials were
  recorded.

These are empirical MCP wrapper findings, not claims about SigNoz HTTP APIs.

### OpenTelemetry

Phase 1 owns the exact OTLP transport, resource attributes, trace/log/metric
emission, and functional equivalence of healthy and broken variants. Those
details require live ingestion evidence.

### Foundry

Phase 1 must empirically validate configuration syntax, lock behavior,
readiness, lifecycle, cleanup, and reproducibility. This phase invents no
Foundry behavior.

### GitHub Actions

Phase 5 consumes the CLI exit contract: 0 pass, 1 verified violation, 2
inconclusive/infrastructure, and 3 invalid configuration. It must preserve
artifacts even on failure. No GitHub App or Checks API integration is implied.

### UI

Phase 5 owns a deterministic offline HTML blast graph and evidence drawer.
Before UI implementation it must use UI/UX Pro Max to choose accessible status,
layout, typography, motion, responsive, and keyboard behavior. Color alone may
not convey PASS, FAIL, or INCONCLUSIVE; reduced motion and visible focus are
required. The UI must not imply causality.

## Interfaces requiring empirical validation

| Interface | Unknowns to validate | Owner |
|---|---|---|
| Dashboard and alert reads | IDs, query nodes, missing fields, malformed resources | Phase 2 |
| Builder, trace, and log queries | supported request forms, field contexts, result counts | Phase 2 |
| Alert history | state vocabulary, timestamps, freshness, pagination | Phase 2 |
| OTLP ingestion | transport, attributes, correlation, ingestion delay | Phase 1 |
| Foundry | casting syntax, lock generation, readiness, teardown | Phase 1 |
| GitHub Actions | exit propagation and failure-artifact retention | Phase 5 |
| Offline report | browser behavior, stable 1280x720 layout, keyboard access | Phase 5 |

## Fixture-backed and live-integration boundary

Sanitized fixtures cover contract parsing, exact supported dependency shapes,
deduplication, malformed/unsupported inputs, result-state logic, exit mapping,
evidence generation, reports, and graph JSON. Unit and golden tests must run
without SigNoz.

Live tests cover only adapter authentication and response compatibility,
OpenTelemetry ingestion, query timing, dashboard/alert existence, alert history,
healthy/broken verification, and stale-window rejection. Fixture tests cannot
claim that a live alert fired or that telemetry was ingested.

## Result semantics

- `PASS`: the check completed and sufficient in-window data proves satisfaction.
- `FAIL`: the check completed and sufficient in-window data proves violation.
- `INCONCLUSIVE`: evidence is insufficient or verification failed because of
  no data, query/authentication/timeout errors, readiness, unsupported forms, or
  missing/stale alert history.

No-data is always `INCONCLUSIVE`, never `PASS`. Overall state cannot hide an
inconclusive required check.

## Evidence requirements

Every check result preserves its run ID, retrieval or query description,
explicit start/end window, sample count, result summary, data-quality state,
and returned SigNoz deep link when available. Violations also retain all known
affected consumers. Evidence must omit tokens and unnecessary raw attribute or
log values.

## Timeout and stale-data isolation

Every network call receives context cancellation and an explicit timeout.
Polling has a deadline, bounded interval, readiness condition, and terminal
error. Each verification uses a unique run ID and a recorded start time;
queries are restricted to that window. Alert events must occur after the fault
injection timestamp. Data from prior runs cannot satisfy a current check.
Concrete durations and minimum sample counts are fixture/live findings owned by
Phases 1, 2, and 4, not Phase 0 guesses.

## Future worktree ownership

- Phase 1 environment worktree: `foundry/**`, `scripts/env/**`.
- Phase 1 demo worktree: `demo/**`, `fixtures/**`, `scripts/seed/**`,
  `scripts/load/**`.
- Phase 2: `internal/signoz/**` and initial CLI composition.
- Phase 3: `internal/contracts/**`, `internal/miner/**`, contract fixtures.
- Phase 4: `internal/verifier/**`, `internal/evidence/**`, verdict artifacts.
- Phase 5 CI worktree: `.github/**`, `internal/report/**`, `scripts/ci/**`.
- Phase 5 UI worktree: `web/**` and graph fixtures/tests.

The integration owner alone changes root `Makefile`, shared schemas, or shared
documentation during parallel work. No two agents edit one worktree.

## Critical assumptions

- **ASSUMPTION A1:** Go remains sufficient for the CLI/backend; revisit only if
  a required Phase 2 interface cannot be implemented safely.
- **ASSUMPTION A2:** the configured MCP identity remains read-only-capable for
  discovery; Phase 2 validates the exact permissions needed.
- **ASSUMPTION A3:** healthy and broken releases can share functional code and
  differ only at documented telemetry emission points; Phase 1 proves this.
- **ASSUMPTION A4:** returned SigNoz deep links, if available, are used verbatim;
  Phase 2 does not construct them from guessed URLs.

## Highest-risk demo failure modes

1. Ingestion lag or no load creates a false verdict.
2. Stale telemetry or alert history satisfies a new run.
3. Foundry reports ready before required components are usable or fails cleanup.
4. Healthy and broken variants diverge functionally.
5. Unsupported SigNoz query shapes are silently dropped.
6. Alert polling times out without a distinct inconclusive classification.
7. Secrets or raw private telemetry enter logs, fixtures, or screenshots.
8. CI loses verdict artifacts on nonzero exit.
9. The graph jitters or implies incident causality.

## Deferred decisions

| Decision | Assigned phase |
|---|---|
| Exact Foundry configuration, OTLP path, and readiness probes | Phase 1 |
| SigNoz operations, shapes, typed errors, and timeout values | Phase 2 |
| YAML library and exact fixture-proven extraction forms | Phase 3 |
| Minimum samples, polling intervals, and state aggregation | Phase 4 |
| Static report implementation and accessibility design direction | Phase 5 |
| CI environment strategy and artifact retention details | Phase 5 |

Nothing in this document authorizes a stretch feature or excluded capability.
