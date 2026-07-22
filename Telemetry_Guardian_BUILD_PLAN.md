# Telemetry Guardian — Final Agent Build Plan

**Document status:** Approved execution plan  
**Canonical repository path:** `docs/BUILD_PLAN.md`  
**Product authority:** `docs/PRODUCT_SPEC.md`  
**Implementation model:** Codex implements; Claude Code reviews and architects; human approves merges  
**Delivery window:** 7 days  
**Last updated:** 2026-07-21

---

## 1. Purpose and authority

This document defines **how Telemetry Guardian will be built**. It does not redefine product scope.

Before every task, agents must read:

1. `docs/PRODUCT_SPEC.md`
2. `docs/STATUS.md`
3. `AGENTS.md` or `CLAUDE.md`
4. The active phase in this document

When documents conflict, follow the authority order in `docs/PRODUCT_SPEC.md`.

### Protected MVP

The protected end-to-end path is:

> Mine dashboard and alert consumers → produce a telemetry contract → verify a healthy and broken release in Foundry → identify affected consumers → block CI → demonstrate a real alert miss → repair → demonstrate the alert firing.

The canonical demo contains **three check types and four check instances**:

| Instance | Type | Healthy | Broken |
|---|---|---:|---:|
| `cart.value` exists | Required field | PASS | FAIL |
| `error.type` exists | Required field | PASS | FAIL |
| `payment.authorize` exists | Required operation | PASS | PASS |
| `payment-timeout` alert fires | Alert must fire | PASS | FAIL |

The broken release therefore produces exactly **three expected failures** while remaining functionally correct.

---

## 2. Agent operating model

| Role | Tool | Responsibility |
|---|---|---|
| Implementer | **Codex** | Feature code, tests, fixtures, CI, Foundry environment, demo scripts and documentation changes requested by review |
| Reviewer and architect | **Claude Code** | Plan-mode architecture, adversarial reviews, integration diagnosis, scope policing and demo critique |
| Final authority | **Human** | Scope decisions, merge approval, go/no-go checkpoints, release approval and submission |

### Reviewer independence

Claude Code should normally produce a review artifact rather than edit the implementation directly:

```text
docs/reviews/phase<N>.md
```

Codex implements review fixes. Claude may edit code only for an explicitly recorded integration emergency; a human or Codex must then independently review that edit.

### Worktree rules

1. No two agents edit the same worktree.
2. Parallel worktrees must have exclusive directory ownership.
3. Root integration files such as `Makefile`, `README.md`, and shared lockfiles have one designated owner per phase.
4. Nothing merges before its acceptance script passes and Claude records a review verdict.
5. Codex updates `docs/STATUS.md` after every completed task.

### Standard repository documents

```text
docs/
├── PRODUCT_SPEC.md
├── BUILD_PLAN.md
├── ARCHITECTURE.md
├── STATUS.md
├── ROADMAP.md
└── reviews/
```

---

## 3. Non-negotiable engineering invariants

1. `INCONCLUSIVE` is distinct from `PASS` and `FAIL`.
2. No-data is never success.
3. Every verdict links to evidence: query, time window, sample count and result summary.
4. The broken release differs from the healthy release only in telemetry behavior.
5. Functional tests pass for both variants.
6. The injected fault is deterministic.
7. Verification uses a unique run ID and excludes stale telemetry.
8. Exit codes remain `0` pass, `1` verified violation, `2` inconclusive/infrastructure, `3` invalid configuration.
9. GitHub Actions passes only on exit code `0`; exits `1` and `2` both fail but carry different classifications.
10. No GitHub App is required for the MVP.
11. No SigNoz source modification unless a human records that public APIs/MCP are insufficient.
12. Stretch work starts only after three consecutive successful end-to-end demo runs.
13. Core feature development stops at the Day-5 demo freeze.

---

# Phase 0 — Specification freeze and architecture

**Schedule:** Day 1 morning  
**Goal:** Establish the repository, authority documents, interfaces and unknowns before feature implementation.

## Codex implementation

Create:

- `docs/PRODUCT_SPEC.md`
- `docs/BUILD_PLAN.md`
- `docs/STATUS.md`
- `AGENTS.md`
- `CLAUDE.md`
- `Makefile` skeleton
- `scripts/accept/` harness
- Repository scaffold from the product specification

