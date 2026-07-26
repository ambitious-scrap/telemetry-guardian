# Telemetry Guardian release-candidate review

## 1. Executive verdict

**NO-GO pending P0 corrections.** The protected Phase 6 path is frozen and
validated: the local healthy, broken, and repaired scenarios passed, the
three-run freeze validation passed, and `demo-freeze` is content-identical to
`origin/main`. Offline checks also pass.

The release candidate still has four correctness blockers at boundaries that
the product documents explicitly protect: unsupported source input can be
silently omitted, a malformed canonical contract can verify the wrong field or
operation, query points are counted without checking their returned timestamp
against the requested window, and a malformed verdict can be rendered as a
healthy report. These are static, reproducible code paths, not claims about a
new live-demo failure. They must be corrected before submission.

Hosted CI is a separate, known environment limitation. Run `30192433727`
built and tested Guardian, then failed only at the connection step because
`SIGNOZ_URL` and `SIGNOZ_TOKEN` were empty. That makes the hosted healthy
workflow criterion **PARTIAL**, not a product-code P0 by itself.

Review method: read the two root authority documents and `docs/STATUS.md`,
inspected each implementation and acceptance boundary once, ran only the
specified offline validation, and did not run SigNoz, Foundry, integration, or
demo cycles. Prior live results below are preserved Phase 1/4/5/6 evidence
recorded in the repository and freeze history; they were not rerun in this
review.

## 2. Release blockers

| ID | Severity | Affected requirement | Blocker |
|---|---|---|---|
| P0-1 | P0 | FR-005; CM-004; NFR-004, NFR-007 | The typed decoder and filter parser can discard unknown query fields or malformed filter terms and still produce a complete-looking contract. |
| P0-2 | P0 | FR-007, FR-008, FR-009; NFR-007 | Verifier validation binds canonical IDs to types, but not to their required field, operation, alert ID, or signal values. A malformed contract can verify a different dependency under a canonical ID. |
| P0-3 | P0 | FR-007, FR-008, FR-009; NFR-004 | `traceCount` sums every returned point without enforcing the requested start/end window. Out-of-window points can satisfy a current run. |
| P0-4 | P0 | FR-013, FR-014; NFR-007 | Report construction trusts `verdict.Overall` instead of recomputing it from check states. A verdict with `overall_state: PASS` and a failed check is rendered as the calm healthy report. |

## 3. FR-001 through FR-015 matrix

`IMPLEMENTED` means executable code and test/acceptance evidence exist for the
canonical path. `PARTIAL` means the main path exists but a required boundary
or acceptance proof is incomplete. `ABSENT` means no executable capability was
found. `NOT_APPLICABLE` is not used for an MVP functional requirement.

| Requirement | Classification | Implementation evidence | Test / acceptance evidence | Exact paths | Remaining risk; smallest safe correction |
|---|---|---|---|---|---|
| FR-001 Dashboard retrieval | IMPLEMENTED | `SigNozClient.GetDashboard` uses the typed HTTP adapter and preserves the returned ID/deep link. | Fixture fake, HTTP adapter tests, and the live adapter test cover retrieval; Phase 2 acceptance seeds and reads a dashboard. | `internal/signoz/client.go:41-54,322-331`; `internal/signoz/fake.go`; `internal/signoz/client_test.go`; `internal/signoz/live_test.go`; `scripts/accept/phase2.sh` | Semantic empty-resource validation is still a P1; add typed invalid-response validation for missing required dashboard fields. |
| FR-002 Alert retrieval | IMPLEMENTED | `SigNozClient.GetAlert` reads the typed alert endpoint and maps the condition, thresholds, ID, and returned deep link. | Fixture, live adapter, and Phase 2 acceptance coverage exist. | `internal/signoz/client.go:333-343,617-705`; `internal/signoz/client_test.go`; `internal/signoz/live_test.go`; `scripts/accept/phase2.sh` | Same semantic malformed-resource P1 as FR-001. |
| FR-003 Dashboard dependency extraction | IMPLEMENTED | Miner accepts the proven builder shape, extracts `cart.value`, the panel identity, filter, and source paths. | Golden contract, stable mapping, deduplication, and mutation tests pass; Phase 3 acceptance checks the canonical fields. | `internal/miner/miner.go:116-233`; `internal/miner/miner_test.go`; `internal/miner/testdata/canonical-contract.yaml`; `scripts/accept/phase3.sh` | The exact supported fixture shape is intentionally narrow; P0-1 covers inputs outside the known shape. |
| FR-004 Alert dependency extraction | IMPLEMENTED | Miner extracts `error.type`, `payment.authorize`, and `payment-timeout` from the alert query and annotations. | Golden and mutation tests plus Phase 3 acceptance cover the seeded alert. | `internal/miner/miner.go:236-333`; `internal/miner/miner_test.go`; `fixtures/alerts/telemetry-guardian-payment-timeout.json`; `scripts/accept/phase3.sh` | Alert consumer IDs depend on the mutable alert name; see P1-4. |
| FR-005 Unsupported-query handling | PARTIAL | Known PromQL, SQL, formulas, trace operators, grouping, ordering, having, functions, disabled queries, and non-builder forms are surfaced as `ErrUnsupported`. | Mutation tests cover nested formula, unsupported query type, and malformed source path, but do not inject unknown raw JSON fields or malformed extra filter terms. | `internal/signoz/client.go:785-803,831-842`; `internal/miner/miner.go:135-176,252-290,382-443`; `internal/miner/miner_test.go`; `scripts/accept/phase3.sh` | P0-1 allows skipped source constructs. Strictly reject unknown relevant wire nodes and reject any unparsed filter term; add raw-fixture mutation tests. |
| FR-006 Contract generation | IMPLEMENTED | Contract domain validation and deterministic handwritten YAML serialization emit the four canonical checks without timestamps, tokens, or machine paths. | Golden-file and byte-stability tests pass; Phase 3 acceptance counts four checks and checks all canonical values. | `internal/contracts/contracts.go:19-30,184-277`; `internal/contracts/contracts_test.go`; `contracts/telemetry.guardian.yaml`; `internal/miner/testdata/canonical-contract.yaml`; `scripts/accept/phase3.sh` | Consumer ID stability under a renamed display name is a P1, not a failure of the current canonical output. |
| FR-007 Required-field verification | IMPLEMENTED | `verifyField` runs bounded trace queries, requires an isolated run filter, distinguishes sufficient absence from no data, and emits PASS/FAIL/INCONCLUSIVE. | Offline healthy/broken/no-load tests and the repeated Phase 4 acceptance prove the canonical outcomes. | `internal/verifier/verifier.go:127-176`; `internal/verifier/verifier_test.go`; `internal/verifier/live_test.go`; `scripts/accept/phase4.sh` | P0-2 and P0-3 can defeat the boundary for malformed contracts or out-of-window response points. |
| FR-008 Required-operation verification | IMPLEMENTED | `verifyOperation` scopes by service/run and requires the canonical operation count. | Offline and preserved Phase 4 healthy/broken/no-load outcomes cover PASS/PASS/INCONCLUSIVE. | `internal/verifier/verifier.go:147-160`; `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh` | P0-2 permits a canonical ID to carry a different operation; bind ID to semantic fields. |
| FR-009 Alert-must-fire verification | IMPLEMENTED | Verifier retrieves alert history after fault injection, filters stale timestamps, uses bounded polling, and distinguishes FAIL from stale/no-data INCONCLUSIVE. | Stale, pre-injection, missing-history, cancellation, and canonical live-test paths are covered; Phase 6 records firing/miss/firing. | `internal/verifier/verifier.go:232-310,350-362`; `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh`; `scripts/demo.sh` | The verifier reads one alert-history page; P1-2 requires bounded cursor traversal. |
| FR-010 Evidence preservation | PARTIAL | Verdicts preserve run ID, retrieval description, window, sample count, data quality, summary, and a sanitized alert deep link. | Phase 4 acceptance asserts those fields; report tests inspect the drawer and unsafe-link omission. | `internal/evidence/evidence.go:19-44`; `internal/verifier/verifier.go:195-228`; `internal/report/report.go:127-133`; `internal/evidence/evidence_test.go`; `internal/report/report_test.go` | The evidence model has no structured query payload or fault timestamp for all checks, and link host/scheme safety is incomplete. Add typed query metadata and an allowlisted link validator. |
| FR-011 Consumer impact resolution | IMPLEMENTED | Requirements retain consumer IDs and the report resolves them against contract consumers, rejecting unknown mappings. | Miner mapping/deduplication tests and broken graph tests cover affected panel and alert mappings. | `internal/contracts/contracts.go:35-59,137-181`; `internal/report/report.go:77-155`; `internal/miner/miner_test.go`; `internal/report/report_test.go` | A tampered but nonempty `AffectedConsumers` list can omit known mappings; merge/check against the contract in P1-3. |
| FR-012 Consumer blast graph | IMPLEMENTED | Report builds deterministic dashboard, panel, requirement, and alert nodes with `BREAKS`, `REQUIRED_BY`, and `PART_OF` edges and fixed coordinates. | Report golden-style tests and Phase 5/6 acceptance verify nodes, edges, deterministic output, offline rendering, and consumer names. | `internal/report/report.go:241-343,385-396`; `internal/report/report_test.go`; `scripts/accept/phase5.sh`; `scripts/accept/phase6.sh` | INCONCLUSIVE checks currently receive `BREAKS` edges; see P1-10. |
| FR-013 CI blocking | PARTIAL | Workflow builds/tests, runs Guardian, preserves artifacts with `if: always()`, and enforces exit code zero; local CI script maps all four classifications. | Phase 5 fixture acceptance proves exit 0/1/2/3 behavior and artifact presence. No successful hosted healthy workflow exists; the only recorded hosted run failed at missing configuration. | `.github/workflows/guardian.yml`; `scripts/ci/classify.sh`; `scripts/ci/guardian.sh`; `scripts/accept/phase5.sh`; hosted run `30192433727` | Hosted proof is incomplete, and the CI wrapper has stale/missing-verdict edge cases. Add artifact-state validation and rerun once repository secrets/variables are configured. |
| FR-014 Inconclusive handling | IMPLEMENTED | `INCONCLUSIVE` has precedence over FAIL, maps to exit 2, and is rendered with a distinct title/classification. | No-load, stale, timeout, cancellation, missing-history, report, and CI classification tests pass. | `internal/evidence/evidence.go:65-86`; `internal/verifier/verifier.go`; `internal/report/report.go:202-224`; `internal/verifier/verifier_test.go`; `internal/report/report_test.go`; `scripts/accept/phase5.sh` | Unknown check states can be treated as PASS by `Aggregate`; malformed verdict validation is included in P1-9. |
| FR-015 Reproducible demo | PARTIAL | `scripts/demo.sh` is a bounded 22-stage, restartable local orchestration with preserved artifacts; README documents prerequisites and outcomes. | Phase 6 acceptance and three freeze runs passed against the isolated environment; no fresh-machine run was performed in this review. | `scripts/demo.sh`; `scripts/accept/phase6.sh`; `README.md`; `docs/STATUS.md`; `foundry/casting.yaml`; `foundry/casting.yaml.lock` | Fresh-machine proof and image reproducibility remain incomplete; pin the validated images and execute one clean evaluator run before submission. |

