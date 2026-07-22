# Telemetry Guardian — Product Specification

**Document status:** Approved MVP specification  
**Product:** Telemetry Guardian  
**Hackathon track:** SigNoz — Signals & Dashboards  
**Primary implementation window:** 7 days  
**Primary audience:** Contributors using Claude Code, Codex, and human reviewers  
**Last updated:** 2026-07-21

---

## Document Authority

**Canonical repository path:** `docs/PRODUCT_SPEC.md`  
**Execution companion:** `docs/BUILD_PLAN.md`

This specification defines **what must be built, what must not be built, and how completion is judged**. The build plan defines execution order and agent ownership but may not override this document.

When documents conflict, use this precedence:

1. `docs/PRODUCT_SPEC.md`
2. Explicit human scope decision recorded in `docs/STATUS.md`
3. `docs/ARCHITECTURE.md`
4. `docs/BUILD_PLAN.md`
5. Agent assumptions

Changes to MVP scope require a reviewed edit to this file.

---

## 1. Product Summary

Telemetry Guardian is a consumer-driven reliability control plane for observability.

It discovers which telemetry fields, operations, and signals are required by existing SigNoz dashboards and alerts, converts those dependencies into executable telemetry contracts, and verifies candidate releases against those contracts before deployment.

The product proves that a release remains observable even when its functional behavior appears healthy.

### Product promise

> **Prove your system is observable before production proves it is not.**

### Category

**Observability Reliability Engineering:** engineering, testing, and governing the observability system itself.

---

## 2. Problem Statement

A software release can remain functionally correct while silently degrading observability.

Examples include:

- Renaming an attribute used by a dashboard.
- Removing a span operation required for an investigation.
- Breaking trace-to-log correlation.
- Changing an error field used by an alert.
- Emitting a field with the wrong type or unit.
- Causing a dashboard to appear healthy because data disappeared.
- Preventing a critical alert from firing during a real failure.

These defects are usually discovered only after:

- A dashboard becomes empty.
- An alert fails during an incident.
- An engineer notices missing telemetry.
- A production investigation becomes impossible.
- A platform team manually audits instrumentation.

Current CI systems test application behavior, but they rarely test whether the application remains diagnosable and operationally observable.

---

## 3. Core Product Insight

Every SigNoz dashboard panel and alert rule is already an implicit telemetry consumer.

Those consumers encode requirements such as:

- Required metric names.
- Required span operations.
- Required log fields.
- Required attributes.
- Required filters and grouping dimensions.
- Required error classifications.
- Required alert evaluation behavior.

Telemetry Guardian extracts those implicit dependencies, represents them as explicit contracts, and verifies emitted telemetry against them.

The MVP supports three contract sources:

1. **Consumer-derived contracts**
   - Extracted from SigNoz dashboards and alerts.

2. **Explicit engineering contracts**
   - Defined in a versioned YAML file.

3. **Runtime verification evidence**
   - Collected from a reproducible SigNoz environment.

---

## 4. Goals

### 4.1 Product goals

The MVP must:

1. Read one SigNoz dashboard.
2. Read one SigNoz alert rule.
3. Extract their telemetry dependencies.
4. Build a normalized telemetry contract.
5. Deploy a healthy and broken application release in a reproducible environment.
6. Execute contract checks against emitted telemetry.
7. Identify which consumers are affected by a violation.
8. Block a CI workflow for verified telemetry breakage.
9. Distinguish product violations from infrastructure or insufficient-data failures.
10. Prove that a broken telemetry contract can cause a real alert to miss a deterministic injected fault.
11. Provide evidence for every verdict.
12. Present impact through a consumer blast graph.

### 4.2 Hackathon goals

The submission must clearly demonstrate:

- Deep use of SigNoz.
- Practical OpenTelemetry engineering value.
- A complete end-to-end workflow.
- A memorable before-and-after demo.
- Reproducibility through Foundry.
- Strong technical execution without overclaiming.

### 4.3 Product-quality goals

The MVP must be:

- Deterministic enough for repeated demonstration.
- Reproducible on a fresh machine.
- Honest about insufficient evidence.
- Safe with credentials and telemetry data.
- Modular enough to extend after the hackathon.
- Small enough to complete within seven days.

---

## 5. Non-Goals

The following are explicitly excluded from the MVP:

- Root-cause analysis.
- Epicenter-style incident-origin detection.
- Change-point detection.
- Causal inference.
- Automatic remediation in production.
- Cardinality forecasting.
- Collector configuration pull-request generation.
- Full Dark Matter inference.
- Full Telemetry Bill of Materials signing.
- Historical alert optimization.
- SLO backtesting.
- Semantic-convention migration automation.
- OpAMP-based collector fleet management.
- GitHub App development.
- Modifying SigNoz core source code unless strictly unavoidable.
- Supporting every SigNoz query shape.
- Supporting every signal and every contract type.
- Building a generic observability policy engine.
- Building an AI chatbot.

Any work on these items requires an explicit product-spec update.

---

## 6. Target Users

### 6.1 Platform engineer

Needs to ensure that application teams emit reliable telemetry and do not silently break shared dashboards or alerts.

### 6.2 Site reliability engineer

Needs confidence that critical alerts and investigation workflows will work during incidents.

### 6.3 Application developer

Needs fast, actionable feedback when an instrumentation change breaks operational consumers.

### 6.4 Observability owner

Needs visibility into telemetry dependencies, coverage, and release-level observability regressions.

---

## 7. Primary User Story

> As a developer changing checkout-service instrumentation, I want CI to verify that existing SigNoz dashboards and alerts still work, so that I do not deploy a functionally correct release that becomes impossible to monitor or debug.

---

## 8. Secondary User Stories

### Contract discovery

> As a platform engineer, I want Guardian to discover telemetry requirements from existing dashboards and alerts so that I do not need to author every contract manually.

### Consumer impact

> As a developer, I want every telemetry violation to name the dashboards, panels, and alerts it affects so that I understand the operational consequence.

### Alert verification

> As an SRE, I want Guardian to inject a known failure and verify that a critical alert fires so that I know the alert is operational rather than merely configured.

### Evidence inspection

> As a reviewer, I want every failed check to link to the exact SigNoz query, time window, and sample count so that I can verify the conclusion independently.

---

## 9. Product Workflow

### 9.1 Contract-mining workflow

1. Guardian connects to SigNoz.
2. Guardian reads a configured dashboard.
3. Guardian reads a configured alert rule.
4. Guardian extracts referenced fields, operations, filters, and signal names.
5. Guardian normalizes duplicate requirements.
6. Guardian records all dependent consumers.
7. Guardian writes a versioned contract.

### 9.2 Release-verification workflow

1. CI provisions or connects to the Foundry test environment.
2. CI deploys the candidate application release.
3. CI runs a deterministic workload.
4. Guardian records the verification start time.
5. Guardian waits for telemetry ingestion.
6. Guardian executes contract checks.
7. Guardian injects the deterministic payment-timeout fault.
8. Guardian verifies whether the expected alert fires.
9. Guardian creates a verification result.
10. Guardian generates a consumer impact graph.
11. Guardian returns an exit code for CI.

### 9.3 Repair workflow

1. Developer reviews the failed requirement.
2. Developer reviews affected consumers.
3. Developer opens linked evidence.
4. Developer restores or migrates the telemetry field.
5. Verification runs again.
6. Contract checks pass.
7. The fault is reinjected.
8. The alert fires successfully.

---

## 10. Demo Scenario

### 10.1 Healthy release

The checkout service emits:

- `cart.value`
- A `payment.authorize` span operation
- `error.type=payment_timeout`
- Trace context in relevant error logs

The configured dashboard and alert depend on these fields.

Expected outcome:

- Functional tests pass.
- Guardian passes.
- The dashboard is populated.
- The injected payment timeout causes the alert to fire.

### 10.2 Broken release

The checkout service remains functionally equivalent but changes telemetry:

- `cart.value` becomes `cart.amount`
- `error.type=payment_timeout` becomes `error.kind=timeout`

Expected outcome:

- Functional tests still pass.
- Guardian fails.
- The blast graph identifies the broken dashboard panel and alert.
- The injected payment timeout does not trigger the existing alert.
- CI blocks the release.

### 10.3 Repaired release

The expected telemetry contract is restored.

Expected outcome:

- Guardian passes.
- The dashboard works.
- The payment-timeout alert fires again.

### 10.4 Demo narrative

> The application still works. Its functional tests are green. But one telemetry change silently breaks a dashboard and disables a critical alert. Telemetry Guardian discovers the consumers, tests the release against their requirements, names every affected resource, and blocks deployment before the next incident becomes undiagnosable.

