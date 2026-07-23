package verifier

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/evidence"
	"github.com/ambitious-scrap/telemetry-guardian/internal/signoz"
)

const (
	defaultMinimumSamples     = 5
	defaultPollInterval       = 2 * time.Second
	defaultCompletenessWindow = 10 * time.Second
	defaultQueryTimeout       = 10 * time.Second
	minimumQueryWindow        = 5 * time.Second
)

type Config struct {
	RunID               string
	AlertResourceID     string
	Start               time.Time
	End                 time.Time
	FaultInjectedAt     time.Time
	MinimumSamples      int
	PollInterval        time.Duration
	CompletenessTimeout time.Duration
	QueryTimeout        time.Duration
}

func Verify(ctx context.Context, client signoz.SigNozClient, contract contracts.Contract, config Config) (evidence.Verdict, error) {
	if err := contract.Validate(); err != nil {
		return evidence.Verdict{}, err
	}
	if client == nil {
		return evidence.Verdict{}, fmt.Errorf("%w: SigNoz client is required", contracts.ErrInvalidContract)
	}
	config = config.defaults()
	if err := config.validate(); err != nil {
		return evidence.Verdict{}, err
	}
	if !safeIdentifier(contract.Service) {
		return evidence.Verdict{}, fmt.Errorf("%w: service must contain only letters, numbers, '.', '_', ':', or '-'", contracts.ErrInvalidContract)
	}
	if err := validateChecks(contract.Checks); err != nil {
		return evidence.Verdict{}, err
	}

	results := make([]evidence.CheckResult, 0, len(contract.Checks))
	for _, requirement := range contract.Checks {
		var result evidence.CheckResult
		switch requirement.Type {
		case "required_field":
			result = verifyField(ctx, client, contract, requirement, config)
		case "required_operation":
			result = verifyOperation(ctx, client, contract, requirement, config)
		case "alert_must_fire":
			result = verifyAlert(ctx, client, contract, requirement, config)
		}
		results = append(results, result)
	}
	return evidence.NewVerdict(config.RunID, contract.Service, contract.Release, config.Start, config.End, results), nil
}

func (config Config) defaults() Config {
	if config.MinimumSamples == 0 {
		config.MinimumSamples = defaultMinimumSamples
	}
	if config.PollInterval == 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.CompletenessTimeout == 0 {
		config.CompletenessTimeout = defaultCompletenessWindow
	}
	if config.QueryTimeout == 0 {
		config.QueryTimeout = defaultQueryTimeout
	}
	return config
}

func (config Config) validate() error {
	if config.RunID == "" || !safeIdentifier(config.RunID) {
		return fmt.Errorf("%w: run ID is required and must contain only letters, numbers, '.', '_', ':', or '-'", contracts.ErrInvalidContract)
	}
	if config.AlertResourceID == "" {
		return fmt.Errorf("%w: SigNoz alert resource ID is required", contracts.ErrInvalidContract)
	}
	if config.Start.IsZero() || config.End.IsZero() || !config.End.After(config.Start) {
		return fmt.Errorf("%w: verification start and end must form a positive window", contracts.ErrInvalidContract)
	}
	if config.FaultInjectedAt.IsZero() || config.FaultInjectedAt.Before(config.Start) || !config.FaultInjectedAt.Before(config.End) {
		return fmt.Errorf("%w: fault injection time must be inside the verification window", contracts.ErrInvalidContract)
	}
	if config.MinimumSamples < 1 || config.PollInterval <= 0 || config.CompletenessTimeout <= 0 || config.QueryTimeout <= 0 {
		return fmt.Errorf("%w: sample count and timeouts must be positive", contracts.ErrInvalidContract)
	}
	return nil
}

