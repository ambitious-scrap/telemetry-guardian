package verifier

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/evidence"
	"github.com/ambitious-scrap/telemetry-guardian/internal/signoz"
)

func TestCanonicalHealthyBrokenAndNoLoad(t *testing.T) {
	tests := []struct {
		mode     string
		expected []evidence.State
		exitCode int
	}{
		{"healthy", []evidence.State{evidence.Pass, evidence.Pass, evidence.Pass, evidence.Pass}, 0},
		{"broken", []evidence.State{evidence.Fail, evidence.Fail, evidence.Pass, evidence.Fail}, 1},
		{"no-load", []evidence.State{evidence.Inconclusive, evidence.Inconclusive, evidence.Inconclusive, evidence.Inconclusive}, 2},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			config := testConfig()
			client := &scenarioClient{mode: test.mode, fault: config.FaultInjectedAt}
			verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.ExitCode() != test.exitCode {
				t.Fatalf("exit = %d, want %d", verdict.ExitCode(), test.exitCode)
			}
			for i, expected := range test.expected {
				if verdict.CheckResults[i].State != expected {
					t.Fatalf("check %d = %s, want %s", i, verdict.CheckResults[i].State, expected)
				}
				assertCompleteEvidence(t, verdict.CheckResults[i], config.RunID)
			}
		})
	}
}

func TestPartialAndStaleTelemetryAreInconclusive(t *testing.T) {
	for _, mode := range []string{"partial", "stale"} {
		t.Run(mode, func(t *testing.T) {
			config := testConfig()
			client := &scenarioClient{mode: mode, fault: config.FaultInjectedAt}
			verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.Overall != evidence.Inconclusive || verdict.ExitCode() != 2 {
				t.Fatalf("verdict = %s/%d", verdict.Overall, verdict.ExitCode())
			}
			for _, filter := range client.filters {
				if !strings.Contains(filter, "run.id = '"+config.RunID+"'") {
					t.Fatalf("query was not isolated to active run: %s", filter)
				}
			}
		})
	}
}

func TestOldAlertEventsCannotSatisfyCurrentRun(t *testing.T) {
	config := testConfig()
	client := &scenarioClient{mode: "stale-alert", fault: config.FaultInjectedAt}
	verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	alert := verdict.CheckResults[3]
	if len(client.historyRequests) == 0 {
		t.Fatal("alert history was not queried")
	}
	for _, request := range client.historyRequests {
		if request.State != "firing" {
			t.Fatalf("alert history state = %q, want firing", request.State)
		}
	}
	if alert.State != evidence.Inconclusive || alert.Evidence.DataQuality != evidence.Stale {
		t.Fatalf("alert = %#v", alert)
	}
}

func TestAlertEventBeforeInjectionIsRejected(t *testing.T) {
	config := testConfig()
	client := &scenarioClient{mode: "before-injection", fault: config.FaultInjectedAt}
	verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.CheckResults[3].State != evidence.Inconclusive {
		t.Fatalf("pre-injection event produced %s", verdict.CheckResults[3].State)
	}
}

func TestMissingAlertHistoryIsInconclusive(t *testing.T) {
	config := testConfig()
	client := &scenarioClient{mode: "healthy", fault: config.FaultInjectedAt, historyErr: signoz.ErrNotFound}
	verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.CheckResults[3].State != evidence.Inconclusive {
		t.Fatalf("missing history produced %s", verdict.CheckResults[3].State)
	}
}