---

## 11. MVP Scope

### 11.1 Required capabilities

The MVP must include:

- SigNoz dashboard retrieval.
- SigNoz alert retrieval.
- Dependency extraction.
- Contract serialization.
- Required-field verification.
- Required-operation verification.
- Alert-must-fire verification.
- Evidence generation.
- Consumer blast graph.
- CI exit codes.
- Reproducible healthy and broken demo releases.
- One-command or low-command demo workflow.
- Documentation and troubleshooting.

### 11.2 Supported contract checks

The MVP supports **three check types** and uses **four instantiated checks** in the canonical demo:

1. Required field: `cart.value`
2. Required field: `error.type`
3. Required operation: `payment.authorize`
4. Alert must fire: `payment-timeout`

The broken release is expected to fail checks 1, 2, and 4 while check 3 continues to pass.

#### Required field

Verifies that a required field appears in the expected signal within the verification window.

Example:

```yaml
type: required_field
signal: traces
field: cart.value
```

#### Required operation

Verifies that a required trace operation appears.

Example:

```yaml
type: required_operation
signal: traces
operation: payment.authorize
```

#### Alert must fire

Injects a deterministic fault and verifies that a configured alert enters firing state within a bounded time window.

Example:

```yaml
type: alert_must_fire
alert_id: payment-timeout
timeout: 60s
```

### 11.3 Optional checks

Only after the required MVP is complete:

- Trace-to-log correlation ratio.
- Basic cardinality budget.
- Peer-service ghost-node inference.

---

## 12. Contract Format

The canonical contract format is YAML.

Example:

```yaml
apiVersion: telemetry.guardian/v1
service: checkout
release: candidate

consumers:
  - id: checkout-revenue-panel
    type: dashboard_panel
    name: Revenue by region
    owner: checkout-team
    criticality: required
    source:
      dashboard_id: checkout-overview
      panel_id: revenue-region
    requires:
      - id: cart-value
        type: required_field
        signal: traces
        field: cart.value

  - id: payment-timeout-alert
    type: alert
    name: Payment timeout
    owner: sre
    criticality: required
    source:
      alert_id: payment-timeout
    requires:
      - id: payment-error-type
        type: required_field
        signal: traces
        field: error.type
      - id: payment-timeout-detection
        type: alert_must_fire
        alert_id: payment-timeout
        timeout: 60s

checks:
  - id: payment-operation
    type: required_operation
    signal: traces
    operation: payment.authorize
```

### Contract requirements

- Every consumer must have a stable ID.
- Every requirement must have a stable ID.
- Every derived requirement must preserve its source path.
- Unsupported expressions must produce explicit warnings or errors.
- No requirement may be silently dropped.
- Duplicate requirements may be normalized only if all consumers remain attached.

---

## 13. Core Domain Model

### Consumer

Represents a SigNoz resource or sub-resource that depends on telemetry.

Fields:

- ID
- Name
- Type
- Owner
- Criticality
- Source reference
- Requirements

Supported MVP consumer types:

- `dashboard_panel`
- `alert`

### Requirement

Represents a condition that emitted telemetry must satisfy.

Fields:

- ID
- Type
- Signal
- Field
- Operation
- Alert ID
- Source path
- Parameters

### Violation

Represents a verified contract failure.

Fields:

- Code
- Message
- Requirement
- Severity
- Affected consumers
- Evidence
- Remediation hint

### Evidence

Represents independently inspectable support for a verdict.

Fields:

- Query type
- Query payload
- Time window
- Sample count
- Result summary
- SigNoz deep link when available
- Data-quality state

### Verification result

Fields:

- Run ID
- Service
- Release
- Start time
- End time
- Overall state
- Check results
- Violations
- Evidence
- Affected consumers

Supported states:

- `PASS`
- `FAIL`
- `INCONCLUSIVE`

---

## 14. Result Semantics

### PASS

The contract check ran successfully and sufficient data proves the requirement is satisfied.

### FAIL

The contract check ran successfully and sufficient data proves the requirement is violated.

### INCONCLUSIVE

The system cannot determine pass or fail because of:

- Insufficient telemetry.
- Query error.
- Authentication failure.
- Timeout.
- Missing environment readiness.
- Unsupported query form.
- Missing or stale alert history.

**INCONCLUSIVE must never be converted into PASS.**