func validateChecks(checks []contracts.Requirement) error {
	expected := map[string]string{
		"required-field-cart-value":            "required_field",
		"required-field-error-type":            "required_field",
		"required-operation-payment-authorize": "required_operation",
		"alert-must-fire-payment-timeout":      "alert_must_fire",
	}
	if len(checks) != len(expected) {
		return fmt.Errorf("%w: Phase 4 requires exactly four canonical checks", contracts.ErrInvalidContract)
	}
	for _, check := range checks {
		if expected[check.ID] != check.Type {
			return fmt.Errorf("%w: unsupported Phase 4 check %q", contracts.ErrInvalidContract, check.ID)
		}
		if check.Type == "required_field" &&
			(!strings.Contains(check.Filter, "run.id") || !strings.Contains(check.Filter, "__RUN_ID__")) {
			return fmt.Errorf("%w: check %q is not isolated by the active run ID", contracts.ErrInvalidContract, check.ID)
		}
	}
	return nil
}

func verifyField(ctx context.Context, client signoz.SigNozClient, contract contracts.Contract, requirement contracts.Requirement, config Config) evidence.CheckResult {
	start, minimum := config.Start, config.MinimumSamples
	baseFilter := scopeFilter(contract.Service, config.RunID)
	aggregation := "sum(" + requirement.Field + ")"
	if requirement.Field == "error.type" {
		start, minimum = config.FaultInjectedAt, 1
		baseFilter += " AND name = 'payment.authorize'"
		aggregation = "count()"
	}
	requiredFilter := strings.ReplaceAll(requirement.Filter, "__RUN_ID__", config.RunID)
	description := fmt.Sprintf("SearchTraces for required field %s in the isolated run window", requirement.Field)
	return pollTelemetry(ctx, client, telemetryCheck{
		requirement: requirement, runID: config.RunID, start: start, end: config.End,
		minimum: minimum, baseFilter: baseFilter, requiredFilter: requiredFilter,
		requiredAggregation: aggregation, description: description,
		success: func(value int) bool { return value > 0 },
	}, config)
}

func verifyOperation(ctx context.Context, client signoz.SigNozClient, contract contracts.Contract, requirement contracts.Requirement, config Config) evidence.CheckResult {
	baseFilter := scopeFilter(contract.Service, config.RunID)
	return pollTelemetry(ctx, client, telemetryCheck{
		requirement: requirement, runID: config.RunID, start: config.Start, end: config.End,
		minimum: config.MinimumSamples, baseFilter: baseFilter,
		requiredFilter:      baseFilter + " AND name = '" + requirement.Operation + "'",
		requiredAggregation: "count()",
		description:         "SearchTraces for required operation payment.authorize in the isolated run window",
		success:             func(value int) bool { return value >= config.MinimumSamples },
	}, config)
}

type telemetryCheck struct {
	requirement         contracts.Requirement
	runID               string
	start               time.Time
	end                 time.Time
	minimum             int
	baseFilter          string
	requiredFilter      string
	requiredAggregation string
	description         string
	success             func(int) bool
}

func pollTelemetry(ctx context.Context, client signoz.SigNozClient, check telemetryCheck, config Config) evidence.CheckResult {
	windowEnd := check.end
	deadline := time.Now().Add(config.CompletenessTimeout)
	if windowEnd.Before(deadline) {
		deadline = windowEnd
	}
	baseCount, requiredValue := 0, 0
	for {
		queryEnd := time.Now()
		if windowEnd.Before(queryEnd) {
			queryEnd = windowEnd
		}
		observed := check
		observed.end = queryEnd
		if queryEnd.Sub(check.start) < minimumQueryWindow {
			if !time.Now().Before(deadline) || !windowEnd.After(queryEnd) {
				return telemetryResult(observed, evidence.Inconclusive, 0, evidence.Insufficient, "verification window is shorter than one query step")
			}
			if !wait(ctx, config.PollInterval, deadline) {
				return telemetryResult(observed, evidence.Inconclusive, 0, evidence.Error, "verification was canceled or timed out")
			}
			continue
		}
		var err error
		baseCount, err = traceCount(ctx, client, check.start, queryEnd, check.baseFilter, "count()", config.QueryTimeout)
		if err != nil {
			return telemetryResult(observed, evidence.Inconclusive, 0, evidence.Error, "SigNoz completeness query failed: "+err.Error())
		}
		requiredValue, err = traceCount(ctx, client, check.start, queryEnd, check.requiredFilter, check.requiredAggregation, config.QueryTimeout)
		if err == nil && baseCount >= check.minimum && check.success(requiredValue) {
			return telemetryResult(observed, evidence.Pass, baseCount, evidence.Complete, "requirement observed with sufficient isolated telemetry")
		}
		if errors.Is(err, signoz.ErrMissingField) && baseCount >= check.minimum {
			return telemetryResult(observed, evidence.Fail, baseCount, evidence.Complete, "required field is absent from sufficient isolated telemetry")
		}
		if err != nil {
			return telemetryResult(observed, evidence.Inconclusive, baseCount, evidence.Error, "SigNoz requirement query failed: "+err.Error())
		}
		if !time.Now().Before(deadline) {
			if baseCount < check.minimum {
				return telemetryResult(observed, evidence.Inconclusive, baseCount, evidence.Insufficient, "minimum expected sample count was not reached")
			}
			return telemetryResult(observed, evidence.Fail, baseCount, evidence.Complete, "sufficient isolated telemetry proves the requirement was not observed")
		}
		if !wait(ctx, config.PollInterval, deadline) {
			return telemetryResult(observed, evidence.Inconclusive, baseCount, evidence.Error, "verification was canceled or timed out")
		}
	}
}

