# Telemetry Guardian roadmap

Deferred ideas only. Nothing here may be built before the demo freeze, and no
new core feature may be added after it. The authoritative documents remain
`Telemetry_Guardian_PRODUCT_SPEC.md` and `Telemetry_Guardian_BUILD_PLAN.md`.

## Deferred to later phases

- Phase 7 hardening and release-candidate review.
- Phase 8 submission packaging.

## Deferred stretch goals

Spec section 28. Stretch work may begin only after all acceptance criteria
pass.

- Trace-to-log correlation coverage as a verifiable percentage.
- Basic cardinality budget: detect a known forbidden metric dimension.
- Ghost topology: peer service observed in client spans but never emitting,
  labeled as a probable instrumentation gap.
- Propagation View: deterministic animated dependency impact.

## Permanently out of scope

Spec section 5 non-goals are binding: root-cause analysis, incident-origin
detection, change-point detection, causal inference, automatic remediation,
cardinality forecasting, collector configuration pull requests, GitHub App
development, SigNoz core source changes, and a generic observability policy
engine.