## 4. NFR matrix

| Requirement | Classification | Implementation evidence | Test / acceptance evidence | Exact paths | Remaining risk; smallest safe correction |
|---|---|---|---|---|---|
| NFR-001 Determinism | IMPLEMENTED | Contract normalization, custom YAML ordering, graph sorting, and fixed coordinates remove map/order/random layout variation. | Contract byte-stability, report byte-stability, and three preserved freeze runs pass. | `internal/contracts/contracts.go:184-277`; `internal/report/report.go:277-343`; `internal/contracts/contracts_test.go`; `internal/miner/miner_test.go`; `internal/report/report_test.go` | Stable IDs based on mutable names weaken determinism across resource renames; see P1-4. |
| NFR-002 Bounded operations | PARTIAL | HTTP requests, verifier queries, polling, readiness, alert observation, and shell curls have explicit bounds. | Offline timeout/cancellation tests, shell syntax, and Phase 4/6 acceptance cover the main loops. | `internal/signoz/client.go:16-18,401-437`; `internal/verifier/verifier.go:188-230,306-362`; `scripts/env/wait-ready.sh`; `scripts/demo.sh`; `scripts/load/wait-alert.sh` | `foundryctl cast` itself has no wrapper timeout and alert-history pagination is not bounded by a page budget. Add a bounded cast invocation and cursor/page cap. |
| NFR-003 Security | PARTIAL | Tokens are excluded from committed fixtures, adapter errors redact server detail, runtime token files are chmod 600, and report text/link filters exist. | Secret scans and secret-redaction tests pass; Phase 5/6 artifact scans pass in preserved acceptance. | `internal/signoz/client.go:85-129,448-460`; `internal/evidence/evidence.go:96-112`; `internal/report/report.go:359-383`; `scripts/seed/auth.sh`; `scripts/demo.sh`; `scripts/accept/phase5.sh`; `scripts/accept/phase6.sh` | Unsafe URI hosts/schemes can survive in verdict evidence, credential-bearing auth files lack explicit 600 modes, and the demo logs arbitrary OTLP error bodies. See P1-1 and P1-8. |
| NFR-004 Evidence integrity | PARTIAL | Run IDs, explicit windows, stale alert timestamps, sample counts, and data quality are represented. | Phase 4 stale/pre-injection tests and preserved repeated acceptance cover canonical isolation. | `internal/verifier/verifier.go:275-303,321-347`; `internal/evidence/evidence.go`; `internal/verifier/verifier_test.go` | P0-3 ignores returned point timestamps; P1-2 ignores history cursors; P1-3 omits structured query payload. |
| NFR-005 Testability | IMPLEMENTED | Fake SigNoz client and scenario clients isolate miner/verifier/report tests from live infrastructure. | `go test ./...` passes offline; fixture, golden, mutation, error, and report tests are present. | `internal/signoz/fake.go`; `internal/signoz/client_test.go`; `internal/miner/miner_test.go`; `internal/verifier/verifier_test.go`; `internal/report/report_test.go` | Add the missing adversarial regression tests identified in P0/P1. |
| NFR-006 Maintainability | IMPLEMENTED | Core packages use domain types; raw SigNoz wire types are confined to `internal/signoz`; transport is one standard-library HTTP adapter. | Package tests and vet pass; the miner depends on `SigNozClient`, not HTTP. | `internal/signoz/client.go`; `internal/miner/miner.go`; `internal/contracts/contracts.go`; `Makefile` | The report template is dense and has a few unused adapter helpers; defer cleanup until correctness fixes are complete. |
| NFR-007 Honest output | PARTIAL | PASS/FAIL/INCONCLUSIVE classifications and relationship-only graph language are explicit; root-cause/causality features are absent. | State and causal-language tests plus Phase 5/6 acceptance pass for canonical fixtures. | `internal/evidence/evidence.go`; `internal/report/report.go:103-113,241-257`; `internal/report/report_test.go`; `README.md` | P0-4 can show a malformed verdict as healthy; P1-10 labels INCONCLUSIVE dependencies as `BREAKS`. |
| NFR-008 Performance | PARTIAL | Verification and alert windows are bounded and configured (10-second query/completeness defaults, 60-second alert check). | Repository evidence proves bounded behavior but contains no preserved timing measurement for the under-10-second mining or under-3-minute verification targets. | `internal/verifier/verifier.go:14-20`; `cmd/guardian/main.go:104-181`; `Telemetry_Guardian_PRODUCT_SPEC.md` | Do not claim target compliance. Capture one non-live timing record in a controlled acceptance run after correctness fixes, or document the measured limitation. |