func TestAlertHistoryFollowsBoundedCursors(t *testing.T) {
	start := time.UnixMilli(1700000000000)
	end := start.Add(time.Minute)
	stale := signoz.AlertHistoryItem{ID: "stale", State: "firing", Timestamp: start.UnixMilli()}
	fresh := signoz.AlertHistoryItem{ID: "fresh", State: "firing", Timestamp: start.Add(time.Second).UnixMilli()}

	tests := []struct {
		name       string
		pages      map[string]signoz.AlertHistory
		wantItems  int
		wantCursor []string
		wantError  string
	}{
		{
			name: "fresh event on page two",
			pages: map[string]signoz.AlertHistory{
				"":       {NextCursor: "page-2"},
				"page-2": {Items: []signoz.AlertHistoryItem{fresh}},
			},
			wantItems:  1,
			wantCursor: []string{"", "page-2"},
		},
		{
			name: "stale first page and fresh second page",
			pages: map[string]signoz.AlertHistory{
				"":       {Items: []signoz.AlertHistoryItem{stale}, NextCursor: "page-2"},
				"page-2": {Items: []signoz.AlertHistoryItem{fresh}},
			},
			wantItems:  2,
			wantCursor: []string{"", "page-2"},
		},
		{
			name: "empty terminal cursor",
			pages: map[string]signoz.AlertHistory{
				"": {Items: []signoz.AlertHistoryItem{stale}},
			},
			wantItems:  1,
			wantCursor: []string{""},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &historyPaginationClient{pages: test.pages}
			got, err := alertHistory(context.Background(), client, "alert", start, end, time.Second)
			if (test.wantError == "") != (err == nil) {
				t.Fatalf("error = %v, want error %q", err, test.wantError)
			}
			if err != nil {
				return
			}
			if len(got.Items) != test.wantItems {
				t.Fatalf("items = %d, want %d", len(got.Items), test.wantItems)
			}
			if len(client.requests) != len(test.wantCursor) {
				t.Fatalf("requests = %#v, want cursors %#v", client.requests, test.wantCursor)
			}
			for index, request := range client.requests {
				if request.Cursor != test.wantCursor[index] {
					t.Fatalf("request %d cursor = %q, want %q", index, request.Cursor, test.wantCursor[index])
				}
			}
		})
	}
}

func TestAlertHistoryPaginationRejectsLoopsAndPageBudget(t *testing.T) {
	start := time.UnixMilli(1700000000000)
	end := start.Add(time.Minute)
	t.Run("repeated cursor", func(t *testing.T) {
		client := &historyPaginationClient{pages: map[string]signoz.AlertHistory{
			"":     {NextCursor: "loop"},
			"loop": {NextCursor: "loop"},
		}}
		_, err := alertHistory(context.Background(), client, "alert", start, end, time.Second)
		if !errors.Is(err, signoz.ErrInvalidResponse) || !strings.Contains(err.Error(), "cursor loop") {
			t.Fatalf("error = %v, want typed cursor-loop invalid response", err)
		}
	})

	t.Run("page budget exhaustion", func(t *testing.T) {
		pages := make(map[string]signoz.AlertHistory, maxAlertHistoryPages)
		cursor := ""
		for index := 0; index < maxAlertHistoryPages; index++ {
			next := fmt.Sprintf("page-%d", index+1)
			pages[cursor] = signoz.AlertHistory{NextCursor: next}
			cursor = next
		}
		client := &historyPaginationClient{pages: pages}
		_, err := alertHistory(context.Background(), client, "alert", start, end, time.Second)
		if !errors.Is(err, signoz.ErrInvalidResponse) || !strings.Contains(err.Error(), "page budget") {
			t.Fatalf("error = %v, want typed page-budget invalid response", err)
		}
		if len(client.requests) != maxAlertHistoryPages {
			t.Fatalf("requests = %d, want bounded %d", len(client.requests), maxAlertHistoryPages)
		}
	})
}

func TestAlertHistoryPaginationTimeoutAndCancellation(t *testing.T) {
	start := time.UnixMilli(1700000000000)
	end := start.Add(time.Minute)
	client := &historyPaginationClient{block: true}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := alertHistory(canceled, client, "alert", start, end, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}

	timedOut, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := alertHistory(timedOut, client, "alert", start, end, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestAlertHistoryPaginationFailureIsInconclusive(t *testing.T) {
	config := testConfig()
	contract := loadFixtureContract(t)
	pages := make(map[string]signoz.AlertHistory, maxAlertHistoryPages)
	cursor := ""
	for index := 0; index < maxAlertHistoryPages; index++ {
		next := fmt.Sprintf("page-%d", index+1)
		pages[cursor] = signoz.AlertHistory{NextCursor: next}
		cursor = next
	}
	client := &historyPaginationClient{
		pages:       pages,
		traceResult: queryResultAt(5, config.FaultInjectedAt.Add(time.Second)),
	}
	result := verifyAlert(context.Background(), client, contract, *checkByID(&contract, "alert-must-fire-payment-timeout"), config)
	if result.State != evidence.Inconclusive || result.Evidence.DataQuality != evidence.Error {
		t.Fatalf("result = %#v, want INCONCLUSIVE error evidence", result)
	}
}

func TestPermanentAndInfrastructureErrorsAreNotRetriedOrPassed(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"unauthorized", signoz.ErrUnauthorized},
		{"forbidden", signoz.ErrForbidden},
		{"timeout", signoz.ErrTimeout},
		{"malformed", signoz.ErrInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig()
			client := &scenarioClient{mode: "healthy", fault: config.FaultInjectedAt, traceErr: test.err}
			verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.Overall != evidence.Inconclusive || client.traceCalls != 4 {
				t.Fatalf("verdict/calls = %s/%d", verdict.Overall, client.traceCalls)
			}
		})
	}
}