`AGENTS.md` and `CLAUDE.md` should remain thin and point to the canonical documents.

## Claude plan-mode review

```text
Read docs/PRODUCT_SPEC.md, docs/BUILD_PLAN.md and the repository.
Do not write code.

Produce docs/ARCHITECTURE.md covering:
1. Package boundaries
2. Domain-model review
3. SigNoz interfaces that must be empirically probed
4. Fixture-fake versus live-integration test split
5. Critical unknowns and their owners
6. Demo failure risks
7. Security and stale-state risks
8. Confirmation that no non-goal is implied by the architecture

Label every assumption.
```

## Acceptance script

```text
scripts/accept/phase0.sh
```

It verifies that canonical documents exist, paths are consistent, repository checks run, and every critical unknown has an owner in `docs/STATUS.md`.

## Exit criteria

- Architecture document is committed.
- Scope is frozen.
- No active reference points to `PROJECT_SPEC.md`; the canonical name is `PRODUCT_SPEC.md`.
- All unresolved external dependencies are listed and assigned.

---

# Phase 1 — Foundry environment and deterministic demo application

**Schedule:** Day 1  
**Goal:** De-risk the highest-unknown dependency before building abstractions.

## Worktree ownership

| Worktree | Exclusive ownership |
|---|---|
| A — environment | `foundry/**`, `scripts/env/**` |
| B — demo application | `demo/**`, `fixtures/**`, `scripts/seed/**`, `scripts/load/**` |
| Integration owner | Root `Makefile` and shared environment documentation after A and B merge |

## Codex worktree A

Implement:

- `foundry/casting.yaml`
- `foundry/casting.yaml.lock`
- One-command environment up/down
- Health and readiness polling based on real endpoints, not fixed sleeps
- Complete cleanup

## Codex worktree B

Implement:

- `demo/checkout` service
- `RELEASE_VARIANT=healthy|broken`
- Shared functional test suite proving response equivalence
- Deterministic workload generator
- Deterministic `payment-timeout` fault injection
- Dashboard seed script
- Alert seed script
- Sanitized dashboard and alert fixtures

### Canonical telemetry

Healthy variant emits:

- Span attribute `cart.value`
- Span attribute `error.type=payment_timeout` for the injected fault
- Span operation `payment.authorize`
- Relevant trace context in error logs
- Standard duration telemetry needed by the seeded dashboard

Broken variant:

- Emits `cart.amount` instead of `cart.value`
- Emits `error.kind=timeout` instead of `error.type=payment_timeout`
- Continues emitting `payment.authorize`
- Produces the same functional HTTP behavior

## Claude review

```text
Review Phase 1 for determinism and functional equivalence.
Run environment seed and load three times.
Verify:
- readiness uses polling, not arbitrary sleeps
- teardown is complete
- secrets are not committed or logged
- healthy and broken functional tests are identical and green
- the variants differ only in the documented telemetry
- the healthy alert fires on the deterministic fault
- the broken alert misses the same fault

Write docs/reviews/phase1.md with blocking and non-blocking findings.
Do not edit implementation code.
```

## Acceptance script

```text
scripts/accept/phase1.sh
```

Required sequence:

> clean checkout → Foundry up → seed resources → generate load → confirm traces/logs/metrics through API → confirm dashboard and alert exist → inject healthy fault and observe alert → deploy broken variant and confirm functional tests → inject fault and observe alert miss → clean teardown

## Go/no-go checkpoint 1

Continue only when Foundry, SigNoz ingestion, resource seeding and both alert outcomes are repeatable.

---

# Phase 2 — Empirically verified SigNoz adapter

**Schedule:** Day 2 morning  
**Goal:** Build one typed boundary for all SigNoz access based on observed API behavior.

## Codex implementation

Before implementing the client:

1. Probe the running SigNoz instance.
2. Record endpoint paths, authentication, request shapes and response examples in:

```text
internal/signoz/API.md
```

Then implement `SigNozClient` with:

- Dashboard retrieval
- Alert retrieval
- Builder-query execution
- Trace search
- Log search where needed
- Alert-history retrieval
- Explicit timeouts and cancellation
- Typed unauthorized, not-found, invalid-response and timeout errors
- Secret redaction
- Raw resource IDs for evidence links
- Fixture-backed fake client