## 5. Acceptance-criteria matrix

The acceptance scripts below are the executable evidence. Where the row says
“preserved,” the prior Phase acceptance is recorded in `docs/STATUS.md` and the
freeze history; this review intentionally did not rerun it.

### Contract mining

| Review ID / criterion | Classification | Implementation and test evidence | Paths | Remaining risk; smallest correction |
|---|---|---|---|---|
| CM-001 One dashboard is read successfully | IMPLEMENTED | Typed `GetDashboard`, fixture/live adapter tests, and Phase 2/3 seed-and-mine path. | `internal/signoz/client.go`; `internal/signoz/client_test.go`; `internal/signoz/live_test.go`; `scripts/accept/phase3.sh` | Semantic empty-resource validation is P1. |
| CM-002 One alert is read successfully | IMPLEMENTED | Typed `GetAlert`, fixture/live tests, and Phase 2/3 seed-and-mine path. | `internal/signoz/client.go`; `internal/signoz/client_test.go`; `internal/signoz/live_test.go`; `scripts/accept/phase3.sh` | Semantic empty-resource validation is P1. |
| CM-003 Dependencies are extracted | IMPLEMENTED | Canonical miner golden output contains all four checks and consumer mappings. | `internal/miner/miner_test.go`; `internal/miner/testdata/canonical-contract.yaml`; `contracts/telemetry.guardian.yaml` | Stable IDs across renames are P1. |
| CM-004 Unsupported constructs are explicit | PARTIAL | Known nodes fail in miner mutation tests; raw unknown fields and malformed filter terms are not rejected. | `internal/signoz/client.go:458`; `internal/miner/miner.go:382-443`; `internal/miner/miner_test.go` | P0-1; add strict/raw mutation coverage. |
| CM-005 Stable YAML contract is produced | IMPLEMENTED | Handwritten deterministic serializer and golden/byte-stability tests pass; Phase 3 acceptance counts four checks. | `internal/contracts/contracts.go`; `internal/contracts/contracts_test.go`; `scripts/accept/phase3.sh` | Stable ID rename behavior remains P1. |

### Verification

| Review ID / criterion | Classification | Implementation and test evidence | Paths | Remaining risk; smallest correction |
|---|---|---|---|---|
| VR-001 Healthy `cart.value` passes | IMPLEMENTED | Offline scenario test and preserved Phase 4/6 live result. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P0-2/P0-3 adversarial boundary. |
| VR-002 Healthy `error.type` passes | IMPLEMENTED | Offline scenario test and preserved Phase 4/6 live result. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P0-2/P0-3 adversarial boundary. |
| VR-003 Healthy `payment.authorize` passes | IMPLEMENTED | Offline scenario test and preserved Phase 4/6 live result. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P0-2/P0-3 adversarial boundary. |
| VR-004 Healthy `payment-timeout` fires | IMPLEMENTED | Alert-history verifier, live verifier test, and preserved Phase 6 healthy firing. | `internal/verifier/verifier.go`; `internal/verifier/live_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P1-2 pagination. |
| VR-005 Broken `cart.value` fails | IMPLEMENTED | Offline broken scenario and exact Phase 4/6 expected failure set. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P0-3 window enforcement. |
| VR-006 Broken `error.type` fails | IMPLEMENTED | Offline broken scenario and exact Phase 4/6 expected failure set. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P0-3 window enforcement. |
| VR-007 Broken `payment.authorize` still passes | IMPLEMENTED | Offline broken scenario asserts operation PASS; Phase 4/6 exact verdict checks it. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P0-2 canonical semantic binding. |
| VR-008 Broken alert check fails | IMPLEMENTED | Offline broken scenario and preserved Phase 4/6 alert miss. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh` | P1-2 pagination and P0 stale boundaries. |
| VR-009 Broken release has exactly three failures | IMPLEMENTED | Phase 4 and Phase 6 scripts compare the exact three IDs and operation PASS. | `scripts/accept/phase4.sh`; `scripts/accept/phase6.sh`; `scripts/demo.sh` | P0 fixes must retain exact canonical assertion. |
| VR-010 Broken release remains functionally correct | IMPLEMENTED | Identical functional test and fault response assertions for healthy/broken/repaired variants. | `demo/checkout/main_test.go`; `scripts/demo.sh`; `scripts/accept/phase6.sh` | No current release-blocking risk found. |
| VR-011 No data produces INCONCLUSIVE | IMPLEMENTED | Offline no-load scenario and Phase 4 live no-load path assert four INCONCLUSIVE results and exit 2. | `internal/verifier/verifier_test.go`; `scripts/accept/phase4.sh`; `internal/evidence/evidence.go` | Unknown malformed states need P1-9 validation. |

### Evidence and impact

| Review ID / criterion | Classification | Implementation and test evidence | Paths | Remaining risk; smallest correction |
|---|---|---|---|---|
| EI-001 Every result contains a query/retrieval and time window | IMPLEMENTED | Verifier fills retrieval/start/end; Phase 4 assertions require nonempty values; report drawer renders them. | `internal/verifier/verifier.go`; `internal/report/report.go`; `scripts/accept/phase4.sh`; `internal/report/report_test.go` | Structured query payload is absent; tracked as FR-010/P1-3. |
| EI-002 Every violation lists affected consumers | IMPLEMENTED | Verifier copies requirement consumers; report rejects missing failed mappings; graph tests name panel and alert. | `internal/verifier/verifier.go:208-228`; `internal/report/report.go:139-155`; `internal/report/report_test.go` | Nonempty but incomplete tampered mappings are not merged; P1-3. |
| EI-003 Blast graph renders deterministically | IMPLEMENTED | Sorted nodes/edges and fixed coordinates; byte-stability tests and Phase 5/6 checks pass. | `internal/report/report.go:277-343`; `internal/report/report_test.go`; `scripts/accept/phase5.sh`; `scripts/accept/phase6.sh` | No force/random layout found. |
| EI-004 Consumer evidence can be inspected | IMPLEMENTED | Native HTML drawer, consumer keyboard handlers, evidence fields, safe-link filtering. | `internal/report/report.go:385-396`; `internal/report/report_test.go`; `scripts/accept/phase5.sh` | P1-1 link safety and P2 focus-trap hardening remain. |

### CI

| Review ID / criterion | Classification | Implementation and test evidence | Paths | Remaining risk; smallest correction |
|---|---|---|---|---|
| CI-001 Healthy workflow is green | PARTIAL | Workflow has a zero exit enforcement path and local healthy fixture acceptance. The only hosted run failed before verification because required repository values were absent. | `.github/workflows/guardian.yml`; `scripts/ci/guardian.sh`; `scripts/accept/phase5.sh`; hosted run `30192433727` | Do not mark hosted healthy green. Configure `SIGNOZ_URL`/`SIGNOZ_TOKEN`, rerun once, and preserve the run. |
| CI-002 Broken workflow is blocked | IMPLEMENTED | Exit 1 classification, `continue-on-error` plus final zero test, and fixture-backed artifact acceptance prove the blocking contract. | `.github/workflows/guardian.yml`; `scripts/ci/classify.sh`; `scripts/ci/guardian.sh`; `scripts/accept/phase5.sh` | No hosted broken run exists; the wrapper edge cases are P1-6. |
| CI-003 Inconclusive state is distinct | IMPLEMENTED | Exit 2 maps to `VERIFICATION_INCONCLUSIVE`; report and summary never use healthy text for inconclusive fixtures. | `scripts/ci/classify.sh`; `internal/report/report.go`; `scripts/accept/phase5.sh`; `internal/report/report_test.go` | P1-10 prevents misleading `BREAKS` edges in inconclusive graph views. |
| CI-004 Verification artifact is uploaded | IMPLEMENTED | Workflow upload has `if: always()` and script writes verdict, classification, exit code, and summary for all four fixture exits. | `.github/workflows/guardian.yml`; `scripts/ci/guardian.sh`; `scripts/accept/phase5.sh` | Stale artifact cleanup and code-0 integrity need P1-6. |