func TestCancellationIsInconclusive(t *testing.T) {
	config := testConfig()
	client := &scenarioClient{mode: "healthy", fault: config.FaultInjectedAt}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	verdict, err := Verify(ctx, client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Overall != evidence.Inconclusive || verdict.ExitCode() != 2 {
		t.Fatalf("canceled verdict = %s/%d", verdict.Overall, verdict.ExitCode())
	}
}

func TestBoundedPollingWaitsForMinimumSamples(t *testing.T) {
	config := testConfig()
	config.End = time.Now().Add(100 * time.Millisecond)
	config.CompletenessTimeout = 50 * time.Millisecond
	config.PollInterval = time.Millisecond
	client := &scenarioClient{mode: "eventual", fault: config.FaultInjectedAt}
	verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.CheckResults[0].State != evidence.Pass || client.traceCalls < 2 {
		t.Fatalf("poll result/calls = %s/%d", verdict.CheckResults[0].State, client.traceCalls)
	}
}

func TestShortCompletedWindowIsInconclusiveWithoutInvalidQuery(t *testing.T) {
	config := testConfig()
	config.Start = config.End.Add(-2 * time.Second)
	config.FaultInjectedAt = config.End.Add(-time.Second)
	client := &scenarioClient{mode: "healthy", fault: config.FaultInjectedAt}
	verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Overall != evidence.Inconclusive || client.traceCalls != 0 {
		t.Fatalf("short window verdict/calls = %s/%d", verdict.Overall, client.traceCalls)
	}
}

func TestInvalidContractAndConfiguration(t *testing.T) {
	contract := loadFixtureContract(t)
	contract.Checks = contract.Checks[:3]
	if _, err := Verify(context.Background(), &scenarioClient{}, contract, testConfig()); !errors.Is(err, contracts.ErrInvalidContract) {
		t.Fatalf("invalid contract error = %v", err)
	}
	config := testConfig()
	config.RunID = "unsafe run"
	if _, err := Verify(context.Background(), &scenarioClient{}, loadFixtureContract(t), config); !errors.Is(err, contracts.ErrInvalidContract) {
		t.Fatalf("invalid config error = %v", err)
	}
}

func TestNilVerifierContextIsTypedInvalidInput(t *testing.T) {
	_, err := Verify(nil, &scenarioClient{}, loadFixtureContract(t), testConfig())
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want typed invalid verifier input", err)
	}
}