Unit tests must not require a live instance.

## Claude review

```text
Review the SigNoz adapter as a reliability engineer.
Hunt:
- retries hiding permanent errors
- missing timeouts or cancellation leaks
- secret exposure
- fake/live behavioral divergence
- response fields assumed without evidence
- untested malformed and authorization paths

Record findings in docs/reviews/phase2.md.
Codex, not Claude, implements corrections.
```

## Acceptance script

```text
scripts/accept/phase2.sh
```

It requires:

- Offline unit suite green
- Live integration suite green
- `internal/signoz/API.md` matching implemented behavior
- Authentication and timeout errors covered

---

# Phase 3 — Consumer-driven contract miner

**Schedule:** Day 2 afternoon  
**Goal:** Convert the seeded dashboard and alert into a stable telemetry contract.

## Codex implementation

Mine:

- Dashboard panel dependency on `cart.value`
- Alert dependency on `error.type`
- Required operation `payment.authorize`
- Alert must-fire requirement for `payment-timeout`

Generate:

```text
contracts/telemetry.guardian.yaml
```

Rules:

- Support only fixture-proven query shapes.
- Unsupported constructs produce explicit warnings or errors.
- Preserve source JSON paths.
- Deduplicate requirements while retaining every consumer.
- Never silently drop an expression.
- Add golden-file tests.

## Claude adversarial review

```text
Attack the contract miner using fixture mutations:
- renamed field
- removed filter
- duplicate consumers
- nested formula
- missing panel title
- unsupported query node
- changed field type
- malformed source path

The miner must fail loudly rather than emit an incomplete contract.
Write docs/reviews/phase3.md with reproduction steps and required tests.
```

## Acceptance script

```text
scripts/accept/phase3.sh
```

It verifies that `guardian mine` produces the four canonical check instances with correct consumer mappings and that all golden and mutation tests pass.

---

# Phase 4 — Verification engine and evidence semantics

**Schedule:** Day 3  
**Goal:** Produce deterministic `PASS`, `FAIL`, or `INCONCLUSIVE` results for the four canonical checks.

## Codex implementation

Implement:

1. Required field: `cart.value`
2. Required field: `error.type`
3. Required operation: `payment.authorize`
4. Alert must fire: `payment-timeout`

Every check returns:

- State
- Query or retrieval operation
- Explicit verification window
- Unique run ID
- Sample count
- Result summary
- Evidence link when available

Add:

- Bounded completeness polling
- Minimum expected event count
- Stale-state isolation
- Alert-event filtering after fault injection time
- `verdict.json`
- Exit-code aggregator

### Expected canonical results

Healthy:

```text
PASS × 4
```

Broken:

```text
FAIL cart.value
FAIL error.type
PASS payment.authorize
FAIL payment-timeout alert
```

No load or insufficient evidence:

```text
INCONCLUSIVE, never PASS
```

## Claude review

```text
Hunt false passes and false failures:
- ingestion races
- no-data-as-healthy
- stale telemetry from prior runs
- wrong time windows
- old alert events satisfying a new run
- retry behavior masking permanent errors
- cancellation leaks
- missing sample counts
- INCONCLUSIVE paths that are unreachable

Write docs/reviews/phase4.md with severity, reproduction and smallest safe correction.
```

## Acceptance script

```text
scripts/accept/phase4.sh
```

It must prove the exact healthy, broken and no-data outcomes above.

## Go/no-go checkpoint 2

Do not build UI or CI until the verifier produces stable results across repeated runs.

---

# Phase 5 — CI gate and consumer blast graph

**Schedule:** Day 4  
**Goal:** Make the violation actionable and block the broken release.

## Worktree ownership

| Worktree | Exclusive ownership |
|---|---|
| A — CI/report text | `.github/**`, `internal/report/**`, `scripts/ci/**` |
| B — blast graph | `web/**`, graph fixtures and component tests |
| Integration owner | Root build targets and final report schema |

## Codex worktree A — CI

Implement `.github/workflows/guardian.yml`:

- Provision or connect to the test environment
- Deploy candidate variant
- Run functional tests
- Mine contract
- Verify telemetry
- Upload `verdict.json` and report artifacts
- Publish a Markdown job summary

### Exit behavior