### Reproducibility

| Review ID / criterion | Classification | Implementation and test evidence | Paths | Remaining risk; smallest correction |
|---|---|---|---|---|
| RP-001 Environment starts from documented configuration | IMPLEMENTED | README prerequisites match `scripts/env/up.sh`, readiness probes, Foundry casting, and locked configuration; preserved Phase 1/6 startup passed. | `README.md`; `foundry/casting.yaml`; `foundry/casting.yaml.lock`; `scripts/env/up.sh`; `scripts/env/wait-ready.sh`; `scripts/accept/phase6.sh` | Image tags are mutable; P1-10. |
| RP-002 Demo succeeds three consecutive times | IMPLEMENTED | Phase 1 and Phase 6 acceptance records report repeated successful scenarios and three freeze runs. | `docs/STATUS.md`; `scripts/accept/phase1.sh`; `scripts/accept/phase6.sh`; `demo-freeze` | No live rerun was allowed in this review. |
| RP-003 Fresh evaluator can follow README | PARTIAL | README has prerequisites, one-command demo, expected outcomes, artifacts, and troubleshooting; no fresh-machine execution evidence is preserved. | `README.md`; `scripts/demo.sh`; `scripts/accept/phase6.sh` | Pin images and run a clean evaluator trial after P0 fixes; do not claim this criterion solely from documentation. |
| RP-004 Teardown removes resources | IMPLEMENTED | `down.sh` removes the named compose project and checkout; Phase 1/6 acceptance assert no containers, volumes, or network survive. | `scripts/env/down.sh`; `scripts/accept/phase1.sh`; `scripts/accept/phase6.sh` | Generated Foundry files remain ignored; no stable-demo redesign recommended. |

### Presentation

| Review ID / criterion | Classification | Implementation and test evidence | Paths | Remaining risk; smallest correction |
|---|---|---|---|---|
| PR-001 Broken release functional tests visibly pass | IMPLEMENTED | Stage 9 runs the same tests and compares healthy/broken responses. | `scripts/demo.sh`; `scripts/accept/phase6.sh`; `demo/checkout/main_test.go` | No current risk. |
| PR-002 Guardian visibly blocks the release | IMPLEMENTED | Stage 12 requires exit 1 and exactly three named failures. | `scripts/demo.sh`; `scripts/accept/phase6.sh` | P0 verifier/report guards remain release blockers. |
| PR-003 Graph names affected consumers | IMPLEMENTED | Stage 14 and acceptance grep for panel, alert, and `BREAKS`; report tests assert expected nodes/edges. | `scripts/demo.sh`; `scripts/accept/phase6.sh`; `internal/report/report_test.go` | No current risk in the frozen canonical graph. |
| PR-004 Live alert miss is demonstrated | IMPLEMENTED | Stage 15 checks no new firing notification after the broken fault; Phase 1/6 acceptance preserves the miss. | `scripts/demo.sh`; `scripts/load/assert-alert-miss.sh`; `scripts/accept/phase6.sh` | No current risk in the protected path. |
| PR-005 Repair and successful alert are demonstrated | IMPLEMENTED | Stages 16–21 deploy the healthy variant as repaired, assert PASS x4, and assert firing. | `scripts/demo.sh`; `scripts/accept/phase6.sh`; `docs/STATUS.md` | No current risk in the protected path. |

## 6. P0 findings

### P0-1 — Unsupported source content can be silently omitted

- **Affected requirement:** FR-005, CM-004, NFR-004, NFR-007.
- **Evidence:** `internal/signoz/client.go:451-460` unmarshals the raw success
  payload into permissive wire structs without `DisallowUnknownFields`.
  `internal/signoz/client.go:785-803,831-842` records only a finite list of
  known unsupported nodes. `internal/miner/miner.go:382-443` parses filters by
  accepting recognized terms while `filterTerms` uses `continue` for an
  unparseable or duplicate term.
- **Inspection/reproduction:** Add an otherwise valid query filter term that
  is not in the exact `field = 'value'` grammar, or add a relevant unknown raw
  query node to the sanitized dashboard/alert response. The recognized terms
  remain, `scopedFilter` succeeds, and the miner can emit a contract without
  the skipped content. Existing mutation tests cover an empty filter and known
  unsupported nodes, not this boundary.
- **Expected:** Unsupported or malformed content must fail loudly or produce
  a machine-detectable warning; a contract must not look complete after a
  dependency is skipped.
- **Actual:** The parsed subset can be normalized into a complete-looking
  contract.
- **Smallest safe correction:** Make the supported wire decoder strict for
  relevant resource/query objects and make filter parsing reject any nonempty
  term that is not fully consumed, including conflicting duplicates. Add raw
  JSON fixture mutations and assert no contract is emitted.
- **Likely files:** `internal/signoz/client.go`, `internal/miner/miner.go`,
  `internal/signoz/client_test.go`, `internal/miner/miner_test.go`, sanitized
  testdata.
- **Required validation:** Offline raw-response and malformed-filter tests,
  Phase 3 acceptance, and the focused secret/diff checks.

### P0-2 — Canonical IDs do not bind canonical semantics

- **Affected requirement:** FR-007, FR-008, FR-009, NFR-007.
- **Evidence:** `internal/verifier/verifier.go:105-124` validates only the
  four IDs, their types, and the presence of a run ID in required-field
  filters. It does not compare `Field`, `Signal`, `Operation`, `AlertID`, or
  timeout to the canonical values. `internal/contracts/contracts.go:137-181`
  validates shape but allows those values to differ.
- **Inspection/reproduction:** Change the canonical contract's
  `required-field-cart-value.field` to another field or its
  `required-operation-payment-authorize.operation` to another operation while
  retaining the canonical ID and type. The contract remains structurally
  valid and `verifyField`/`verifyOperation` query the changed value.
- **Expected:** A canonical check ID must verify exactly its documented
  dependency, or the input must be rejected as invalid configuration.
- **Actual:** A different dependency can receive a canonical PASS/FAIL result.
- **Smallest safe correction:** Validate the complete canonical tuple in
  `validateChecks` (including signal, field/operation, alert ID, and bounded
  timeout), then add mutated-contract tests for every tuple.
- **Likely files:** `internal/verifier/verifier.go`,
  `internal/verifier/verifier_test.go`, possibly
  `internal/contracts/contracts.go` and its tests.
- **Required validation:** Offline malformed-contract tests, all Phase 4
  canonical scenarios, and Phase 5/6 fixture report checks.

### P0-3 — Returned time-series points are not checked against the requested window

- **Affected requirement:** FR-007, FR-008, FR-009, NFR-004.
- **Evidence:** `internal/verifier/verifier.go:321-347` sums every value in
  every returned series but never compares `QueryPoint.Timestamp` with the
  `start` and `end` arguments. Run-ID filtering alone does not prove temporal
  freshness.
- **Inspection/reproduction:** Supply a fake/client result containing a
  positive point timestamp before `start` or after `end`. `traceCount` adds it
  to the sample/count total and can satisfy the check.
- **Expected:** Only points in the explicit verification window may contribute;
  out-of-window or missing timestamps should be rejected or treated as
  insufficient evidence.