func TestCanonicalCheckIDsAreBoundToExactSemantics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*contracts.Contract)
	}{
		{name: "signal", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "required-field-cart-value").Signal = "logs"
		}},
		{name: "field", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "required-field-cart-value").Field = "cart.amount"
		}},
		{name: "operation", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "required-operation-payment-authorize").Operation = "payment.capture"
		}},
		{name: "alert ID", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "alert-must-fire-payment-timeout").AlertID = "other-alert"
		}},
		{name: "timeout", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "alert-must-fire-payment-timeout").Timeout = "61s"
		}},
		{name: "service filter", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "required-field-cart-value").Filter = "service.name = 'other' AND run.id = '__RUN_ID__'"
		}},
		{name: "missing run filter", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "required-field-cart-value").Filter = "service.name = 'checkout'"
		}},
		{name: "wrong error value", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "required-field-error-type").Filter = "service.name = 'checkout' AND run.id = '__RUN_ID__' AND error.type = 'other'"
		}},
		{name: "unexpected extra filter", mutate: func(contract *contracts.Contract) {
			checkByID(contract, "required-field-cart-value").Filter += " AND region = 'us-east-1'"
		}},
		{name: "duplicate check ID", mutate: func(contract *contracts.Contract) {
			contract.Checks[len(contract.Checks)-1].ID = contract.Checks[0].ID
		}},
		{name: "missing canonical check", mutate: func(contract *contracts.Contract) {
			contract.Checks[len(contract.Checks)-1].ID = "unknown-check"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract := loadFixtureContract(t)
			test.mutate(&contract)
			client := &scenarioClient{}
			_, err := Verify(context.Background(), client, contract, testConfig())
			if !errors.Is(err, contracts.ErrInvalidContract) {
				t.Fatalf("error = %v, want invalid contract", err)
			}
			if client.traceCalls != 0 || len(client.historyRequests) != 0 {
				t.Fatalf("invalid contract started queries: traces=%d history=%d", client.traceCalls, len(client.historyRequests))
			}
		})
	}
}

func TestCanonicalFilterOrderIsComparedSemantically(t *testing.T) {
	contract := loadFixtureContract(t)
	check := checkByID(&contract, "required-field-cart-value")
	reordered := "run.id = '__RUN_ID__' AND service.name = 'checkout'"
	check.Filter = reordered
	check.Filters = []string{reordered}
	config := testConfig()
	client := &scenarioClient{mode: "healthy", fault: config.FaultInjectedAt}
	verdict, err := Verify(context.Background(), client, contract, config)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Overall != evidence.Pass || client.traceCalls == 0 {
		t.Fatalf("reordered canonical filter verdict/calls = %s/%d", verdict.Overall, client.traceCalls)
	}
}

func TestTraceCountEnforcesInclusiveReturnedPointWindow(t *testing.T) {
	start := time.UnixMilli(1700000000000)
	end := time.UnixMilli(1700000005000)
	tests := []struct {
		name   string
		points []signoz.QueryPoint
		want   int
	}{
		{name: "before start", points: []signoz.QueryPoint{{Timestamp: start.Add(-time.Millisecond).UnixMilli(), Value: 4}}, want: 0},
		{name: "after end", points: []signoz.QueryPoint{{Timestamp: end.Add(time.Millisecond).UnixMilli(), Value: 4}}, want: 0},
		{name: "at start", points: []signoz.QueryPoint{{Timestamp: start.UnixMilli(), Value: 3}}, want: 3},
		{name: "at end", points: []signoz.QueryPoint{{Timestamp: end.UnixMilli(), Value: 4}}, want: 4},
		{name: "mixed", points: []signoz.QueryPoint{{Timestamp: start.Add(-time.Millisecond).UnixMilli(), Value: 2}, {Timestamp: start.UnixMilli(), Value: 3}, {Timestamp: end.UnixMilli(), Value: 4}, {Timestamp: end.Add(time.Millisecond).UnixMilli(), Value: 5}}, want: 7},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &scenarioClient{traceResult: queryResultPointer(test.points...)}
			got, err := traceCount(context.Background(), client, start, end, "service.name = 'checkout'", "count()", time.Second)
			if err != nil || got != test.want {
				t.Fatalf("count = %d, err = %v, want %d", got, err, test.want)
			}
		})
	}

	client := &scenarioClient{traceResult: queryResultPointer(signoz.QueryPoint{Timestamp: 0, Value: 1})}
	if _, err := traceCount(context.Background(), client, start, end, "service.name = 'checkout'", "count()", time.Second); !errors.Is(err, signoz.ErrInvalidResponse) {
		t.Fatalf("zero timestamp error = %v, want invalid response", err)
	}
}

func TestStalePositivePointsCannotEstablishCurrentTelemetry(t *testing.T) {
	config := testConfig()
	config.CompletenessTimeout = time.Millisecond
	config.PollInterval = time.Millisecond
	client := &scenarioClient{mode: "stale-window", fault: config.FaultInjectedAt}
	verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Overall != evidence.Inconclusive || verdict.ExitCode() != 2 {
		t.Fatalf("stale positive verdict = %s/%d, want inconclusive/2", verdict.Overall, verdict.ExitCode())
	}
}

