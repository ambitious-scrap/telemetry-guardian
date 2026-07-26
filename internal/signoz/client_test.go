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

func TestHTTPClientAlertHistoryLiveEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeFixture(t, response, "testdata/history-live-envelope.json")
	}))
	defer server.Close()
	client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	start := time.UnixMilli(1700000000000)
	history, err := client.GetAlertHistory(context.Background(), "alert-fixture-id", AlertHistoryRequest{
		Start: start.Add(-time.Minute),
		End:   start.Add(time.Minute),
		State: "firing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if history.Total != 1 || len(history.Items) != 1 {
		t.Fatalf("history = %#v", history)
	}
	item := history.Items[0]
	if item.ID != "alert-fixture-id" || item.State != "firing" || item.Timestamp != start.UnixMilli() {
		t.Fatalf("history item = %#v", item)
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

func TestMissingFieldClassificationIsSecretSafe(t *testing.T) {
	err := statusError("SearchTraces", http.StatusBadRequest, []byte(`{"error":{"code":"invalid_input","message":"key cart.value not found"}}`))
	if !errors.Is(err, ErrInvalidRequest) || !errors.Is(err, ErrMissingField) {
		t.Fatalf("classification = %v", err)
	}
	if strings.Contains(err.Error(), "cart.value") {
		t.Fatalf("error exposed server detail: %v", err)
	}
}

func TestDashboardAcceptsEmptyObjectHaving(t *testing.T) {
	var wire dashboardWire
	payload := `{"id":"dashboard","data":{"title":"Checkout","widgets":[{"id":"panel","title":"Cart","panelTypes":"graph","query":{"queryType":"builder","builder":{"queryData":[{"queryName":"A","dataSource":"traces","aggregations":[{"expression":"sum(cart.value)"}],"filter":{"expression":"service.name = 'checkout'"},"having":{"expression":""}}]}}}]}}`
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		t.Fatal(err)
	}
	dashboard := wire.dashboard()
	if len(dashboard.Widgets) != 1 || len(dashboard.Widgets[0].Query.UnsupportedNodes) != 0 {
		t.Fatalf("dashboard = %#v", dashboard)
	}
}

func TestHTTPClientStrictlyDecodesDependencyQueries(t *testing.T) {
	readPayload := func(t *testing.T, name string) map[string]any {
		t.Helper()
		payload, err := fixtureFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(payload, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	serve := func(t *testing.T, value map[string]any) *HTTPClient {
		t.Helper()
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(payload)
		}))
		t.Cleanup(server.Close)
		client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}

	t.Run("unknown dashboard query key is invalid response", func(t *testing.T) {
		value := readPayload(t, "testdata/dashboard-success.json")
		data := value["data"].(map[string]any)
		dashboard := data["data"].(map[string]any)
		widget := dashboard["widgets"].([]any)[0].(map[string]any)
		query := widget["query"].(map[string]any)
		query["unknownQueryNode"] = true
		client := serve(t, value)
		_, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
		if !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "query") {
			t.Fatalf("error = %v, want typed query invalid response", err)
		}
		if strings.Contains(err.Error(), "unknownQueryNode") {
			t.Fatalf("error exposed raw query content: %v", err)
		}
	})

	t.Run("unknown alert query key is invalid response", func(t *testing.T) {
		value := readPayload(t, "testdata/alert-success.json")
		data := value["data"].(map[string]any)
		condition := data["condition"].(map[string]any)
		composite := condition["compositeQuery"].(map[string]any)
		query := composite["queries"].([]any)[0].(map[string]any)
		spec := query["spec"].(map[string]any)
		spec["unknownDependencyNode"] = true
		client := serve(t, value)
		_, err := client.GetAlert(context.Background(), "alert-fixture-id")
		if !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "query") {
			t.Fatalf("error = %v, want typed query invalid response", err)
		}
	})

	t.Run("unknown response metadata remains accepted", func(t *testing.T) {
		value := readPayload(t, "testdata/dashboard-success.json")
		value["futureMetadata"] = map[string]any{"provider": "signoz"}
		client := serve(t, value)
		dashboard, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
		if err != nil || dashboard.ID != "dashboard-fixture-id" {
			t.Fatalf("dashboard = %#v, err = %v", dashboard, err)
		}
	})
}

