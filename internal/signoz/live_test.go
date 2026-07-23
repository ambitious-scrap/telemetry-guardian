package signoz

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveSigNozAdapter(t *testing.T) {
	baseURL := os.Getenv("SIGNOZ_URL")
	token := os.Getenv("SIGNOZ_TOKEN")
	dashboardID := os.Getenv("SIGNOZ_DASHBOARD_ID")
	alertID := os.Getenv("SIGNOZ_ALERT_ID")
	if baseURL == "" || token == "" || dashboardID == "" || alertID == "" {
		t.Skip("set SIGNOZ_URL, SIGNOZ_TOKEN, SIGNOZ_DASHBOARD_ID, and SIGNOZ_ALERT_ID for live integration")
	}
	client, err := NewHTTPClient(Config{BaseURL: baseURL, Token: token, Timeout: 15 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dashboard, err := client.GetDashboard(ctx, dashboardID)
	if err != nil || dashboard.ID != dashboardID || len(dashboard.Widgets) == 0 {
		t.Fatalf("live dashboard = %#v, err = %v", dashboard, err)
	}
	alert, err := client.GetAlert(ctx, alertID)
	if err != nil || alert.ID != alertID || alert.Condition.CompositeQuery.QueryType != "builder" {
		t.Fatalf("live alert = %#v, err = %v", alert, err)
	}

	end := time.Now()
	start := end.Add(-30 * time.Minute)
	request := BuilderQueryRequest{
		Start: start, End: end, Signal: "traces", Filter: "service.name = 'telemetry-guardian-checkout'",
		Aggregations: []Aggregation{{Expression: "count()"}},
	}
	if _, err := client.ExecuteBuilderQuery(ctx, request); err != nil {
		t.Fatalf("live builder query: %v", err)
	}
	if _, err := client.SearchTraces(ctx, SearchRequest{Start: start, End: end, Filter: request.Filter, Aggregations: request.Aggregations}); err != nil {
		t.Fatalf("live trace search: %v", err)
	}
	if _, err := client.SearchLogs(ctx, SearchRequest{Start: start, End: end, Filter: request.Filter, Aggregations: request.Aggregations}); err != nil {
		t.Fatalf("live log search: %v", err)
	}
	history, err := client.GetAlertHistory(ctx, alertID, AlertHistoryRequest{Start: start, End: end, Limit: 100, Order: "desc", State: "all"})
	if err != nil {
		t.Fatalf("live alert history: %v", err)
	}
	if history.Total < len(history.Items) {
		t.Fatalf("live history total %d is smaller than page size %d", history.Total, len(history.Items))
	}
}
