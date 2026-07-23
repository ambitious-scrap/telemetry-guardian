# SigNoz adapter API notes

These notes record the behavior observed against the isolated Phase 1 SigNoz
instance on 2026-07-23. Examples are sanitized; the access token and tenant
values are never stored here.

## Transport and authentication

- Base URL: the configured Phase 1 instance, for example
  `http://127.0.0.1:18080`.
- Authentication: `Authorization: Bearer <access-token>` on every protected
  request.
- The shipped adapter uses one standard-library HTTP transport. MCP was used
  only to investigate the live resource behavior.
- `HTTPClient` adds a bounded request context (10 seconds by default, 15
  seconds in the live test). A caller's cancellation is returned as
  `context.Canceled`.

## Dashboard retrieval

Operation: `GetDashboard(id)`

```text
GET /api/v1/dashboards/{id}
```

Observed success envelope:

```json
{"status":"success","data":{"id":"<id>","data":{"title":"<title>","widgets":[]}}}
```

The adapter maps the nested dashboard content into `Dashboard`, retains the
resource ID, and copies a returned `webUrl` into `DeepLink`. The observed HTTP
response did not include a deep link; no link is constructed from an ID.

## Alert retrieval

Operation: `GetAlert(id)`

```text
GET /api/v2/rules/{id}
```

The success envelope is also `{"status":"success","data":...}`. The
observed data included `id`, `alert`, `alertType`, `ruleType`, `condition`,
`evalWindow`, `frequency`, `labels`, `annotations`, and `webUrl` when present.
The typed result retains the raw ID and returned deep link.

## Builder queries, traces, and logs

Operations: `ExecuteBuilderQuery`, `SearchTraces`, `SearchLogs`

All three use the observed endpoint:

```text
POST /api/v5/query_range
```

The request is a typed `time_series` builder query with Unix-millisecond
`start` and `end` values:

```json
{
  "schemaVersion":"v1",
  "start":1700000000000,
  "end":1700000060000,
  "requestType":"time_series",
  "compositeQuery":{"queries":[{"type":"builder_query","spec":{
    "name":"A",
    "signal":"traces",
    "stepInterval":5,
    "disabled":false,
    "filter":{"expression":"service.name = 'checkout'"},
    "aggregations":[{"expression":"count()"}]
  }}]},
  "formatOptions":{"formatTableResultForUI":false,"fillGaps":false}
}
```

`SearchTraces` and `SearchLogs` use this same shape with `signal` set to
`traces` or `logs`. Observed success data has `type`, `meta`, and nested
`data.results[]`; each result has a `queryName` and either a valid empty
`aggregations` value (`null` or an empty array) or typed aggregation series and
millisecond timestamps. No-data is therefore a valid `QueryResult`, not an
adapter error.

An unknown filter field returned HTTP 400 with `error.type` `invalid-input` and
`error.code` `invalid_input`; the adapter reports that as a typed invalid
request error without copying the server message into an error string.

## Alert history

Operation: `GetAlertHistory(id, request)`

```text
GET /api/v2/rules/{id}/history/timeline
```

Observed query parameters are `start`, `end`, `limit`, `order`, `state`,
`filterExpression`, and `cursor`. The success data is:

```json
{"items":[],"total":0}
```

An empty page is valid. When the service returns `nextCursor`, the typed result
retains it and the caller must pass it back as `AlertHistoryRequest.Cursor` to
fetch the next page. The adapter never silently discards pagination.

## Error observations and mapping

| Observation | Adapter result |
|---|---|
| Missing bearer, HTTP 401, `unauthenticated` | `errors.Is(err, ErrUnauthorized)` |
| Missing permission, HTTP 403 | `errors.Is(err, ErrForbidden)` |
| Missing dashboard/alert, HTTP 404, `not-found` | `errors.Is(err, ErrNotFound)` |
| HTTP 408/504 or bounded transport timeout | `errors.Is(err, ErrTimeout)` |
| Malformed JSON, missing success data, or wrong typed fields | `errors.Is(err, ErrInvalidResponse)` |
| Invalid query input, HTTP 400 `invalid_input` | `errors.Is(err, ErrInvalidRequest)` |
| Caller cancellation | `errors.Is(err, context.Canceled)` and not `ErrTimeout` |

Error strings contain only the operation, classification, and safe API error
code. They never contain the bearer token, authorization header, or raw
telemetry response.