func TestHTTPClientRejectsSemanticallyMalformedSuccessResources(t *testing.T) {
	readPayload := func(t *testing.T, name string) map[string]any {
		t.Helper()
		payload, err := fixtureFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(payload, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	serve := func(t *testing.T, value map[string]any) *HTTPClient {
		t.Helper()
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(payload)
		}))
		t.Cleanup(server.Close)
		client, err := NewHTTPClient(Config{BaseURL: server.URL, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}

	tests := []struct {
		name    string
		fixture string
		mutate  func(map[string]any)
		get     func(*HTTPClient) error
	}{
		{name: "missing dashboard id", fixture: "testdata/dashboard-success.json", mutate: func(value map[string]any) { delete(value["data"].(map[string]any), "id") }, get: func(client *HTTPClient) error {
			_, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
			return err
		}},
		{name: "missing dashboard title", fixture: "testdata/dashboard-success.json", mutate: func(value map[string]any) { delete(value["data"].(map[string]any)["data"].(map[string]any), "title") }, get: func(client *HTTPClient) error {
			_, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
			return err
		}},
		{name: "missing dashboard widgets", fixture: "testdata/dashboard-success.json", mutate: func(value map[string]any) { delete(value["data"].(map[string]any)["data"].(map[string]any), "widgets") }, get: func(client *HTTPClient) error {
			_, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
			return err
		}},
		{name: "missing widget identity", fixture: "testdata/dashboard-success.json", mutate: func(value map[string]any) {
			widget := value["data"].(map[string]any)["data"].(map[string]any)["widgets"].([]any)[0].(map[string]any)
			delete(widget, "id")
		}, get: func(client *HTTPClient) error {
			_, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
			return err
		}},
		{name: "missing widget query", fixture: "testdata/dashboard-success.json", mutate: func(value map[string]any) {
			widget := value["data"].(map[string]any)["data"].(map[string]any)["widgets"].([]any)[0].(map[string]any)
			delete(widget, "query")
		}, get: func(client *HTTPClient) error {
			_, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
			return err
		}},
		{name: "missing supported dashboard query", fixture: "testdata/dashboard-success.json", mutate: func(value map[string]any) {
			builder := value["data"].(map[string]any)["data"].(map[string]any)["widgets"].([]any)[0].(map[string]any)["query"].(map[string]any)["builder"].(map[string]any)
			builder["queryData"] = []any{}
		}, get: func(client *HTTPClient) error {
			_, err := client.GetDashboard(context.Background(), "dashboard-fixture-id")
			return err
		}},
		{name: "missing alert id", fixture: "testdata/alert-success.json", mutate: func(value map[string]any) { delete(value["data"].(map[string]any), "id") }, get: func(client *HTTPClient) error {
			_, err := client.GetAlert(context.Background(), "alert-fixture-id")
			return err
		}},
		{name: "missing alert name", fixture: "testdata/alert-success.json", mutate: func(value map[string]any) { delete(value["data"].(map[string]any), "alert") }, get: func(client *HTTPClient) error {
			_, err := client.GetAlert(context.Background(), "alert-fixture-id")
			return err
		}},
		{name: "missing composite query", fixture: "testdata/alert-success.json", mutate: func(value map[string]any) {
			delete(value["data"].(map[string]any)["condition"].(map[string]any), "compositeQuery")
		}, get: func(client *HTTPClient) error {
			_, err := client.GetAlert(context.Background(), "alert-fixture-id")
			return err
		}},
		{name: "missing supported alert query", fixture: "testdata/alert-success.json", mutate: func(value map[string]any) {
			composite := value["data"].(map[string]any)["condition"].(map[string]any)["compositeQuery"].(map[string]any)
			composite["queries"] = []any{}
		}, get: func(client *HTTPClient) error {
			_, err := client.GetAlert(context.Background(), "alert-fixture-id")
			return err
		}},
		{name: "missing alert thresholds", fixture: "testdata/alert-success.json", mutate: func(value map[string]any) {
			condition := value["data"].(map[string]any)["condition"].(map[string]any)
			condition["thresholds"] = map[string]any{"spec": []any{}}
		}, get: func(client *HTTPClient) error {
			_, err := client.GetAlert(context.Background(), "alert-fixture-id")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := readPayload(t, test.fixture)
			test.mutate(value)
			if err := test.get(serve(t, value)); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error = %v, want typed invalid response", err)
			}
		})
	}
}

func TestQueryRequestOmitsResponseOnlyNodes(t *testing.T) {
	request, err := newQueryRequest(BuilderQueryRequest{
		Start: time.Now().Add(-time.Minute), End: time.Now(), Signal: "traces",
		Filter: "service.name = 'checkout'", Aggregations: []Aggregation{{Expression: "count()"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"orderBy"`, `"having"`, `"functions"`, `"groupBy"`} {
		if strings.Contains(string(payload), field) {
			t.Fatalf("request contains response-only field %s: %s", field, payload)
		}
	}
}

func TestQueryWarningAcceptsStringOrObject(t *testing.T) {
	for input, expected := range map[string]string{
		`"legacy warning"`:            "legacy warning",
		`{"message":"typed warning"}`: "typed warning",
	} {
		var wire queryWire
		if err := json.Unmarshal([]byte(`{"warning":`+input+`,"data":{"results":[]}}`), &wire); err != nil {
			t.Fatal(err)
		}
		if warning := wire.result().Warning; warning != expected {
			t.Fatalf("warning = %q, want %q", warning, expected)
		}
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
