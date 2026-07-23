package signoz

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
)

// FakeClient is a deterministic, fixture-backed implementation of SigNozClient.
// The exported error fields let offline tests exercise the same failure boundary.
type FakeClient struct {
	DashboardResult Dashboard
	AlertResult     Alert
	QueryResult     QueryResult
	TraceResult     QueryResult
	LogResult       QueryResult
	HistoryResult   AlertHistory

	DashboardError error
	AlertError     error
	QueryError     error
	TraceError     error
	LogError       error
	HistoryError   error
}

var _ SigNozClient = (*FakeClient)(nil)

//go:embed testdata/*.json
var fixtureFiles embed.FS

func NewFixtureFake() (*FakeClient, error) {
	var dashboard dashboardWire
	if err := loadFixture("testdata/dashboard-success.json", &dashboard); err != nil {
		return nil, err
	}
	var alert alertWire
	if err := loadFixture("testdata/alert-success.json", &alert); err != nil {
		return nil, err
	}
	var query queryWire
	if err := loadFixture("testdata/query-success.json", &query); err != nil {
		return nil, err
	}
	var emptyQuery queryWire
	if err := loadFixture("testdata/query-empty.json", &emptyQuery); err != nil {
		return nil, err
	}
	var history historyWire
	if err := loadFixture("testdata/history-empty.json", &history); err != nil {
		return nil, err
	}
	historyResult, err := history.history()
	if err != nil {
		return nil, err
	}
	return &FakeClient{
		DashboardResult: dashboard.dashboard(),
		AlertResult:     alert.alert(),
		QueryResult:     query.result(),
		TraceResult:     query.result(),
		LogResult:       emptyQuery.result(),
		HistoryResult:   historyResult,
	}, nil
}

func loadFixture(name string, output any) error {
	payload, err := fixtureFiles.ReadFile(name)
	if err != nil {
		return err
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("fixture %s: %w", name, err)
	}
	if envelope.Status != "success" || len(envelope.Data) == 0 {
		return fmt.Errorf("fixture %s: missing success data", name)
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("fixture %s: %w", name, err)
	}
	return nil
}

func (f *FakeClient) GetDashboard(ctx context.Context, id string) (Dashboard, error) {
	if id == "" {
		return Dashboard{}, invalidRequest("GetDashboard", "dashboard id")
	}
	if err := fakeContext(ctx, "GetDashboard"); err != nil {
		return Dashboard{}, err
	}
	if f.DashboardError != nil {
		return Dashboard{}, f.DashboardError
	}
	if f.DashboardResult.ID != "" && id != f.DashboardResult.ID {
		return Dashboard{}, &Error{Kind: ErrorNotFound, Operation: "GetDashboard", cause: ErrNotFound}
	}
	return f.DashboardResult, nil
}

func (f *FakeClient) GetAlert(ctx context.Context, id string) (Alert, error) {
	if id == "" {
		return Alert{}, invalidRequest("GetAlert", "alert id")
	}
	if err := fakeContext(ctx, "GetAlert"); err != nil {
		return Alert{}, err
	}
	if f.AlertError != nil {
		return Alert{}, f.AlertError
	}
	if f.AlertResult.ID != "" && id != f.AlertResult.ID {
		return Alert{}, &Error{Kind: ErrorNotFound, Operation: "GetAlert", cause: ErrNotFound}
	}
	return f.AlertResult, nil
}

func (f *FakeClient) ExecuteBuilderQuery(ctx context.Context, request BuilderQueryRequest) (QueryResult, error) {
	if _, err := newQueryRequest(request); err != nil {
		return QueryResult{}, err
	}
	if err := fakeContext(ctx, "ExecuteBuilderQuery"); err != nil {
		return QueryResult{}, err
	}
	if f.QueryError != nil {
		return QueryResult{}, f.QueryError
	}
	return f.QueryResult, nil
}

func (f *FakeClient) SearchTraces(ctx context.Context, request SearchRequest) (QueryResult, error) {
	if _, err := newQueryRequest(BuilderQueryRequest{Start: request.Start, End: request.End, Signal: "traces", QueryName: request.QueryName, StepInterval: request.StepInterval, Filter: request.Filter, Aggregations: request.Aggregations}); err != nil {
		return QueryResult{}, err
	}
	if err := fakeContext(ctx, "SearchTraces"); err != nil {
		return QueryResult{}, err
	}
	if f.TraceError != nil {
		return QueryResult{}, f.TraceError
	}
	return f.TraceResult, nil
}

func (f *FakeClient) SearchLogs(ctx context.Context, request SearchRequest) (QueryResult, error) {
	if _, err := newQueryRequest(BuilderQueryRequest{Start: request.Start, End: request.End, Signal: "logs", QueryName: request.QueryName, StepInterval: request.StepInterval, Filter: request.Filter, Aggregations: request.Aggregations}); err != nil {
		return QueryResult{}, err
	}
	if err := fakeContext(ctx, "SearchLogs"); err != nil {
		return QueryResult{}, err
	}
	if f.LogError != nil {
		return QueryResult{}, f.LogError
	}
	return f.LogResult, nil
}

func (f *FakeClient) GetAlertHistory(ctx context.Context, id string, request AlertHistoryRequest) (AlertHistory, error) {
	if id == "" {
		return AlertHistory{}, invalidRequest("GetAlertHistory", "alert id")
	}
	if err := validateWindow(request.Start, request.End); err != nil {
		return AlertHistory{}, invalidRequest("GetAlertHistory", err.Error())
	}
	if request.Limit < 0 || request.Limit > 1000 {
		return AlertHistory{}, invalidRequest("GetAlertHistory", "limit")
	}
	if err := fakeContext(ctx, "GetAlertHistory"); err != nil {
		return AlertHistory{}, err
	}
	if f.HistoryError != nil {
		return AlertHistory{}, f.HistoryError
	}
	if f.AlertResult.ID != "" && id != f.AlertResult.ID {
		return AlertHistory{}, &Error{Kind: ErrorNotFound, Operation: "GetAlertHistory", cause: ErrNotFound}
	}
	return f.HistoryResult, nil
}

func fakeContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return invalidRequest(operation, "nil context")
	}
	if ctx.Err() == context.DeadlineExceeded {
		return timeoutError(operation, context.DeadlineExceeded)
	}
	return ctx.Err()
}
