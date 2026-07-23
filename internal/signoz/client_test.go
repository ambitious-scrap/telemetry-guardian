package signoz

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFixtureFakeImplementsContract(t *testing.T) {
	fake, err := NewFixtureFake()
	if err != nil {
		t.Fatal(err)
	}

	dashboard, err := fake.GetDashboard(context.Background(), "dashboard-fixture-id")
	if err != nil || dashboard.ID != "dashboard-fixture-id" || dashboard.DeepLink == "" || len(dashboard.Widgets) != 1 {
		t.Fatalf("dashboard = %#v, err = %v", dashboard, err)
	}
	alert, err := fake.GetAlert(context.Background(), "alert-fixture-id")
	if err != nil || alert.ID != "alert-fixture-id" || alert.Condition.CompositeQuery.Queries[0].Signal != "traces" {
		t.Fatalf("alert = %#v, err = %v", alert, err)
	}
	start := time.UnixMilli(1700000000000)
	query, err := fake.ExecuteBuilderQuery(context.Background(), BuilderQueryRequest{Start: start, End: start.Add(time.Minute), Signal: "traces", Aggregations: []Aggregation{{Expression: "count()"}}})
	if err != nil || len(query.Results) != 1 || len(query.Results[0].Aggregations) != 1 {
		t.Fatalf("query = %#v, err = %v", query, err)
	}
	logs, err := fake.SearchLogs(context.Background(), SearchRequest{Start: start, End: start.Add(time.Minute), Aggregations: []Aggregation{{Expression: "count()"}}})
	if err != nil || len(logs.Results) != 1 || len(logs.Results[0].Aggregations) != 0 {
		t.Fatalf("valid empty logs = %#v, err = %v", logs, err)
	}
	history, err := fake.GetAlertHistory(context.Background(), "alert-fixture-id", AlertHistoryRequest{Start: start, End: start.Add(time.Minute)})
	if err != nil || history.Total != 0 || len(history.Items) != 0 {
		t.Fatalf("empty history = %#v, err = %v", history, err)
	}
	if _, err := fake.GetDashboard(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fake missing dashboard error = %v", err)
	}
}

func TestHTTPClientRetrievesTypedResourcesAndExecutesBuilderQuery(t *testing.T) {
	seenSignals := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Errorf("authorization header was not set")
		}
		switch {
		case request.URL.Path == "/api/v1/dashboards/dashboard-fixture-id":
			writeFixture(t, response, "testdata/dashboard-success.json")
		case request.URL.Path == "/api/v2/rules/alert-fixture-id":
			writeFixture(t, response, "testdata/alert-success.json")
		case request.URL.Path == "/api/v5/query_range":
			var requestBody struct {
				Start          int64 `json:"start"`
				End            int64 `json:"end"`
				CompositeQuery struct {
					Queries []struct {
						Spec struct {
							Signal string `json:"signal"`
							Filter struct {
								Expression string `json:"expression"`
							} `json:"filter"`
						} `json:"spec"`
					} `json:"queries"`
				} `json:"compositeQuery"`
			}
			if err := jsonDecode(request, &requestBody); err != nil {
				t.Errorf("decode query request: %v", err)
			}
			if requestBody.Start == 0 || requestBody.End <= requestBody.Start || requestBody.CompositeQuery.Queries[0].Spec.Filter.Expression != "service.name = 'checkout'" {
				t.Errorf("unexpected query request: %#v", requestBody)
			}
			seenSignals = append(seenSignals, requestBody.CompositeQuery.Queries[0].Spec.Signal)
			writeFixture(t, response, "testdata/query-success.json")
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewHTTPClient(Config{BaseURL: server.URL, Token: "fixture-token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	windowStart := time.UnixMilli(1700000000000)
	windowEnd := windowStart.Add(time.Minute)

	dashboard, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
	if err != nil || dashboard.DeepLink != "https://signoz.example/dashboards/dashboard-fixture-id" {
		t.Fatalf("dashboard = %#v, err = %v", dashboard, err)
	}
	alert, err := client.GetAlert(context.Background(), "alert-fixture-id")
	if err != nil || alert.DeepLink != "https://signoz.example/alerts/alert-fixture-id" {
		t.Fatalf("alert = %#v, err = %v", alert, err)
	}
	query, err := client.ExecuteBuilderQuery(context.Background(), BuilderQueryRequest{
		Start: windowStart, End: windowEnd, Signal: "traces", Filter: "service.name = 'checkout'",
		Aggregations: []Aggregation{{Expression: "count()"}},
	})
	if err != nil || query.Results[0].Aggregations[0].Series[0].Values[0].Value != 1 {
		t.Fatalf("query = %#v, err = %v", query, err)
	}
	if _, err := client.SearchTraces(context.Background(), SearchRequest{Start: windowStart, End: windowEnd, Filter: "service.name = 'checkout'"}); err != nil {
		t.Fatal(err)
	}
	if len(seenSignals) != 2 || seenSignals[0] != "traces" || seenSignals[1] != "traces" {
		t.Fatalf("signals = %#v", seenSignals)
	}
}

func TestHTTPClientAlertHistoryPaginationAndEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("start") != "1700000000000" || request.URL.Query().Get("end") != "1700000060000" || request.URL.Query().Get("limit") != "1" {
			t.Errorf("history query = %s", request.URL.RawQuery)
		}
		if request.URL.Query().Get("cursor") == "history-cursor-2" {
			writeFixture(t, response, "testdata/history-page-2.json")
			return
		}
		if request.URL.Query().Get("cursor") != "" || request.URL.Query().Get("order") != "desc" || request.URL.Query().Get("state") != "all" || request.URL.Query().Get("filterExpression") != "state = 'firing'" {
			t.Errorf("history pagination query = %s", request.URL.RawQuery)
		}
		writeFixture(t, response, "testdata/history-page-1.json")
	}))
	defer server.Close()
	client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	start := time.UnixMilli(1700000000000)
	end := time.UnixMilli(1700000060000)
	request := AlertHistoryRequest{Start: start, End: end, Limit: 1, Order: "desc", State: "all", FilterExpression: "state = 'firing'"}
	first, err := client.GetAlertHistory(context.Background(), "alert-fixture-id", request)
	if err != nil || first.Total != 2 || first.NextCursor != "history-cursor-2" || first.Items[0].Timestamp != 1700000000000 {
		t.Fatalf("first history = %#v, err = %v", first, err)
	}
	request.Cursor = first.NextCursor
	second, err := client.GetAlertHistory(context.Background(), "alert-fixture-id", request)
	if err != nil || second.Total != 2 || second.Items[0].ID != "history-event-2" || second.Items[0].Timestamp != 1700000060000 {
		t.Fatalf("second history = %#v, err = %v", second, err)
	}
}

func TestHTTPClientTypedErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		fixture string
		want    error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, fixture: "testdata/unauthorized-response.json", want: ErrUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, fixture: "testdata/forbidden-response.json", want: ErrForbidden},
		{name: "not found", status: http.StatusNotFound, fixture: "testdata/not-found-response.json", want: ErrNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
				writeFixture(t, response, test.fixture)
			}))
			defer server.Close()
			client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetDashboard(context.Background(), "dashboard-fixture-id")
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	t.Run("malformed response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			writeFixture(t, response, "testdata/malformed-response.json")
		}))
		defer server.Close()
		client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.GetDashboard(context.Background(), "dashboard-fixture-id")
		if !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			writeFixture(t, response, "testdata/dashboard-success.json")
		}))
		defer server.Close()
		client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: 10 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.GetDashboard(context.Background(), "dashboard-fixture-id")
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("cancellation propagates", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		defer server.Close()
		client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = client.GetDashboard(ctx, "dashboard-fixture-id")
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("secret is not copied into errors", func(t *testing.T) {
		const secret = "fixture-secret-token"
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(response, `{"status":"error","error":{"code":"invalid_input","message":"`+secret+`"}}`)
		}))
		defer server.Close()
		client, err := NewHTTPClient(Config{BaseURL: server.URL, Token: secret, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.GetDashboard(context.Background(), "dashboard-fixture-id")
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked secret: %v", err)
		}
	})
}

func TestHTTPClientRejectsInvalidRequests(t *testing.T) {
	client, err := NewHTTPClient(Config{BaseURL: "http://127.0.0.1:1", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ExecuteBuilderQuery(context.Background(), BuilderQueryRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid query error = %v", err)
	}
	if _, err := client.GetAlertHistory(context.Background(), "alert-fixture-id", AlertHistoryRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid history error = %v", err)
	}
}

func writeFixture(t *testing.T, response http.ResponseWriter, name string) {
	t.Helper()
	payload, err := fixtureFiles.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	response.Header().Set("Content-Type", "application/json")
	_, _ = response.Write(payload)
}

func jsonDecode(request *http.Request, output any) error {
	return json.NewDecoder(request.Body).Decode(output)
}