func checkByID(contract *contracts.Contract, id string) *contracts.Requirement {
	for index := range contract.Checks {
		if contract.Checks[index].ID == id {
			return &contract.Checks[index]
		}
	}
	panic("test contract check not found: " + id)
}

func TestVerdictJSONContainsCompleteEvidenceAndNoSecret(t *testing.T) {
	const secret = "phase4-super-secret"
	config := testConfig()
	client := &scenarioClient{
		mode: "healthy", fault: config.FaultInjectedAt,
		deepLink: "https://signoz.example/alert?access_token=" + secret,
	}
	verdict, err := Verify(context.Background(), client, loadFixtureContract(t), config)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := evidence.WriteJSON(&output, verdict); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), secret) {
		t.Fatal("verdict JSON contains a secret")
	}
	for _, result := range verdict.CheckResults {
		assertCompleteEvidence(t, result, config.RunID)
	}
}

func loadFixtureContract(t *testing.T) contracts.Contract {
	t.Helper()
	file, err := os.Open("../miner/testdata/canonical-contract.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contract, err := contracts.LoadYAML(file)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func testConfig() Config {
	end := time.Now().Add(-time.Second).UTC()
	return Config{
		RunID: "phase4-candidate", AlertResourceID: "alert-fixture-id",
		Start: end.Add(-2 * time.Minute), FaultInjectedAt: end.Add(-time.Minute), End: end,
		MinimumSamples: 5, PollInterval: time.Millisecond,
		CompletenessTimeout: time.Millisecond, QueryTimeout: time.Second,
	}
}

func assertCompleteEvidence(t *testing.T, result evidence.CheckResult, runID string) {
	t.Helper()
	if result.State == "" || result.RequirementID == "" || result.RunID != runID ||
		result.Evidence.Retrieval == "" || result.Evidence.Start.IsZero() || result.Evidence.End.IsZero() ||
		result.Evidence.Summary == "" || result.Evidence.DataQuality == "" ||
		result.Evidence.MinimumSampleCount < 1 || len(result.AffectedConsumers) == 0 {
		t.Fatalf("incomplete evidence: %#v", result)
	}
}

type scenarioClient struct {
	mode            string
	fault           time.Time
	traceErr        error
	historyErr      error
	deepLink        string
	traceResult     *signoz.QueryResult
	filters         []string
	traceCalls      int
	historyRequests []signoz.AlertHistoryRequest
}

type historyPaginationClient struct {
	pages       map[string]signoz.AlertHistory
	requests    []signoz.AlertHistoryRequest
	block       bool
	traceResult signoz.QueryResult
}

func (client *historyPaginationClient) GetDashboard(context.Context, string) (signoz.Dashboard, error) {
	return signoz.Dashboard{}, nil
}

func (client *historyPaginationClient) GetAlert(ctx context.Context, id string) (signoz.Alert, error) {
	if err := ctx.Err(); err != nil {
		return signoz.Alert{}, err
	}
	return signoz.Alert{ID: id}, nil
}

func (client *historyPaginationClient) ExecuteBuilderQuery(context.Context, signoz.BuilderQueryRequest) (signoz.QueryResult, error) {
	return signoz.QueryResult{}, nil
}

func (client *historyPaginationClient) SearchTraces(ctx context.Context, _ signoz.SearchRequest) (signoz.QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return signoz.QueryResult{}, err
	}
	return client.traceResult, nil
}

func (client *historyPaginationClient) SearchLogs(context.Context, signoz.SearchRequest) (signoz.QueryResult, error) {
	return signoz.QueryResult{}, nil
}