---

## 15. CI Exit Codes

| Exit code | Meaning |
|---:|---|
| `0` | All required checks passed |
| `1` | One or more verified contract violations |
| `2` | Verification was inconclusive or infrastructure failed |
| `3` | Invalid configuration or contract |

CI must:

- Pass only for exit code `0`.
- Fail for exit code `1` with classification `TELEMETRY_CONTRACT_VIOLATION`.
- Fail for exit code `2` with classification `VERIFICATION_INCONCLUSIVE` unless a future Checks API integration can represent a neutral state.
- Clearly distinguish product violations from infrastructure or evidence failures.
- Never present exit code `2` as a healthy release.
- Upload the complete verification result as an artifact.
- Not require a GitHub App for the MVP.

---

## 16. SigNoz Integration

### 16.1 Required SigNoz capabilities

Telemetry Guardian must use:

- OpenTelemetry ingestion.
- Traces.
- Metrics or trace-derived aggregations.
- Logs where supported.
- Query Builder or equivalent query API.
- Dashboards.
- Alerts.
- Alert history.
- SigNoz APIs or MCP.
- Foundry for reproducibility.

### 16.2 SigNoz client boundary

All SigNoz access must go through a typed client interface.

Required operations:

- Retrieve dashboard.
- Retrieve alert.
- Execute Builder query.
- Search traces.
- Search logs.
- Retrieve alert history.
- Produce deep links when possible.

### 16.3 Client requirements

- Explicit timeouts.
- Context cancellation.
- Typed errors.
- Secret redaction.
- Fixture-backed fake client.
- No direct SigNoz response types in the core domain model.
- No API token logging.

---

## 17. Foundry Environment

The repository must include:

```text
foundry/casting.yaml
foundry/casting.yaml.lock
```

The environment must provision:

- SigNoz.
- Required OpenTelemetry components.
- Healthy checkout release.
- Broken checkout release.
- Deterministic workload generator.
- Fault injector.
- Seed dashboard.
- Seed alert.

### Environment requirements

- Startup must wait for real readiness.
- Teardown must remove all project resources.
- Healthy and broken releases must be functionally equivalent.
- Telemetry differences must be intentional and documented.
- The fault injector must produce repeatable outcomes.
- The scenario must work at least three consecutive times before release.

---

## 18. Consumer Blast Graph

### Purpose

Show the operational consequences of a telemetry violation.

### Required nodes

- Failed requirement.
- Dashboard.
- Dashboard panel.
- Alert.

### Required edges

- `REQUIRED_BY`
- `PART_OF`
- `BREAKS`

### UX requirements

- Failed requirement is visually central.
- Affected consumers are immediately visible.
- Layout is deterministic for screen recording.
- Selecting a consumer opens evidence.
- Healthy state is visually simple.
- The graph must not imply causality beyond the known dependency relationship.

Example:

```text
cart.value
  ├── breaks → Checkout Revenue / Revenue by region
  └── breaks → Payment timeout alert
```

---

## 19. CLI

Recommended commands:

```bash
guardian mine
guardian verify
guardian report
guardian demo
```

### `guardian mine`

Reads configured SigNoz resources and produces a contract.

### `guardian verify`

Runs checks against a release and produces a verification result.

### `guardian report`

Renders:

- Terminal summary.
- JSON.
- GitHub Markdown summary.

### `guardian demo`

Runs or guides the complete healthy-to-broken-to-repaired scenario.

### CLI requirements

- Machine-readable JSON output.
- Stable exit codes.
- No secrets in output.
- Clear stage markers.
- Helpful validation errors.
- Non-interactive smoke-test mode.

---

## 20. API

The MVP may expose a read-only local HTTP API.

Recommended endpoints:

```text
GET /api/runs
GET /api/runs/{id}
GET /api/runs/{id}/graph
```

No mutation API is required for the MVP.

### API requirements

- Stable JSON schema.
- Input validation.
- No secret exposure.
- Local-development CORS configuration.
- Fixture-backed tests.

---

## 21. Repository Structure

Recommended structure:

```text
telemetry-guardian/
├── cmd/
│   └── guardian/
├── internal/
│   ├── contracts/
│   ├── miner/
│   ├── signoz/
│   ├── verifier/
│   ├── impact/
│   ├── evidence/
│   └── report/
├── web/
├── demo/
│   ├── healthy/
│   ├── broken/
│   └── fault-injector/
├── foundry/
│   ├── casting.yaml
│   └── casting.yaml.lock
├── fixtures/
│   ├── dashboards/
│   ├── alerts/
│   └── telemetry/
├── .github/
│   └── workflows/
├── docs/
├── AGENTS.md
├── CLAUDE.md
└── README.md
```

---

## 22. Functional Requirements

### FR-001: Dashboard retrieval

Guardian must retrieve one configured SigNoz dashboard.

### FR-002: Alert retrieval

Guardian must retrieve one configured SigNoz alert.

### FR-003: Dashboard dependency extraction

Guardian must extract supported field and signal dependencies from at least one dashboard panel.

### FR-004: Alert dependency extraction

Guardian must extract supported field and signal dependencies from one alert.

### FR-005: Unsupported-query handling

Guardian must explicitly report unsupported query constructs.

### FR-006: Contract generation

Guardian must produce a stable YAML contract.

### FR-007: Required-field verification

Guardian must verify whether a required field appears in the configured verification window.

### FR-008: Required-operation verification

Guardian must verify whether a required trace operation appears.

### FR-009: Alert-must-fire verification

Guardian must inject the deterministic fault and verify alert state within a bounded window.

### FR-010: Evidence preservation

Every check must preserve query, time window, sample count, and result summary.

### FR-011: Consumer impact resolution

Every violation must list all known affected consumers.

### FR-012: Consumer blast graph

Guardian must produce a graph representation of violations and affected consumers.

### FR-013: CI blocking

Guardian must return exit code `1` for verified contract violations.

### FR-014: Inconclusive handling

Guardian must return exit code `2` for insufficient evidence or infrastructure failure.

### FR-015: Reproducible demo

A fresh evaluator must be able to run the documented demo.

---

## 23. Non-Functional Requirements

### NFR-001: Determinism

The healthy and broken scenarios must produce the same expected verdict across three consecutive runs.

### NFR-002: Bounded operations

All network calls, polling loops, and verification stages must have explicit timeouts.

### NFR-003: Security

API keys, tokens, and secrets must never appear in logs, reports, fixtures, or screenshots.

### NFR-004: Evidence integrity

Verification results must include run IDs and explicit time windows to prevent stale telemetry from satisfying a check.

### NFR-005: Testability

The core verifier and miner must be testable without a live SigNoz instance.

### NFR-006: Maintainability

Core domain packages must not depend directly on SigNoz JSON response types.

### NFR-007: Honest output

Guardian must not claim root cause, causal attribution, or certainty beyond available evidence.

### NFR-008: Performance

MVP verification should complete within a practical CI window for the demo environment.

Target:

- Contract mining: under 10 seconds after API availability.
- Verification: under 3 minutes excluding environment provisioning.
- Alert must-fire check: bounded by configured timeout.

---

## 24. Security and Privacy

### Required controls

- Read credentials from environment variables or secret stores.
- Redact tokens from errors.
- Avoid storing raw sensitive attribute values when only field presence is needed.
- Use sanitized fixtures.
- Do not expose full log bodies in CI summaries.
- Restrict evidence links to the configured SigNoz environment.
- Ensure demo data contains no real customer information.

### Out of scope

- Enterprise role-based access control.
- Multi-tenant authorization.
- Evidence encryption.
- Signed manifests.
- Long-term evidence retention.

---

## 25. Testing Strategy

### Unit tests

Required for:

- Contract parsing.
- Dependency extraction.
- Requirement deduplication.
- Evidence generation.
- Result-state logic.
- Exit-code mapping.
- Report rendering.

### Golden-file tests

Required for:

- Dashboard-to-contract extraction.
- Alert-to-contract extraction.
- Verification JSON.
- GitHub Markdown output.
- Blast graph JSON.

### Integration tests

Required for:

- SigNoz client adapter.
- Healthy verification.
- Broken required-field verification.
- Alert must-fire success.
- Alert must-fire failure.
- Stale-data isolation.

### End-to-end tests

Required sequence:

1. Healthy release passes.
2. Broken release remains functionally correct.
3. Broken release fails Guardian.
4. Broken alert misses the injected fault.
5. Repaired release passes.
6. Repaired alert fires.

### Adversarial cases

Tests must cover:

- No telemetry.
- Stale telemetry from a previous run.
- Authentication failure.
- Query timeout.
- Unsupported query node.
- Missing dashboard panel title.
- Duplicate consumers.
- Malformed SigNoz response.
- Alert firing from an earlier run.
- Environment not ready.

---

## 26. Acceptance Criteria

The MVP is complete only when all criteria pass.

### Contract mining

- [ ] One dashboard is read successfully.
- [ ] One alert is read successfully.
- [ ] Their dependencies are extracted.
- [ ] Unsupported constructs are explicit.
- [ ] A stable YAML contract is produced.

### Verification

- [ ] Healthy release passes the `cart.value` required-field check.
- [ ] Healthy release passes the `error.type` required-field check.
- [ ] Healthy release passes the `payment.authorize` required-operation check.
- [ ] Healthy release passes the `payment-timeout` alert-must-fire check.
- [ ] Broken release fails the `cart.value` required-field check.
- [ ] Broken release fails the `error.type` required-field check.
- [ ] Broken release still passes the `payment.authorize` required-operation check.
- [ ] Broken release fails the `payment-timeout` alert-must-fire check.
- [ ] Broken release therefore produces exactly three expected failures.
- [ ] Broken release remains functionally correct.
- [ ] No-data produces `INCONCLUSIVE`.

### Evidence and impact

- [ ] Every result contains a query and time window.
- [ ] Every violation lists affected consumers.
- [ ] Blast graph renders deterministically.
- [ ] Consumer evidence can be inspected.

### CI

- [ ] Healthy workflow is green.
- [ ] Broken workflow is blocked.
- [ ] Inconclusive state is distinct.
- [ ] Verification artifact is uploaded.

### Reproducibility

- [ ] Environment starts from documented configuration.
- [ ] Demo succeeds three consecutive times.
- [ ] Fresh evaluator can follow README instructions.
- [ ] Teardown removes resources.

### Presentation

- [ ] Functional tests visibly pass for the broken release.
- [ ] Guardian visibly blocks the release.
- [ ] Blast graph names affected consumers.
- [ ] Live alert miss is demonstrated.
- [ ] Repair and successful alert are demonstrated.

---

## 27. Seven-Day Delivery Plan

### Day 1 — Architecture and domain model

Deliver:

- Repository scaffold.
- Core domain types.
- Canonical contract model.
- Fixture strategy.
- SigNoz client interface.

Exit condition:

- Domain tests pass.
- Scope is frozen.

### Day 2 — SigNoz adapter and contract miner

Deliver:

- Dashboard retrieval.
- Alert retrieval.
- Dependency extraction.
- Golden contract fixtures.

Exit condition:

- One command generates the expected contract.

### Day 3 — Foundry and demo environment

Deliver:

- Foundry configuration.
- Healthy service.
- Broken service.
- Workload generator.
- Fault injector.
- Seed dashboard and alert.

Exit condition:

- Healthy alert fires.
- Broken alert misses.
- Functional outputs remain equivalent.

### Day 4 — Verification and evidence

Deliver:

- Required-field check.
- Required-operation check.
- Alert-must-fire check.
- Result-state semantics.
- Evidence model.

Exit condition:

- Healthy passes.
- Broken fails.
- No-data is inconclusive.

### Day 5 — Blast graph and CI

Deliver:

- Consumer impact graph.
- GitHub Actions workflow.
- Markdown summary.
- End-to-end command.

Exit condition:

- Full healthy-to-broken flow works.

### Day 6 — Hardening and demo rehearsal

Deliver:

- Adversarial tests.
- Documentation fixes.
- Stable screen layout.
- Feature freeze.

Exit condition:

- Three consecutive successful demo runs.
- No submission-blocking defects.

### Day 7 — Submission

Deliver:

- Final README.
- Architecture document.
- Demo video.
- Screenshots.
- Known limitations.
- Judging-criteria mapping.

No new core features are permitted.

---

## 28. Stretch Goals

Stretch work may begin only after all acceptance criteria pass.

### Stretch 1: Trace-to-log correlation

Verify that a configurable percentage of relevant error logs contain trace context.

### Stretch 2: Basic cardinality budget

Detect a known forbidden metric dimension in the candidate release.

No forecasting is required.

### Stretch 3: Ghost topology

Display a peer service observed in client spans but not observed as an emitting service.

The result must be labeled as a probable instrumentation gap, not a confirmed missing service.

### Stretch 4: Propagation View