func telemetryResult(check telemetryCheck, state evidence.State, samples int, quality evidence.DataQuality, summary string) evidence.CheckResult {
	return evidence.CheckResult{
		State: state, RequirementID: check.requirement.ID, RunID: check.runID,
		AffectedConsumers: append([]string(nil), check.requirement.Consumers...),
		Evidence: evidence.Record{
			Retrieval: check.description, Start: check.start, End: check.end,
			SampleCount: samples, MinimumSampleCount: check.minimum,
			Summary: summary, DataQuality: quality,
		},
	}
}

func verifyAlert(ctx context.Context, client signoz.SigNozClient, contract contracts.Contract, requirement contracts.Requirement, config Config) evidence.CheckResult {
	timeout, _ := time.ParseDuration(requirement.Timeout)
	end := config.FaultInjectedAt.Add(timeout)
	if config.End.Before(end) {
		end = config.End
	}
	result := evidence.CheckResult{
		RequirementID: requirement.ID, RunID: config.RunID,
		AffectedConsumers: append([]string(nil), requirement.Consumers...),
		Evidence: evidence.Record{
			Retrieval: "GetAlertHistory firing events strictly after fault injection for the isolated run",
			Start:     config.FaultInjectedAt, End: end, FaultInjectedAt: &config.FaultInjectedAt,
			MinimumSampleCount: 1,
		},
	}
	alert, err := getAlert(ctx, client, config.AlertResourceID, config.QueryTimeout)
	if err != nil {
		return finishAlert(result, evidence.Inconclusive, 0, evidence.Error, "SigNoz alert retrieval failed", "")
	}
	link := evidence.SafeDeepLink(alert.DeepLink)
	sawStale := false
	for {
		now := time.Now()
		queryEnd := now
		if end.Before(queryEnd) {
			queryEnd = end
		}
		if !queryEnd.After(config.FaultInjectedAt) {
			if !wait(ctx, config.PollInterval, end) {
				return finishAlert(result, evidence.Inconclusive, 0, evidence.Error, "alert observation was canceled or timed out", link)
			}
			continue
		}
		if queryEnd.Sub(config.FaultInjectedAt) < minimumQueryWindow {
			if !end.After(queryEnd) {
				return finishAlert(result, evidence.Inconclusive, 0, evidence.Insufficient, "fault window is shorter than one query step", link)
			}
			if !wait(ctx, config.PollInterval, end) {
				return finishAlert(result, evidence.Inconclusive, 0, evidence.Error, "alert observation was canceled or timed out", link)
			}
			continue
		}
		faultCount, queryErr := traceCount(ctx, client, config.FaultInjectedAt, queryEnd,
			scopeFilter(contract.Service, config.RunID)+" AND name = 'payment.authorize'", "count()", config.QueryTimeout)
		if queryErr != nil {
			return finishAlert(result, evidence.Inconclusive, 0, evidence.Error, "fault telemetry query failed: "+queryErr.Error(), link)
		}
		history, historyErr := alertHistory(ctx, client, config.AlertResourceID, config.FaultInjectedAt, queryEnd, config.QueryTimeout)
		if historyErr != nil {
			return finishAlert(result, evidence.Inconclusive, faultCount, evidence.Error, "SigNoz alert history retrieval failed: "+historyErr.Error(), link)
		}
		fresh := 0
		for _, item := range history.Items {
			eventTime, ok := alertEventTime(item)
			if !ok || !eventTime.After(config.FaultInjectedAt) || eventTime.After(queryEnd) {
				sawStale = true
				continue
			}
			fresh++
			if strings.EqualFold(item.State, "firing") && faultCount >= 1 {
				return finishAlert(result, evidence.Pass, faultCount, evidence.Complete, "firing alert history observed after fault injection", link)
			}
		}
		if !time.Now().Before(end) {
			switch {
			case faultCount < 1:
				return finishAlert(result, evidence.Inconclusive, faultCount, evidence.Insufficient, "fault telemetry was not observed", link)
			case fresh == 0 && sawStale:
				return finishAlert(result, evidence.Inconclusive, faultCount, evidence.Stale, "only stale alert history was returned", link)
			default:
				return finishAlert(result, evidence.Fail, faultCount, evidence.Complete, "bounded observation completed without a firing alert event", link)
			}
		}
		if !wait(ctx, config.PollInterval, end) {
			return finishAlert(result, evidence.Inconclusive, faultCount, evidence.Error, "alert observation was canceled or timed out", link)
		}
	}
}