- **Actual:** Any returned point value contributes regardless of timestamp.
- **Smallest safe correction:** Validate/filter every point by the requested
  window and classify missing/invalid timestamps as invalid response or
  inconclusive. Update fake fixtures to carry timestamps and add before/after
  window mutations.
- **Likely files:** `internal/verifier/verifier.go`,
  `internal/verifier/verifier_test.go`, `internal/signoz/testdata` if needed.
- **Required validation:** Offline stale-window tests, repeated Phase 4
  acceptance, and one focused live verifier smoke only if the correction
  changes the live result.

### P0-4 — A malformed verdict can render as healthy

- **Affected requirement:** FR-013, FR-014, NFR-007.
- **Evidence:** `internal/report/report.go:85-113` validates that
  `verdict.Overall` is a known state, but never recomputes it from the sorted
  check states. The healthy HTML branch at the report template renders the
  calm healthy state whenever `Document.State` is `PASS`.
- **Inspection/reproduction:** Provide a structurally complete verdict with
  `overall_state: PASS` and one check state `FAIL`. `Build` accepts the state
  values and the report takes the PASS branch. The existing report test checks
  incomplete check counts and separate healthy/inconclusive fixtures, but not
  an inconsistent aggregate.
- **Expected:** An inconsistent verdict must be rejected or its state must be
  recomputed; a failed check must never produce a healthy report.
- **Actual:** The report can say “contract healthy” while containing a failed
  check in its input.
- **Smallest safe correction:** Recompute `evidence.Aggregate(checks)` in
  `report.Build` and reject a mismatch before constructing the document. Make
  `Aggregate` reject unknown/empty states rather than defaulting to PASS.
- **Likely files:** `internal/report/report.go`, `internal/report/report_test.go`,
  `internal/evidence/evidence.go`, `internal/evidence/evidence_test.go`.
- **Required validation:** Malformed-verdict tests for every aggregate
  mismatch, all four exit classifications, report fixtures, and
  `scripts/accept/phase5.sh`.

## 7. P1 findings

### Cheap, low-risk P1 findings

#### P1-1 — Deep-link safety is not environment-scoped

- **Affected requirement:** FR-010, NFR-003; evidence/link acceptance.
- **Evidence:** `internal/evidence/evidence.go:96-112` accepts any parsed scheme
  and host, while `internal/report/report.go:373-383` accepts any HTTP(S)
  host. Neither function has the configured SigNoz host available for an
  allowlist. A `javascript:` link can survive into `verdict.json` even though
  the HTML renderer later declines to link it; an arbitrary HTTPS host can be
  linked in the report.
- **Inspection/reproduction:** Set the returned alert `webUrl` to an unsafe
  scheme or an unrelated HTTPS host and run the existing offline verdict/report
  boundary.
- **Expected:** Only a returned link for the configured SigNoz environment and
  safe HTTP(S) scheme should survive; all other links should be omitted.
- **Actual:** Verdict evidence is broader than the safe report link policy,
  and the report has no configured-host check.
- **Smallest safe correction:** Pass the configured SigNoz origin into evidence
  and report sanitizers, require HTTP(S) plus exact origin, and add scheme,
  host, userinfo, query, and fragment mutations.
- **Likely files:** `internal/evidence/evidence.go`, `internal/report/report.go`,
  `internal/verifier/verifier.go`, tests.
- **Required validation:** Offline unsafe-link tests, Phase 5 acceptance, and
  secret scan.

#### P1-2 — Verifier ignores alert-history cursors

- **Affected requirement:** FR-009, NFR-002, NFR-004.
- **Evidence:** The adapter preserves `AlertHistory.NextCursor` and tests two
  pages (`internal/signoz/client.go:974-1017`,
  `internal/signoz/client_test.go`), but `internal/verifier/verifier.go:356-362`
  requests one page with `Limit: 100` and never follows `NextCursor`.
- **Inspection/reproduction:** Return 100 older firing events plus a
  `NextCursor` whose next page contains the current event. The verifier can
  finish without seeing the current event.
- **Expected:** Page traversal must continue, within a bounded page/time
  budget, until a fresh event is found or the result is exhausted.
- **Actual:** Correct pagination is available at the adapter boundary but is
  not consumed by verification.
- **Smallest safe correction:** Add a bounded cursor loop in verifier alert
  history retrieval and tests for current event on page two and a cursor loop.
- **Likely files:** `internal/verifier/verifier.go`,
  `internal/verifier/verifier_test.go`.
- **Required validation:** Offline pagination/stale tests and focused Phase 4
  acceptance; no full demo rerun unless live behavior changes.

#### P1-3 — Evidence and Markdown summary omit structured query and complete consumer impact

- **Affected requirement:** FR-010, FR-011, evidence/impact acceptance.
- **Evidence:** `internal/evidence.Record` contains a free-form `Retrieval`
  string but no query payload, filter, aggregation, or fault timestamp for
  ordinary field/operation checks. `internal/report/report.go:178-199`
  Markdown prints requirement/state/sample/quality/link, but no panel or alert
  consumer names. `Build` trusts a nonempty `AffectedConsumers` list instead of
  merging it with the contract mapping (`report.go:139-155`).
- **Inspection/reproduction:** Generate the broken local fixture summary and
  inspect it: the HTML graph has consumer names, while the Markdown job
  summary does not. Remove one known consumer from a nonempty verdict mapping;
  the report accepts the incomplete impact set.
- **Expected:** Evidence should retain the query/retrieval payload required by
  the product model, and CI text should name affected consumers; known mappings
  should not be silently omitted.
- **Actual:** The HTML drawer is richer than the CI Markdown and the report
  trusts caller-provided nonempty mappings.
- **Smallest safe correction:** Add sanitized structured query metadata and
  render consumer names/source IDs in Markdown; union or validate verdict
  consumers against contract consumers.
- **Likely files:** `internal/evidence/evidence.go`, `internal/verifier/verifier.go`,
  `internal/report/report.go`, tests and fixtures.
- **Required validation:** Offline golden/report tests and Phase 5 acceptance.

#### P1-4 — Consumer IDs depend on mutable display names

- **Affected requirement:** Contract-format stable consumer IDs; FR-006,
  NFR-001.
- **Evidence:** `internal/miner/miner.go:300-305` derives alert consumer IDs
  from `alert.Name`; `dashboardConsumerID` at `miner.go:467-468` includes the
  dashboard title. Stable resource IDs are already available in the source.
- **Inspection/reproduction:** Change only a dashboard panel title or alert
  display name while retaining its resource ID. The generated consumer ID
  changes.
- **Expected:** A display-name edit should retain the consumer identity and
  update only its label.
- **Actual:** Consumer identity changes with the display name.
- **Smallest safe correction:** Derive IDs from dashboard ID + panel ID and
  alert resource ID; retain names only as labels. Update the canonical golden
  output and add rename mutations.
- **Likely files:** `internal/miner/miner.go`, `internal/miner/miner_test.go`,
  `contracts/telemetry.guardian.yaml`.
- **Required validation:** Golden, deterministic, rename, and Phase 3 tests.

#### P1-5 — Semantically malformed successful resources are not invalid responses

- **Affected requirement:** FR-001, FR-002, FR-005; typed error coverage.
- **Evidence:** `internal/signoz/client.go:451-460` checks envelope/JSON
  syntax, but `dashboardWire.dashboard` and `alertWire.alert` map empty IDs,
  titles, widgets, conditions, or thresholds without returning
  `ErrInvalidResponse`; the miner catches some of these only later.
- **Inspection/reproduction:** Return a syntactically valid success envelope
  with `{}` as dashboard or alert data. The adapter returns a zero-valued typed
  resource rather than an invalid-response error.
- **Expected:** Malformed resource shape at the adapter boundary should be a
  typed invalid-response error.
- **Actual:** Semantic validation is deferred to the miner and is absent for
  direct adapter callers.