func (client *historyPaginationClient) GetAlertHistory(ctx context.Context, _ string, request signoz.AlertHistoryRequest) (signoz.AlertHistory, error) {
	client.requests = append(client.requests, request)
	if client.block {
		<-ctx.Done()
		return signoz.AlertHistory{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return signoz.AlertHistory{}, err
	}
	return client.pages[request.Cursor], nil
}

func (client *scenarioClient) GetDashboard(context.Context, string) (signoz.Dashboard, error) {
	return signoz.Dashboard{}, nil
}

func (client *scenarioClient) GetAlert(ctx context.Context, id string) (signoz.Alert, error) {
	if err := ctx.Err(); err != nil {
		return signoz.Alert{}, err
	}
	return signoz.Alert{ID: id, DeepLink: client.deepLink}, nil
}

func (client *scenarioClient) ExecuteBuilderQuery(context.Context, signoz.BuilderQueryRequest) (signoz.QueryResult, error) {
	return signoz.QueryResult{}, nil
}

func (client *scenarioClient) SearchTraces(ctx context.Context, request signoz.SearchRequest) (signoz.QueryResult, error) {
	client.traceCalls++
	client.filters = append(client.filters, request.Filter)
	if err := ctx.Err(); err != nil {
		return signoz.QueryResult{}, err
	}
	if client.traceErr != nil {
		return signoz.QueryResult{}, client.traceErr
	}
	if client.traceResult != nil {
		return *client.traceResult, nil
	}
	if client.mode == "eventual" && request.End.After(time.Now().Add(5*time.Millisecond)) {
		return signoz.QueryResult{}, signoz.ErrInvalidRequest
	}
	value := 5.0
	switch client.mode {
	case "no-load", "stale":
		value = 0
	case "partial":
		value = 2
	case "eventual":
		if client.traceCalls < 3 {
			value = 0
		}
	case "broken":
		if strings.Contains(request.Filter, "error.type") || strings.HasPrefix(request.Aggregations[0].Expression, "sum(") {
			value = 0
		}
	}
	if request.Start.Equal(client.fault) && strings.Contains(request.Filter, "name = 'payment.authorize'") && value > 0 {
		value = 1
	}
	if strings.Contains(request.Filter, "error.type") && client.mode != "broken" && value > 0 {
		value = 1
	}
	if strings.HasPrefix(request.Aggregations[0].Expression, "sum(") && value > 0 {
		value = 210
	}
	pointTime := request.End
	if client.mode == "stale-window" {
		pointTime = request.Start.Add(-time.Millisecond)
	}
	return queryResultAt(value, pointTime), nil
}

func (client *scenarioClient) SearchLogs(context.Context, signoz.SearchRequest) (signoz.QueryResult, error) {
	return signoz.QueryResult{}, nil
}

func (client *scenarioClient) GetAlertHistory(ctx context.Context, _ string, request signoz.AlertHistoryRequest) (signoz.AlertHistory, error) {
	client.historyRequests = append(client.historyRequests, request)
	if err := ctx.Err(); err != nil {
		return signoz.AlertHistory{}, err
	}
	if client.historyErr != nil {
		return signoz.AlertHistory{}, client.historyErr
	}
	state, eventTime := "firing", client.fault.Add(time.Second)
	switch client.mode {
	case "broken":
		state = "normal"
	case "stale-alert":
		eventTime = client.fault.Add(-time.Second)
	case "before-injection":
		eventTime = client.fault
	case "no-load", "partial", "stale":
		return signoz.AlertHistory{}, nil
	}
	return signoz.AlertHistory{
		Items: []signoz.AlertHistoryItem{{ID: "event", State: state, Timestamp: eventTime.UnixMilli()}},
		Total: 1,
	}, nil
}

func queryResultAt(value float64, timestamp time.Time) signoz.QueryResult {
	if value == 0 {
		return signoz.QueryResult{}
	}
	return signoz.QueryResult{Results: []signoz.QuerySeries{{
		QueryName: "A",
		Aggregations: []signoz.QueryAggregation{{
			Series: []signoz.QueryTimeSeries{{Values: []signoz.QueryPoint{{Timestamp: timestamp.UnixMilli(), Value: value}}}},
		}},
	}}}
}

func queryResultPoints(points ...signoz.QueryPoint) signoz.QueryResult {
	return signoz.QueryResult{Results: []signoz.QuerySeries{{
		QueryName: "A",
		Aggregations: []signoz.QueryAggregation{{
			Series: []signoz.QueryTimeSeries{{Values: points}},
		}},
	}}}
}

func queryResultPointer(points ...signoz.QueryPoint) *signoz.QueryResult {
	result := queryResultPoints(points...)
	return &result
}