func finishAlert(result evidence.CheckResult, state evidence.State, samples int, quality evidence.DataQuality, summary, link string) evidence.CheckResult {
	result.State = state
	result.Evidence.SampleCount = samples
	result.Evidence.DataQuality = quality
	result.Evidence.Summary = summary
	result.Evidence.SigNozDeepLink = link
	return result
}

func traceCount(ctx context.Context, client signoz.SigNozClient, start, end time.Time, filter, aggregation string, timeout time.Duration) (int, error) {
	queryContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := client.SearchTraces(queryContext, signoz.SearchRequest{
		Start: start, End: end, Filter: filter,
		Aggregations: []signoz.Aggregation{{Expression: aggregation}},
	})
	if err != nil {
		return 0, err
	}
	total := 0.0
	for _, series := range result.Results {
		for _, aggregation := range series.Aggregations {
			for _, timeSeries := range aggregation.Series {
				for _, point := range timeSeries.Values {
					if math.IsNaN(point.Value) || math.IsInf(point.Value, 0) || point.Value < 0 {
						return 0, errors.New("malformed SigNoz count result")
					}
					total += point.Value
				}
			}
		}
	}
	if total > math.MaxInt {
		return 0, errors.New("SigNoz count result overflow")
	}
	return int(total), nil
}

func getAlert(ctx context.Context, client signoz.SigNozClient, id string, timeout time.Duration) (signoz.Alert, error) {
	queryContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.GetAlert(queryContext, id)
}

func alertHistory(ctx context.Context, client signoz.SigNozClient, id string, start, end time.Time, timeout time.Duration) (signoz.AlertHistory, error) {
	queryContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.GetAlertHistory(queryContext, id, signoz.AlertHistoryRequest{
		Start: start.Add(time.Millisecond), End: end, Limit: 100, Order: "asc", State: "firing",
	})
}

func alertEventTime(item signoz.AlertHistoryItem) (time.Time, bool) {
	if item.Timestamp > 0 {
		return time.UnixMilli(item.Timestamp), true
	}
	if item.CreatedAt == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
	return parsed, err == nil
}

func scopeFilter(service, runID string) string {
	return "service.name = '" + service + "' AND run.id = '" + runID + "'"
}

func safeIdentifier(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return value != ""
}

func wait(ctx context.Context, interval time.Duration, deadline time.Time) bool {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return true
	}
	if interval > remaining {
		interval = remaining
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