- **Smallest safe correction:** Validate required typed resource identity and
  envelope shape in the adapter; retain miner validation for supported query
  semantics.
- **Likely files:** `internal/signoz/client.go`, `internal/signoz/client_test.go`.
- **Required validation:** Offline malformed dashboard/alert fixtures and
  Phase 2 acceptance.

#### P1-6 — CI artifact wrapper has stale/missing-verdict and false-green edges

- **Affected requirement:** FR-013, artifact-retention acceptance; NFR-004.
- **Evidence:** `scripts/ci/guardian.sh:14-20` ignores `cp` failure in fixture
  mode. Lines 28-42 create an invalid fallback verdict but retain the original
  code, and a code-0 run can still exit 0 after report rendering fails. The
  script also does not clear an old verdict before a failing run.
- **Inspection/reproduction:** Use `GUARDIAN_FIXTURE_VERDICT` pointing to a
  missing file with `GUARDIAN_FIXTURE_EXIT=0`, or leave an old verdict in the
  artifact directory and force an invalid run. The wrapper can return zero or
  upload stale content.
- **Expected:** Every exit 0 must have a valid current verdict and report;
  failed runs must not reuse prior artifacts.
- **Actual:** Artifact content and exit classification can diverge at these
  edges.
- **Smallest safe correction:** Clear/publish artifacts atomically, fail fixture
  copy errors, validate verdict/report for exit 0, and write a fresh typed
  invalid-configuration artifact on exit 3.
- **Likely files:** `scripts/ci/guardian.sh`, `scripts/accept/phase5.sh`.
- **Required validation:** Fixture mutations for all exit codes, artifact
  retention tests, and Phase 5 acceptance.

#### P1-7 — Concurrent demos and verdict writes can collide

- **Affected requirement:** FR-015; NFR-001, NFR-004; restartability.
- **Evidence:** `scripts/demo.sh:29-50` uses shared `.run/demo`, `.run/demo.prev`,
  fixed ports/container names, and a second-resolution stamp without a lock.
  `cmd/guardian/main.go:293-307` writes every verdict through the shared
  `path + ".tmp"` file.
- **Inspection/reproduction:** Start two demo invocations or two verifies
  targeting the same output path. They can move/delete each other's evidence,
  race on the checkout container, or overwrite the shared temp file.
- **Expected:** Overlapping invocations must be rejected safely or isolated;
  temporary publication must not corrupt another run.
- **Actual:** Shared state is uncoordinated.
- **Smallest safe correction:** Add an atomic repository runtime lock for the
  demo and use unique temporary files plus atomic publish for CLI outputs.
- **Likely files:** `scripts/demo.sh`, `cmd/guardian/main.go`, focused tests.
- **Required validation:** Offline shell/CLI concurrency fixture tests and
  `scripts/accept/phase6.sh` if the lock path changes.

#### P1-8 — Secret-bearing runtime files and exporter error bodies need tighter handling

- **Affected requirement:** NFR-003; security acceptance.
- **Evidence:** `scripts/seed/auth.sh:14-15,27-31` writes registration and login
  request/response files containing passwords or access tokens without an
  explicit 600 mode, relying on the parent directory's 700 mode. In
  `demo/checkout/main.go`, the exporter reads up to 1024 bytes of an OTLP error
  body and logs it through `log.Printf`.
- **Inspection/reproduction:** Inspect file creation modes after auth setup or
  return a sensitive collector error body. The files are not explicitly
  private and the body is copied into application logs.
- **Expected:** Credential-bearing files should be 600 and telemetry/error
  bodies should never be logged raw.
- **Actual:** Directory protection helps, but the explicit file policy and
  body redaction are incomplete.
- **Smallest safe correction:** Create request/response files with 600 mode
  (or chmod immediately) and log only endpoint/status/classification, never the
  response body.
- **Likely files:** `scripts/seed/auth.sh`, `demo/checkout/main.go`.
- **Required validation:** Offline mode/secret tests, shell syntax, and
  preserved artifact scans.

#### P1-9 — Malformed verdict/state and nil verifier context are under-validated

- **Affected requirement:** FR-013, FR-014; NFR-004, NFR-007.
- **Evidence:** `internal/evidence.Aggregate` at `evidence.go:65-75` starts at
  PASS and ignores unknown/empty states. `cmd/guardian/main.go` decodes one JSON
  value without rejecting trailing content. `verifier.Verify` does not reject
  a nil context before `context.WithTimeout` is called in `traceCount` and
  `getAlert`.
- **Inspection/reproduction:** Construct a verdict with an unknown check state
  or trailing JSON; or call `Verify(nil, ...)` with a valid contract. The first
  can default aggregate behavior, and the second can panic at the context
  boundary.
- **Expected:** Malformed verdicts and nil contexts should be typed invalid
  input, never PASS or panic.
- **Actual:** Validation is incomplete outside the normal CLI-generated path.
- **Smallest safe correction:** Validate all state/evidence fields and trailing
  JSON, and return a typed invalid-input error for nil context.
- **Likely files:** `internal/evidence/evidence.go`, `cmd/guardian/main.go`,
  `internal/verifier/verifier.go`, tests.
- **Required validation:** Offline malformed-input, aggregate, decoder, and
  cancellation tests.

#### P1-10 — INCONCLUSIVE graph edges are labeled as breaks

- **Affected requirement:** FR-012, FR-014; NFR-007.
- **Evidence:** `internal/report/report.go:136-155` sends every non-PASS check
  into `addConsumerGraph`; `addConsumerGraph` always emits `BREAKS` at lines
  248-256, including when `state == INCONCLUSIVE`.
- **Inspection/reproduction:** Render `internal/report/testdata/inconclusive.json`
  and inspect the graph document/HTML. The report correctly says
  `VERIFICATION_INCONCLUSIVE`, but its relationship legend and edges say
  `BREAKS`/“Failed dependency”.
- **Expected:** Inconclusive evidence must be visibly distinct and must not
  assert a verified break.
- **Actual:** State badge is distinct, but the graph relationship overclaims.
- **Smallest safe correction:** Use an `UNRESOLVED`/`OBSERVED_BY` presentation
  for inconclusive checks or omit the break graph while retaining the evidence
  drawer.
- **Likely files:** `internal/report/report.go`, `internal/report/report_test.go`.
- **Required validation:** Inconclusive golden/report tests and Phase 5
  acceptance.

### Deferred P1 findings

#### P1-11 — Reproducibility uses mutable `latest` image tags

- **Affected requirement:** FR-015, RP-001; NFR-001, NFR-008.
- **Evidence:** `foundry/casting.yaml.lock:153-154,305-306,373-374` uses
  `signoz/signoz-otel-collector:latest`, `signoz/signoz-mcp-server:latest`,
  and `signoz/signoz:latest`, while `docs/STATUS.md` records a validated
  SigNoz `v0.133.0` environment.
- **Inspection/reproduction:** Compare the lock file image/version fields with
  the documented validated version. A fresh cast can resolve different images
  without a tree change.
- **Expected:** The lock should identify the validated immutable image/version
  used by the freeze.
- **Actual:** The environment is reproducible only while `latest` remains
  compatible.
- **Smallest safe correction:** Pin the exact validated images/digests in the
  Foundry lock and perform one focused clean-start validation; do not change the
  frozen tag until that is explicitly approved.
- **Likely files:** `foundry/casting.yaml.lock`, potentially
  `foundry/casting.yaml`, README/status documentation.
- **Required validation:** Foundry start/readiness and the protected acceptance
  sequence after human approval; deferred here because live/demo reruns are
  prohibited.

#### P1-12 — Hosted candidate telemetry endpoint is an unresolved deployment assumption

