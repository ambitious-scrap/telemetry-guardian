package verifier

import (
	"bytes"
	"context"
	"errors"
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
	filters         []string
	traceCalls      int
	historyRequests []signoz.AlertHistoryRequest
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
	return queryResult(value), nil
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

func queryResult(value float64) signoz.QueryResult {
	if value == 0 {
		return signoz.QueryResult{}
	}
	return signoz.QueryResult{Results: []signoz.QuerySeries{{
		QueryName: "A",
		Aggregations: []signoz.QueryAggregation{{
			Series: []signoz.QueryTimeSeries{{Values: []signoz.QueryPoint{{Value: value}}}},
		}},
	}}}
}