- `0`: workflow passes
- `1`: workflow fails and summary begins `TELEMETRY_CONTRACT_VIOLATION`
- `2`: workflow fails and summary begins `VERIFICATION_INCONCLUSIVE`
- `3`: workflow fails and summary begins `INVALID_GUARDIAN_CONFIGURATION`

A true neutral GitHub check is out of scope because the MVP does not build a GitHub App or Checks API integration.

## Codex worktree B — blast graph

Implement a deterministic, self-contained HTML report:

- Failed requirement centered
- Affected dashboard and panel nodes
- Affected alert node
- `BREAKS`, `REQUIRED_BY`, and `PART_OF` edges
- Evidence drawer
- Calm healthy state
- Stable 1280×720 recording layout
- No force-layout jitter

## Claude review

```text
Review CI for false green and false red outcomes.
Confirm exit 2 cannot appear healthy and that all artifacts survive failure.
Check fresh run IDs, secret redaction and evidence links.

Review the graph cold at 1280x720:
- central violation is obvious
- consumers are understandable without narration
- layout is stable between runs
- graph does not imply incident causality

Write docs/reviews/phase5.md.
```

## Acceptance script

```text
scripts/accept/phase5.sh
```

It must demonstrate:

- Healthy branch/workflow green
- Broken branch/workflow red while functional tests remain green
- Exactly three expected Guardian failures in the report
- Blast graph opens offline and renders deterministically

---

# Phase 6 — End-to-end demo command and feature freeze

**Schedule:** Day 5  
**Goal:** Protect the complete judging narrative as an executable invariant.

## Codex implementation

Create:

```text
scripts/demo.sh
make demo
make demo-smoke
```

Canonical sequence:

1. Start or verify environment readiness.
2. Deploy healthy release.
3. Run functional tests: pass.
4. Mine and verify: Guardian `PASS × 4`.
5. Inject payment timeout: alert fires.
6. Deploy broken release.
7. Run functional tests: pass.
8. Mine and verify: exactly three Guardian failures.
9. Open or produce consumer blast graph.
10. Inject the same timeout: alert misses.
11. Deploy repaired release.
12. Verify: Guardian `PASS × 4`.
13. Inject timeout: alert fires.
14. Preserve artifacts and print a final success summary.

Requirements:

- Clear stage markers
- Bounded waits
- Stops on unexpected state
- Idempotent or restartable
- Non-interactive smoke mode
- Preserved logs and verdicts

## Claude cold-evaluator review

```text
Act as a hackathon judge unfamiliar with the repository.
Follow only README instructions and run the demo.
Record every ambiguous step, undocumented prerequisite, flaky wait and state mismatch.
Recommend only documentation and determinism fixes—no new product features.
Write docs/reviews/phase6.md.
```

## Acceptance script

```text
scripts/accept/phase6.sh
```

It invokes the smoke path and verifies the final artifacts and expected state transitions.

## Demo freeze

After three consecutive successful runs:

```text
git tag demo-freeze
```

No new core features after this tag. Deferred ideas go to `docs/ROADMAP.md`.

---

# Phase 7 — Hardening and optional stretch gate

**Schedule:** Day 6  
**Goal:** Remove submission risk rather than add capability.

## Claude release-candidate review

```text
Review the repository against docs/PRODUCT_SPEC.md.
For every functional requirement FR-001 through FR-015 and every acceptance criterion, report:
- implemented
- partial
- absent
- file and test evidence

Then identify code not required by the spec and recommend deletion where it adds risk.
Attack:
- secret handling
- query-window isolation
- stale state
- malformed resources
- concurrency
- timeouts
- cleanup
- README accuracy
- demo determinism

Rank findings P0 / P1 / P2 in docs/reviews/release-candidate.md.
Do not edit implementation code.
```

## Codex implementation

- Fix every P0.
- Fix cheap, low-risk P1 findings.
- Do not redesign stable modules.
- Run:

```text
make fmt-check
make lint
make test
make integration-test
make demo-smoke
```

## Stretch gate

Stretch work is permitted only when:

- No P0 remains.
- All required checks are green.
- `scripts/demo.sh` passed three consecutive times.
- The human explicitly approves.

Stretch order:

1. **Ghost topology:** one `peer.service` without an observed emitting service, clearly labeled probable rather than confirmed.
2. **Propagation View:** deterministic telemetry-change-to-consumer impact animation.

Do not implement change-point detection, causal inference or incident-origin claims.

---

# Phase 8 — Submission and demonstration

**Schedule:** Day 7  
**Goal:** Ship a reproducible project and a concise, undeniable demonstration.

## Codex deliverables

- Final README
- Validated quickstart
- Architecture overview
- Checks reference
- Troubleshooting
- Known limitations
- Roadmap
- Screenshots or GIF
- AI-assistant usage declaration
- Three-minute demo video

Every command in the README must be executed and verified.

## Claude demo critique

```text
Review the final demo as a judge.
Evaluate:
- problem clarity in the first 20 seconds
- whether SigNoz is visibly central
- whether the blast graph is understandable cold
- whether the alert miss and repair are undeniable
- whether anything appears staged or overclaimed
- whether technical explanation delays the story

Suggest cuts, never additions.
Write docs/reviews/demo-final.md.
```

## Human submission checklist

- Fresh-machine reproduction succeeds.
- Foundry `casting.yaml` and lock are committed.
- Healthy and broken functional equivalence is visible.
- Broken PR is blocked for exactly the expected reasons.
- Demo video is within the event limit.
- AI-assistant usage is declared where required.
- Submission form and event-specific requirements are rechecked.

---

## 4. Daily schedule

| Day | Codex | Claude Code | Human checkpoint |
|---|---|---|---|
| 1 | Scaffold, Foundry, demo app, fixtures | Architecture and determinism review | Foundry and signals flow; alert behavior proven |
| 2 | SigNoz adapter and contract miner | Adapter and mutation reviews | Contract contains four canonical checks |
| 3 | Verification engine and alert verification | False-pass review | Healthy PASS ×4; broken has exactly 3 FAILs |
| 4 | CI gate and blast graph | CI and visual review | Broken workflow blocked; report understandable |
| 5 | End-to-end demo command | Cold-evaluator review | Three green runs; `demo-freeze` |
| 6 | P0/P1 fixes; stretch only if earned | Release-candidate review | Zero P0s |
| 7 | README, video and submission | Final demo critique | Submission approved |

---

## 5. Standard agent prompts

### Codex start-of-phase prompt

```text
Read AGENTS.md, docs/PRODUCT_SPEC.md, docs/BUILD_PLAN.md and docs/STATUS.md.
Implement Phase <N> only.

Before editing, state:
1. Phase objective
2. Files and directories you expect to change
3. The acceptance script that must pass
4. Any ambiguity or external dependency
5. Confirmation that no product non-goal is required

Implement the smallest complete solution.
Before finishing:
- run all relevant tests
- run scripts/accept/phase<N>.sh
- update docs/STATUS.md
- report files changed, commands run and unresolved risks
- never claim success while any check fails
```

### Claude phase-review prompt

```text
Review Phase <N> independently against docs/PRODUCT_SPEC.md and docs/BUILD_PLAN.md.
Do not restyle working code and do not edit implementation by default.

Hunt:
- false passes and false failures
- nondeterminism
- unsupported API assumptions
- missing tests
- secret exposure
- stale-state contamination
- misleading evidence
- hidden scope expansion
- demo failure modes

For every finding provide:
- severity
- reproduction
- expected behavior
- smallest safe correction

Write the result to docs/reviews/phase<N>.md.
```

### Claude scope-police prompt

```text
Compare the proposed diff with docs/PRODUCT_SPEC.md.
Classify each material addition as required, optional, unnecessary or risky.
Recommend deletions or postponements that increase the probability of a complete,
reproducible submission.
Do not approve scope expansion without a spec edit and human decision.
```

---

## 6. Final definition of execution success

The build plan succeeds when a fresh evaluator can:

1. Launch the documented environment.
2. Observe healthy functional tests and Guardian checks passing.
3. Observe the healthy alert firing.
4. Deploy the functionally equivalent broken release.
5. Observe exactly three evidence-backed Guardian failures.
6. See the affected panel and alert in the blast graph.
7. Observe CI block the release.
8. Inject the same fault and observe the alert miss.
9. Deploy the repair.
10. Observe Guardian pass and the alert fire again.

Anything not needed for this sequence is secondary.