- **Affected requirement:** FR-013, CI-001; hosted reproducibility.
- **Evidence:** `.github/workflows/guardian.yml` only health-checks
  `SIGNOZ_URL`; candidate deployment is delegated to
  `scripts/env/deploy.sh`, which hard-codes
  `OTEL_EXPORTER_OTLP_ENDPOINT=http://host.docker.internal:14318` at line 31.
  The workflow does not provision the local collector or configure a remote
  OTLP endpoint.
- **Inspection/reproduction:** Trace the workflow's connect → deploy path in
  the committed scripts. There is no workflow step that starts
  `scripts/env/up.sh` or maps the candidate exporter to `SIGNOZ_URL`.
- **Expected:** A hosted healthy run should have an explicitly reachable
  candidate telemetry ingestion path.
- **Actual:** Hosted success is unproven even after the known repository values
  are configured; this is recorded as an assumption, not asserted as a new
  live failure.
- **Smallest safe correction:** Before changing code, have the human choose and
  document either a runner-local collector or an externally reachable OTLP
  endpoint, then run one focused hosted smoke. Do not debug it in this review.
- **Likely files:** `.github/workflows/guardian.yml`, `scripts/env/deploy.sh`,
  repository configuration.
- **Required validation:** One configured hosted smoke only; no live run was
  performed here.

## 8. P2 findings

### P2-1 — Evidence dialog has no focus trap or browser-level accessibility test

- **Affected requirement:** Presentation accessibility; NFR-005.
- **Evidence:** The HTML has keyboard handlers, visible `:focus-visible`, a
  close button, Escape restoration, and reduced-motion CSS, but the dialog at
  `internal/report/report.go:396` does not trap Tab focus. Tests inspect source
  hooks rather than exercising a browser.
- **Inspection/reproduction:** Tab repeatedly after opening the drawer; focus
  can leave the modal while `aria-modal="true"` remains set.
- **Expected:** Keyboard users should have a complete modal interaction or the
  drawer should use non-modal semantics.
- **Actual:** The required basic keyboard hooks exist; focus containment is
  incomplete.
- **Smallest safe correction:** Add a small native focus trap or remove modal
  semantics and add a browser fixture test.
- **Likely files:** `internal/report/report.go`, `internal/report/report_test.go`.
- **Required validation:** Focused offline/browser accessibility check; not a
  release blocker for the frozen local demo.

### P2-2 — State CSS class casing is inconsistent

- **Affected requirement:** Report polish; NFR-007.
- **Evidence:** The template emits `class="state-{{.Document.State}}"` with
  uppercase states, while CSS defines lowercase `.state-pass`,
  `.state-fail`, and `.state-inconclusive`. Text, borders, and semantic labels
  still communicate state, so this is not color-only or a verdict error.
- **Inspection/reproduction:** Inspect the generated HTML class and CSS
  selectors in `internal/report/report.go:389-396`.
- **Expected:** State-specific styling selectors should match their emitted
  classes.
- **Actual:** Those selectors do not match the uppercase body class.
- **Smallest safe correction:** Normalize the emitted class or selectors and
  add a source assertion.
- **Likely files:** `internal/report/report.go`, report tests.
- **Required validation:** Offline report acceptance; defer until P0/P1 work.

### P2-3 — Phase 0 architecture notes contain stale optional directory references

- **Affected requirement:** Maintainability/documentation only.
- **Evidence:** `docs/ARCHITECTURE.md` describes `internal/impact` and `web/`
  ownership, but the tracked tree has no such directories; the actual Phase 5
  implementation is intentionally in `internal/report` and is validated.
- **Inspection/reproduction:** Compare `git ls-files` with the package table and
  worktree ownership sections.
- **Expected:** Architecture notes should describe the shipped boundary.
- **Actual:** They retain the pre-implementation recommended layout.
- **Smallest safe correction:** Update the non-authoritative architecture note
  during documentation cleanup; do not create packages or redesign the graph.
- **Likely files:** `docs/ARCHITECTURE.md`.
- **Required validation:** Documentation link/path check.

## 9. Deletion candidates

No deletion is recommended before the P0/P1 corrections. The following were
reviewed but deferred because deletion would not clearly reduce submission
risk:

| Candidate | Why it might be outside the minimum | Dependencies | Regression risk | Recommendation |
|---|---|---|---|---|
| `internal/signoz/client.go:740` `mapQuerySpecs` and `:756` `mapQuerySpec` | No non-test caller was found; the adapter currently uses `mapQuerySpecsAt`/`mapQuerySpecAt`. | Same-package wire mapping and possible future adapter tests. | Hidden fixture shape support could be removed accidentally. | **Defer**; do not delete specialized adapter helpers during hardening. |
| `internal/miner/miner.go:108` `MineContract` alias | No current caller was found; `Mine` is the active API. | External callers may use the descriptive alias. | Breaking package API for negligible gain. | **Defer**. |

No `internal/impact` or `web` package should be created or deleted: the
validated report implementation already supplies the required graph.

## 10. Frozen-demo integrity

- At review start, before creating the review artifact,
  `git diff --stat demo-freeze HEAD` produced no output.
- At review start, `git diff --exit-code demo-freeze HEAD` exited 0 with no
  output. After the review-only commit, the branch necessarily adds only the
  two review-document paths listed below; that documentation diff is not a
  product-tree change.
- `git diff --exit-code demo-freeze origin/main`: exit 0 after fetch.
- `git diff --name-only demo-freeze HEAD` after the review commit contains only
  `docs/STATUS.md` and `docs/reviews/release-candidate.md`.
- `origin/main` and the current `main` both contain Phase 6 merge commit
  `a883e72` (`feat: add protected end-to-end demo and Phase 6 freeze gate (#7)`).
- The `demo-freeze` tag was not moved, recreated, or modified.
- No finding recommends redesigning `scripts/demo.sh`, Phase 6 acceptance,
  Foundry casting, canonical contract, verifier semantics, alert timing, or
  frozen graph relationships before the P0 boundary fixes are understood.

## 11. Hosted-CI environment limitation

The recorded hosted run `30192433727` passed Guardian build/test and failed at
“Connect to isolated SigNoz environment” because the repository variable and
secret were not configured. The safe error was:

```text
curl: (3) URL rejected: No host part in the URL
```

The repository still has no externally reachable `SIGNOZ_URL` and no
`SIGNOZ_TOKEN` secret. Therefore:

- local protected demo and Phase 6 acceptance are recorded as passed;
- hosted healthy workflow is **PARTIAL**, not IMPLEMENTED;
- this missing configuration is not called a product-code P0;
- the next hosted action is configuration by a human admin, followed by one
  healthy workflow run and artifact inspection;
- the unresolved OTLP deployment assumption is separately recorded as P1-12,
  without debugging it here.

## 12. Recommended implementation order

1. Fix P0-1 through P0-4 with focused offline regression tests. These are the
   release gate because they affect completeness, stale evidence, canonical
   verdict correctness, or healthy-report truthfulness.
2. Fix cheap security and artifact safeguards: P1-1, P1-6, and P1-8.
3. Fix verifier/report boundary integrity: P1-2, P1-3, P1-9, and P1-10.
4. Fix stable identity and concurrency behavior: P1-4 and P1-7.
5. Resolve or explicitly defer the environment items P1-11 and P1-12 with the
   human; configure hosted CI separately.
6. Consider P2 accessibility/documentation polish only after the protected
   acceptance remains green.

Required validation after fixes: focused tests for each changed boundary,
`make fmt-check`, `make lint`, `make test`, the affected acceptance scripts,
one final `scripts/accept/phase5.sh`/Phase 6-preserving check as appropriate,
and no tag movement.

## 13. Go or no-go decision

**NO-GO for submission at this review checkpoint.** The freeze tree is intact
and the validated demo is strong, but P0-1 through P0-4 can produce a silently
incomplete contract, a semantically false check result, stale-data PASS, or a
false healthy report. After those are corrected and their focused tests pass,
the human should reassess P1s and configure hosted CI. Do not begin stretch
work while this decision remains no-go.