Animate deterministic dependency impact:

```text
Telemetry field removed
        ↓
Dashboard panel breaks
        ↓
Alert expression breaks
        ↓
Injected fault becomes undetected
```

This is dependency propagation, not incident root-cause analysis.

---

## 29. Risks and Mitigations

### Risk: SigNoz query schemas are more complex than expected

Mitigation:

- Support only the exact fixture query shapes.
- Fail explicitly for unsupported forms.
- Preserve raw source paths.
- Do not attempt universal parsing.

### Risk: Telemetry ingestion delay causes flaky checks

Mitigation:

- Use bounded polling.
- Require minimum sample counts.
- Use unique verification IDs.
- Query only the active run window.

### Risk: Alert history contains stale events

Mitigation:

- Tag injected faults with unique run IDs where possible.
- Restrict alert history by verification time window.
- Reject events preceding the fault.

### Risk: Foundry provisioning consumes too much demo time

Mitigation:

- Support a pre-provisioned demo mode.
- Keep Foundry as the reproducibility path.
- Use readiness checks and cached images.

### Risk: Guardian appears to be only a linter

Mitigation:

- Lead with consumer impact.
- Demonstrate a real alert miss.
- Show the blast graph.
- Present the product as reliability testing for observability.

### Risk: Feature expansion prevents completion

Mitigation:

- No new core features after Day 5.
- Non-goals are binding.
- Stretch work requires three successful end-to-end runs.

---

## 30. Judging-Criteria Alignment

### Potential Impact

Prevents silent dashboard and alert breakage before production.

### Creativity and Innovation

Introduces consumer-driven contracts for telemetry and release-level observability verification.

### Technical Excellence

Combines OpenTelemetry, SigNoz, Query Builder, alerts, dashboards, Foundry, CI, deterministic fault injection, and evidence-driven verification.

### Best Use of SigNoz

Uses SigNoz as:

- Telemetry backend.
- Consumer registry.
- Query engine.
- Dashboard system.
- Alert engine.
- Alert-history source.
- Verification evidence surface.

### User Experience

Transforms an abstract telemetry break into named affected consumers and a reviewable CI verdict.

### Presentation Quality

Provides a clear narrative:

```text
Healthy
  ↓
Functionally healthy but observability broken
  ↓
Consumers identified
  ↓
Release blocked
  ↓
Real alert miss demonstrated
  ↓
Repair
  ↓
Alert restored
```

---

## 31. Positioning

### Primary message

> **Telemetry Guardian is the reliability system for your observability system.**

### Secondary message

> Functional correctness is not enough. A release must remain diagnosable.

### Do not position as

- A linter.
- A chatbot.
- An RCA engine.
- A dashboard generator.
- A generic policy engine.
- An alerting replacement.

---

## 32. Definition of Done

Telemetry Guardian is done when a fresh evaluator can:

1. Launch the documented environment.
2. Observe the healthy release passing.
3. Observe the healthy alert firing.
4. Deploy the functionally correct broken release.
5. See Guardian detect the broken telemetry contract.
6. See the affected dashboard panel and alert in the blast graph.
7. See CI block the release.
8. Inject the same fault.
9. Observe the broken alert miss it.
10. Restore the contract.
11. Observe Guardian pass.
12. Observe the alert fire again.

Anything beyond this sequence is optional.

---

## 33. Agent Instructions

Every implementation agent must:

1. Read this file before editing.
2. Implement only the active phase.
3. List acceptance tests before writing code.
4. Avoid excluded features.
5. Add or update tests.
6. Run all relevant checks.
7. Report unresolved assumptions.
8. Never claim success when tests fail.
9. Treat `INCONCLUSIVE` as distinct from `PASS`.
10. Update project status after completing work.

The current phase and ownership should be recorded in:

```text
docs/STATUS.md
```

---

## 34. Final Product Decision

Telemetry Guardian is the selected submission because it offers:

- The highest probability of a complete result.
- Strong use of the full SigNoz surface.
- A clear and practical engineering problem.
- Deterministic evidence.
- Multiple independent success paths.
- A defensible product roadmap.
- A memorable demo when paired with the consumer blast graph and live alert test.

Epicenter-style incident-origin analysis is intentionally excluded from the core project.

The only Epicenter-inspired stretch is a deterministic **Propagation View** showing how a telemetry change breaks known consumers.

---

# End of Specification