## 14. P0 resolution pass

The following four approved P0 corrections were implemented without changing
the protected Phase 6 path. The original findings and all P1/P2 classifications
above remain unchanged.

| Finding | Resolution evidence | Validation | Status |
|---|---|---|---|
| P0-1 silently omitted query content | `internal/signoz/client.go` now strictly decodes dependency-bearing query structures while retaining permissive resource metadata; `internal/miner/miner.go` fully consumes supported filter grammar and rejects malformed or duplicate terms. | SigNoz/miner regression tests, canonical golden tests, malformed-filter mutations, sanitized metadata/query fixtures, and focused Phase 3 live mining acceptance. | RESOLVED |
| P0-2 canonical IDs not bound to semantics | `internal/verifier/verifier.go` validates every canonical ID against its exact type, signal, field/operation, filter, alert ID, and timeout; filters are compared as parsed term sets. | Canonical tuple mutations cover signal, field, operation, alert ID, timeout, service/run/error filters, extras, duplicate/missing IDs; invalid inputs make zero query calls and CLI exit 3. | RESOLVED |
| P0-3 returned points outside requested window | `internal/verifier/verifier.go` rejects invalid timestamps and counts only inclusive Unix-millisecond window points. | Boundary, mixed, zero-timestamp, stale-positive, healthy/broken/no-load tests and the final Phase 4 acceptance passed. | RESOLVED |
| P0-4 contradictory verdict aggregate | `internal/evidence/evidence.go` treats empty/unknown states as inconclusive, and `internal/report/report.go` recomputes and validates aggregate state before rendering. | Contradictory PASS/FAIL/INCONCLUSIVE, empty, and unknown-state tests plus CLI no-output/exit-3 coverage passed. | RESOLVED |

The hosted healthy-workflow limitation remains **PARTIAL** because the
externally reachable `SIGNOZ_URL` and `SIGNOZ_TOKEN` repository configuration
is still absent. P1 and P2 findings remain deferred; no stretch work was begun.

## 15. Selected P1 resolution pass

The following approved P1 corrections were implemented without changing the
protected Phase 6 path. The original audit findings above remain preserved;
unselected P1 and all P2 findings remain deferred.

| Finding | Resolution evidence | Validation | Status |
|---|---|---|---|
| P1-2 bounded alert-history pagination | `internal/verifier/verifier.go` follows `NextCursor` with a five-page bound, repeated-cursor detection, a single query timeout context, and fresh-event short-circuiting. | Verifier tests cover page-two fresh events, stale-plus-fresh pages, terminal cursors, loops, page budgets, cancellation, timeout, and INCONCLUSIVE mapping; Phase 4 acceptance passed. | RESOLVED |
| P1-5 semantic success-envelope validation | `internal/signoz/client.go` validates required dashboard identity/title/widgets, supported widget queries, alert identity/composite query, supported alert query, and canonical threshold fields while retaining permissive top-level metadata decoding. | Sanitized success mutations cover missing identities, widgets, queries, composite query, and thresholds; typed `ErrInvalidResponse` assertions and existing fixtures pass. | RESOLVED |
| P1-6 CI artifact integrity | `scripts/ci/guardian.sh` clears stale outputs, stages and publishes artifacts, validates current verdicts before report generation, and emits fresh exit-3 diagnostics. | `scripts/accept/phase5.sh` covers all exit classes, stale outputs, missing fixtures, malformed verdicts, report failure, retention, and classifications; Phase 5 acceptance passed. | RESOLVED |
| P1-8 runtime secret files and exporter bodies | `scripts/seed/auth.sh` applies `umask 077` and explicit `0600` modes to credential-bearing files; `demo/checkout/main.go` logs only safe OTLP status classifications. | Checkout exporter regression test and Phase 5 shell assertions prove raw response bodies are omitted and private-file safeguards are present. | RESOLVED |
| P1-9 malformed input/state validation | `cmd/guardian/main.go` rejects trailing JSON; `internal/verifier/verifier.go` rejects nil contexts with `ErrInvalidInput`; report construction rejects unknown or empty aggregate/check states. | CLI, verifier, evidence, and report regression tests cover trailing input, nil context, unknown/empty states, and precedence. | RESOLVED |
| P1-10 honest INCONCLUSIVE graph relationships | `internal/report/report.go` emits `UNRESOLVED` for INCONCLUSIVE requirement-to-consumer edges and preserves `BREAKS` for FAIL. | Report tests and Phase 5 acceptance verify deterministic neutral relationships and distinct healthy/inconclusive output. | RESOLVED |

Focused offline tests, formatting, lint, repository tests, Phase 4
acceptance, and Phase 5 acceptance passed. Phase 2 acceptance reached the
live adapter but its standalone filtered builder-query smoke was rejected by
the current instance as `invalid_input`; a focused safe probe reproduced that
nonempty-filter compatibility issue while an empty-filter request succeeded.
No adapter query breadth or deferred P1-11 image pinning was changed. The
required `make demo-smoke` run was therefore not started. `demo-freeze` was
not moved or recreated.

### Deferred after selected P1 pass

P1-1, P1-3, P1-4, P1-7, P1-11, P1-12, and every P2 finding remain deferred.
No stretch work or Phase 8 work began.

### Phase 2 acceptance blocker correction

The standalone Phase 2 failure was reproduced with a bounded safe probe. An
empty-filter trace request returned HTTP 200, while the exact service-filtered
trace and log requests returned HTTP 400 with `invalid_input` before telemetry
warmup. After one unique warmup run used the existing deploy, workload, fault,
and telemetry assertion scripts, the same filtered trace and log requests
returned HTTP 200. This confirms a cold SigNoz field-discovery/data precondition,
not an obsolete adapter query shape.

`scripts/accept/phase2.sh` now performs that deterministic warmup before
`TestLiveSigNozAdapter`, keeps the filtered trace and log checks unchanged, and
cleans only the checkout runtime container. Phase 2 acceptance passed. No
adapter, query-language, image, Foundry, or Phase 4 implementation was changed.

The separately requested `make demo-smoke` validation was attempted once,
followed by one focused sample/window probe and the permitted final attempt.
Both smoke attempts stopped at healthy verification because `cart.value` and
`payment.authorize` reached sample count 1 of 5 and were INCONCLUSIVE, while
`error.type` and alert firing passed. The focused probe confirmed positive
bounded query points after warmup. The strict verifier/demo window boundary is
outside this Phase 2-only correction and remains unresolved; no protected demo
or verifier semantics were changed. No Phase 4 or Phase 5 rerun was performed,
and `demo-freeze` remains unchanged.

### Demo query-window blocker correction

The Phase 6 healthy verifier failure was caused by five rapid requests sharing
a SigNoz 5-second bucket whose timestamp preceded the second-level demo start.
P0-3 timestamp filtering behaved correctly by excluding that out-of-window
point, leaving the later fault point and an observed sample count of 1/5.

`scripts/demo.sh` now aligns the shared candidate start to the beginning of the
current `QUERY_STEP_SECONDS=5` bucket immediately before generating the five
requests. The helper asserts divisible epochs remain unchanged, offsets one
through four align backward, no adjustment moves forward, and every adjustment
is less than five seconds. Unique run IDs preserve candidate isolation.

Verifier semantics, SigNoz query construction, minimum samples, fault timing,
and end-window logic remain unchanged. `make accept-phase6` passed healthy
PASS x4, broken exactly three FAILs with `payment.authorize` PASS, and repaired
PASS x4. Functional responses remained identical; healthy and repaired alerts
fired and the broken alert missed. No image or Foundry file changed, and
`demo-freeze` remains unchanged.
