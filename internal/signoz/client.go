package signoz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout     = 10 * time.Second
	maxResponseBytes   = 4 << 20
	defaultQueryName   = "A"
	defaultStepSeconds = 5
)

// SigNozClient is the only boundary used by later Guardian phases for SigNoz access.
type SigNozClient interface {
	GetDashboard(context.Context, string) (Dashboard, error)
	GetAlert(context.Context, string) (Alert, error)
	ExecuteBuilderQuery(context.Context, BuilderQueryRequest) (QueryResult, error)
	SearchTraces(context.Context, SearchRequest) (QueryResult, error)
	SearchLogs(context.Context, SearchRequest) (QueryResult, error)
	GetAlertHistory(context.Context, string, AlertHistoryRequest) (AlertHistory, error)
}

// Config controls the HTTP adapter. Token is never included in an error or log message.
type Config struct {
	BaseURL    string
	Token      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// HTTPClient is the production SigNoz adapter.
type HTTPClient struct {
	baseURL string
	token   string
	timeout time.Duration
	http    *http.Client
}

var _ SigNozClient = (*HTTPClient)(nil)

func NewClient(baseURL, token string) (*HTTPClient, error) {
	return NewHTTPClient(Config{BaseURL: baseURL, Token: token})
}

func NewHTTPClient(config Config) (*HTTPClient, error) {
	base, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" || base.User != nil || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, fmt.Errorf("invalid SigNoz base URL")
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	transport := config.HTTPClient
	if transport == nil {
		transport = &http.Client{}
	} else {
		copy := *transport
		transport = &copy
	}
	if transport.Timeout <= 0 || transport.Timeout > timeout {
		transport.Timeout = timeout
	}
	return &HTTPClient{baseURL: strings.TrimRight(base.String(), "/"), token: config.Token, timeout: timeout, http: transport}, nil
}

type ErrorKind string

const (
	ErrorUnauthorized    ErrorKind = "unauthorized"
	ErrorForbidden       ErrorKind = "forbidden"
	ErrorNotFound        ErrorKind = "not_found"
	ErrorTimeout         ErrorKind = "timeout"
	ErrorInvalidResponse ErrorKind = "invalid_response"
	ErrorInvalidRequest  ErrorKind = "invalid_request"
	ErrorUnexpected      ErrorKind = "unexpected_status"
)

var (
	ErrUnauthorized    = errors.New("signoz unauthorized")
	ErrForbidden       = errors.New("signoz forbidden")
	ErrNotFound        = errors.New("signoz not found")
	ErrTimeout         = errors.New("signoz timeout")
	ErrInvalidResponse = errors.New("signoz invalid response")
	ErrInvalidRequest  = errors.New("signoz invalid request")
	ErrMissingField    = errors.New("signoz field not found")
)

// Error is a stable, secret-safe classification of an adapter failure.
type Error struct {
	Kind         ErrorKind
	StatusCode   int
	Operation    string
	Code         string
	MissingField bool
	cause        error
}

func (e *Error) Error() string {
	if e.Operation == "" {
		return fmt.Sprintf("signoz %s", e.Kind)
	}
	if e.Code == "" {
		return fmt.Sprintf("signoz %s: %s", e.Operation, e.Kind)
	}
	return fmt.Sprintf("signoz %s: %s (%s)", e.Operation, e.Kind, e.Code)
}

func (e *Error) Unwrap() error { return e.cause }

func (e *Error) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.Kind == ErrorUnauthorized
	case ErrForbidden:
		return e.Kind == ErrorForbidden
	case ErrNotFound:
		return e.Kind == ErrorNotFound
	case ErrTimeout:
		return e.Kind == ErrorTimeout
	case ErrInvalidResponse:
		return e.Kind == ErrorInvalidResponse
	case ErrInvalidRequest:
		return e.Kind == ErrorInvalidRequest
	case ErrMissingField:
		return e.MissingField
	default:
		return false
	}
}

type Dashboard struct {
	ID          string
	Title       string
	Description string
	DeepLink    string
	Widgets     []DashboardWidget
}

type DashboardWidget struct {
	ID          string
	Title       string
	Description string
	PanelType   string
	SourcePath  string
	Query       DashboardQuery
}

type DashboardQuery struct {
	QueryType        string
	Builder          BuilderQuery
	UnsupportedNodes []string
}

type BuilderQuery struct {
	QueryData        []QuerySpec
	UnsupportedNodes []string
}

type QuerySpec struct {
	Name              string
	NodeType          string
	Signal            string
	DataSource        string
	AggregateOperator string
	Aggregations      []Aggregation
	Filter            string
	FilterSourcePath  string
	FilterDataType    string
	StepInterval      int
	Disabled          bool
	Legend            string
	SourcePath        string
	FieldDataType     string
	GroupBy           []QueryField
	UnsupportedNodes  []string
}

type Aggregation struct {
	Expression    string
	SourcePath    string
	FieldDataType string
}

type QueryField struct {
	Name          string
	FieldContext  string
	FieldDataType string
}

type Alert struct {
	ID            string
	Name          string
	AlertType     string
	RuleType      string
	State         string
	EvalWindow    string
	Frequency     string
	Version       string
	SchemaVersion string
	DeepLink      string
	Condition     AlertCondition
	Labels        map[string]string
	Annotations   AlertAnnotations
}

type AlertCondition struct {
	CompositeQuery    CompositeQuery
	SelectedQueryName string
	Thresholds        []Threshold
}

type CompositeQuery struct {
	QueryType        string
	PanelType        string
	Queries          []QuerySpec
	UnsupportedNodes []string
}

type Threshold struct {
	Name       string
	Target     float64
	TargetUnit string
	MatchType  string
	Operation  string
	Channels   []string
}

type AlertAnnotations struct {
	Description string
	Summary     string
}

type BuilderQueryRequest struct {
	Start        time.Time
	End          time.Time
	Signal       string
	QueryName    string
	StepInterval int
	Filter       string
	Aggregations []Aggregation
}

type SearchRequest struct {
	Start        time.Time
	End          time.Time
	Filter       string
	QueryName    string
	StepInterval int
	Aggregations []Aggregation
}

type QueryResult struct {
	Type    string
	Meta    QueryMeta
	Results []QuerySeries
	Warning string
}

type QueryMeta struct {
	RowsScanned   int64
	BytesScanned  int64
	DurationMs    int64
	StepIntervals map[string]int
}

type QuerySeries struct {
	QueryName    string
	Aggregations []QueryAggregation
}

type QueryAggregation struct {
	Index  int
	Alias  string
	Series []QueryTimeSeries
}

type QueryTimeSeries struct {
	Values []QueryPoint
}

type QueryPoint struct {
	Timestamp int64
	Value     float64
}

type AlertHistoryRequest struct {
	Start            time.Time
	End              time.Time
	Limit            int
	Order            string
	State            string
	FilterExpression string
	Cursor           string
}

type AlertHistory struct {
	Items      []AlertHistoryItem
	Total      int
	NextCursor string
}

type AlertHistoryItem struct {
	ID        string
	State     string
	Timestamp int64
	CreatedAt string
}

func (c *HTTPClient) GetDashboard(ctx context.Context, id string) (Dashboard, error) {
	if id == "" {
		return Dashboard{}, invalidRequest("GetDashboard", "dashboard id")
	}
	var response dashboardWire
	if err := c.do(ctx, http.MethodGet, "/api/v1/dashboards/"+url.PathEscape(id), nil, "GetDashboard", &response); err != nil {
		return Dashboard{}, err
	}
	return response.dashboard(), nil
}

func (c *HTTPClient) GetAlert(ctx context.Context, id string) (Alert, error) {
	if id == "" {
		return Alert{}, invalidRequest("GetAlert", "alert id")
	}
	var response alertWire
	if err := c.do(ctx, http.MethodGet, "/api/v2/rules/"+url.PathEscape(id), nil, "GetAlert", &response); err != nil {
		return Alert{}, err
	}
	return response.alert(), nil
}

func (c *HTTPClient) ExecuteBuilderQuery(ctx context.Context, request BuilderQueryRequest) (QueryResult, error) {
	wireRequest, err := newQueryRequest(request)
	if err != nil {
		return QueryResult{}, err
	}
	var response queryWire
	if err := c.do(ctx, http.MethodPost, "/api/v5/query_range", wireRequest, "ExecuteBuilderQuery", &response); err != nil {
		return QueryResult{}, err
	}
	return response.result(), nil
}

func (c *HTTPClient) SearchTraces(ctx context.Context, request SearchRequest) (QueryResult, error) {
	return c.ExecuteBuilderQuery(ctx, BuilderQueryRequest{
		Start: request.Start, End: request.End, Signal: "traces", QueryName: request.QueryName,
		StepInterval: request.StepInterval, Filter: request.Filter, Aggregations: request.Aggregations,
	})
}

func (c *HTTPClient) SearchLogs(ctx context.Context, request SearchRequest) (QueryResult, error) {
	return c.ExecuteBuilderQuery(ctx, BuilderQueryRequest{
		Start: request.Start, End: request.End, Signal: "logs", QueryName: request.QueryName,
		StepInterval: request.StepInterval, Filter: request.Filter, Aggregations: request.Aggregations,
	})
}

func (c *HTTPClient) GetAlertHistory(ctx context.Context, id string, request AlertHistoryRequest) (AlertHistory, error) {
	if id == "" {
		return AlertHistory{}, invalidRequest("GetAlertHistory", "alert id")
	}
	if err := validateWindow(request.Start, request.End); err != nil {
		return AlertHistory{}, invalidRequest("GetAlertHistory", err.Error())
	}
	if request.Limit < 0 || request.Limit > 1000 {
		return AlertHistory{}, invalidRequest("GetAlertHistory", "limit")
	}
	values := url.Values{}
	values.Set("start", strconv.FormatInt(request.Start.UnixMilli(), 10))
	values.Set("end", strconv.FormatInt(request.End.UnixMilli(), 10))
	if request.Limit > 0 {
		values.Set("limit", strconv.Itoa(request.Limit))
	}
	if request.Order != "" {
		values.Set("order", request.Order)
	}
	if request.State != "" {
		values.Set("state", request.State)
	}
	if request.FilterExpression != "" {
		values.Set("filterExpression", request.FilterExpression)
	}
	if request.Cursor != "" {
		values.Set("cursor", request.Cursor)
	}
	var response historyWire
	path := "/api/v2/rules/" + url.PathEscape(id) + "/history/timeline?" + values.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, "GetAlertHistory", &response); err != nil {
		return AlertHistory{}, err
	}
	return response.history()
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, operation string, output any) error {
	if ctx == nil {
		return invalidRequest(operation, "nil context")
	}
	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return invalidRequest(operation, "request encoding")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(requestContext, method, c.baseURL+path, reader)
	if err != nil {
		return invalidRequest(operation, "request creation")
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return context.Canceled
		}
		if ctx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) || isNetworkTimeout(err) {
			return timeoutError(operation, err)
		}
		return fmt.Errorf("signoz %s request failed: %w", operation, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return invalidResponse(operation, err)
	}
	if len(payload) > maxResponseBytes {
		return invalidResponse(operation, errors.New("response too large"))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return statusError(operation, response.StatusCode, payload)
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return invalidResponse(operation, err)
	}
	if envelope.Status != "success" || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return invalidResponse(operation, errors.New("missing success data"))
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return invalidResponse(operation, err)
	}
	return nil
}

func newQueryRequest(request BuilderQueryRequest) (queryRequestWire, error) {
	if err := validateWindow(request.Start, request.End); err != nil {
		return queryRequestWire{}, invalidRequest("ExecuteBuilderQuery", err.Error())
	}
	if request.Signal == "" {
		return queryRequestWire{}, invalidRequest("ExecuteBuilderQuery", "signal")
	}
	name := request.QueryName
	if name == "" {
		name = defaultQueryName
	}
	step := request.StepInterval
	if step == 0 {
		step = defaultStepSeconds
	}
	if step < 0 {
		return queryRequestWire{}, invalidRequest("ExecuteBuilderQuery", "step interval")
	}
	aggregations := make([]aggregationWire, 0, len(request.Aggregations))
	for _, aggregation := range request.Aggregations {
		if aggregation.Expression == "" {
			return queryRequestWire{}, invalidRequest("ExecuteBuilderQuery", "aggregation expression")
		}
		aggregations = append(aggregations, aggregationWire{Expression: aggregation.Expression})
	}
	return queryRequestWire{
		SchemaVersion: "v1",
		Start:         request.Start.UnixMilli(),
		End:           request.End.UnixMilli(),
		RequestType:   "time_series",
		CompositeQuery: compositeQueryRequestWire{Queries: []queryRequestItemWire{{
			Type: "builder_query",
			Spec: queryRequestSpecWire{
				Name: name, Signal: request.Signal, StepInterval: step, Disabled: false,
				Filter: filterWire{Expression: request.Filter}, Aggregations: aggregations,
			},
		}}},
		FormatOptions: formatOptionsWire{FormatTableResultForUI: false, FillGaps: false},
	}, nil
}

func validateWindow(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return errors.New("start and end are required")
	}
	if !end.After(start) {
		return errors.New("end must be after start")
	}
	return nil
}

func invalidRequest(operation, code string) error {
	return &Error{Kind: ErrorInvalidRequest, Operation: operation, Code: code, cause: ErrInvalidRequest}
}

func invalidResponse(operation string, cause error) error {
	return &Error{Kind: ErrorInvalidResponse, Operation: operation, cause: cause}
}

func timeoutError(operation string, cause error) error {
	return &Error{Kind: ErrorTimeout, Operation: operation, cause: cause}
}

func statusError(operation string, status int, payload []byte) error {
	code := ""
	missingField := false
	var envelope responseEnvelope
	if json.Unmarshal(payload, &envelope) == nil {
		code = envelope.Error.Code
		if code == "" {
			code = envelope.Error.Type
		}
		missingField = status == http.StatusBadRequest &&
			strings.HasPrefix(envelope.Error.Message, "key ") &&
			strings.HasSuffix(envelope.Error.Message, " not found")
	}
	kind := ErrorUnexpected
	var cause error
	switch status {
	case http.StatusUnauthorized:
		kind, cause = ErrorUnauthorized, ErrUnauthorized
	case http.StatusForbidden:
		kind, cause = ErrorForbidden, ErrForbidden
	case http.StatusNotFound:
		kind, cause = ErrorNotFound, ErrNotFound
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		kind, cause = ErrorTimeout, ErrTimeout
	case http.StatusBadRequest:
		kind, cause = ErrorInvalidRequest, ErrInvalidRequest
	}
	return &Error{Kind: kind, StatusCode: status, Operation: operation, Code: code, MissingField: missingField, cause: cause}
}

func isNetworkTimeout(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

type responseEnvelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  wireAPIError    `json:"error"`
}

type wireAPIError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type dashboardWire struct {
	ID     string               `json:"id"`
	WebURL string               `json:"webUrl"`
	Data   dashboardContentWire `json:"data"`
}

type dashboardContentWire struct {
	Title       string                `json:"title"`
	Description string                `json:"description"`
	WebURL      string                `json:"webUrl"`
	Widgets     []dashboardWidgetWire `json:"widgets"`
}

type dashboardWidgetWire struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	PanelType   string             `json:"panelTypes"`
	Query       dashboardQueryWire `json:"query"`
}

type dashboardQueryWire struct {
	QueryType     string            `json:"queryType"`
	Builder       builderQueryWire  `json:"builder"`
	PromQL        []json.RawMessage `json:"promql"`
	ClickHouseSQL []json.RawMessage `json:"clickhouse_sql"`
}

type builderQueryWire struct {
	QueryData          []querySpecWire   `json:"queryData"`
	QueryFormulas      []json.RawMessage `json:"queryFormulas"`
	QueryTraceOperator []json.RawMessage `json:"queryTraceOperator"`
}

type querySpecWire struct {
	Name              string            `json:"name"`
	QueryName         string            `json:"queryName,omitempty"`
	Signal            string            `json:"signal"`
	DataSource        string            `json:"dataSource,omitempty"`
	AggregateOperator string            `json:"aggregateOperator,omitempty"`
	Aggregations      []aggregationWire `json:"aggregations"`
	Filter            filterWire        `json:"filter"`
	StepInterval      int               `json:"stepInterval"`
	Disabled          bool              `json:"disabled"`
	Legend            string            `json:"legend,omitempty"`
	FieldDataType     string            `json:"fieldDataType,omitempty"`
	GroupBy           []queryFieldWire  `json:"groupBy"`
	OrderBy           []json.RawMessage `json:"orderBy"`
	Having            json.RawMessage   `json:"having"`
	Functions         []json.RawMessage `json:"functions"`
}

type filterWire struct {
	Expression    string `json:"expression"`
	FieldDataType string `json:"fieldDataType,omitempty"`
	DataType      string `json:"dataType,omitempty"`
}

type aggregationWire struct {
	Expression    string `json:"expression"`
	FieldDataType string `json:"fieldDataType,omitempty"`
	DataType      string `json:"dataType,omitempty"`
}

type queryFieldWire struct {
	Name          string `json:"name"`
	FieldContext  string `json:"fieldContext"`
	FieldDataType string `json:"fieldDataType"`
}

func (wire dashboardWire) dashboard() Dashboard {
	widgets := make([]DashboardWidget, 0, len(wire.Data.Widgets))
	for index, widget := range wire.Data.Widgets {
		widgetPath := fmt.Sprintf("$.data.data.widgets[%d]", index)
		builderPath := widgetPath + ".query.builder"
		builder := mapBuilderQuery(widget.Query.Builder, builderPath)
		unsupported := make([]string, 0, 2)
		if len(widget.Query.PromQL) > 0 {
			unsupported = append(unsupported, "promql")
		}
		if len(widget.Query.ClickHouseSQL) > 0 {
			unsupported = append(unsupported, "clickhouse_sql")
		}
		unsupported = append(unsupported, builder.UnsupportedNodes...)
		widgets = append(widgets, DashboardWidget{
			ID: widget.ID, Title: widget.Title, Description: widget.Description, PanelType: widget.PanelType,
			SourcePath: widgetPath,
			Query:      DashboardQuery{QueryType: widget.Query.QueryType, Builder: builder, UnsupportedNodes: uniqueStrings(unsupported)},
		})
	}
	deepLink := wire.WebURL
	if deepLink == "" {
		deepLink = wire.Data.WebURL
	}
	return Dashboard{ID: wire.ID, Title: wire.Data.Title, Description: wire.Data.Description, DeepLink: deepLink, Widgets: widgets}
}

type alertWire struct {
	ID            string            `json:"id"`
	Alert         string            `json:"alert"`
	AlertType     string            `json:"alertType"`
	RuleType      string            `json:"ruleType"`
	State         string            `json:"state"`
	EvalWindow    string            `json:"evalWindow"`
	Frequency     string            `json:"frequency"`
	Version       string            `json:"version"`
	SchemaVersion string            `json:"schemaVersion"`
	WebURL        string            `json:"webUrl"`
	Condition     conditionWire     `json:"condition"`
	Labels        map[string]string `json:"labels"`
	Annotations   AlertAnnotations  `json:"annotations"`
}

type conditionWire struct {
	CompositeQuery    compositeQueryWire `json:"compositeQuery"`
	SelectedQueryName string             `json:"selectedQueryName"`
	Thresholds        thresholdSetWire   `json:"thresholds"`
}

type compositeQueryWire struct {
	QueryType string           `json:"queryType"`
	PanelType string           `json:"panelType"`
	Queries   []alertQueryWire `json:"queries"`
}

type alertQueryWire struct {
	Type string        `json:"type"`
	Spec querySpecWire `json:"spec"`
}

type thresholdSetWire struct {
	Spec []thresholdWire `json:"spec"`
}

type thresholdWire struct {
	Name       string   `json:"name"`
	Target     float64  `json:"target"`
	TargetUnit string   `json:"targetUnit"`
	MatchType  string   `json:"matchType"`
	Operation  string   `json:"op"`
	Channels   []string `json:"channels"`
}

func (wire alertWire) alert() Alert {
	queries := make([]QuerySpec, 0, len(wire.Condition.CompositeQuery.Queries))
	for index, query := range wire.Condition.CompositeQuery.Queries {
		path := fmt.Sprintf("$.data.condition.compositeQuery.queries[%d].spec", index)
		queries = append(queries, mapQuerySpecAt(query.Spec, path, query.Type))
	}
	thresholds := make([]Threshold, 0, len(wire.Condition.Thresholds.Spec))
	for _, threshold := range wire.Condition.Thresholds.Spec {
		thresholds = append(thresholds, Threshold{
			Name: threshold.Name, Target: threshold.Target, TargetUnit: threshold.TargetUnit,
			MatchType: threshold.MatchType, Operation: threshold.Operation, Channels: threshold.Channels,
		})
	}
	return Alert{
		ID: wire.ID, Name: wire.Alert, AlertType: wire.AlertType, RuleType: wire.RuleType, State: wire.State,
		EvalWindow: wire.EvalWindow, Frequency: wire.Frequency, Version: wire.Version, SchemaVersion: wire.SchemaVersion,
		DeepLink: wire.WebURL, Condition: AlertCondition{
			CompositeQuery:    CompositeQuery{QueryType: wire.Condition.CompositeQuery.QueryType, PanelType: wire.Condition.CompositeQuery.PanelType, Queries: queries},
			SelectedQueryName: wire.Condition.SelectedQueryName, Thresholds: thresholds,
		}, Labels: wire.Labels, Annotations: wire.Annotations,
	}
}

func mapQuerySpecs(specs []querySpecWire) []QuerySpec {
	return mapQuerySpecsAt(specs, "", "")
}

func mapQuerySpecsAt(specs []querySpecWire, basePath, nodeType string) []QuerySpec {
	result := make([]QuerySpec, 0, len(specs))
	for index, spec := range specs {
		path := ""
		if basePath != "" {
			path = fmt.Sprintf("%s[%d]", basePath, index)
		}
		result = append(result, mapQuerySpecAt(spec, path, nodeType))
	}
	return result
}

func mapQuerySpec(spec querySpecWire) QuerySpec {
	return mapQuerySpecAt(spec, "", "")
}

func mapQuerySpecAt(spec querySpecWire, sourcePath, nodeType string) QuerySpec {
	name := spec.Name
	if name == "" {
		name = spec.QueryName
	}
	aggregations := make([]Aggregation, 0, len(spec.Aggregations))
	for index, aggregation := range spec.Aggregations {
		aggregationPath := ""
		if sourcePath != "" {
			aggregationPath = fmt.Sprintf("%s.aggregations[%d].expression", sourcePath, index)
		}
		fieldDataType := aggregation.FieldDataType
		if fieldDataType == "" {
			fieldDataType = aggregation.DataType
		}
		aggregations = append(aggregations, Aggregation{Expression: aggregation.Expression, SourcePath: aggregationPath, FieldDataType: fieldDataType})
	}
	filterPath := ""
	if sourcePath != "" {
		filterPath = sourcePath + ".filter.expression"
	}
	groupBy := make([]QueryField, 0, len(spec.GroupBy))
	for _, field := range spec.GroupBy {
		groupBy = append(groupBy, QueryField{Name: field.Name, FieldContext: field.FieldContext, FieldDataType: field.FieldDataType})
	}
	unsupported := make([]string, 0, 3)
	if len(spec.GroupBy) > 0 {
		unsupported = append(unsupported, "groupBy")
	}
	if len(spec.OrderBy) > 0 {
		unsupported = append(unsupported, "orderBy")
	}
	if hasJSONContent(spec.Having) {
		unsupported = append(unsupported, "having")
	}
	if len(spec.Functions) > 0 {
		unsupported = append(unsupported, "functions")
	}
	return QuerySpec{
		Name: name, NodeType: nodeType, Signal: spec.Signal, DataSource: spec.DataSource, AggregateOperator: spec.AggregateOperator,
		Aggregations: aggregations, Filter: spec.Filter.Expression, FilterSourcePath: filterPath, FilterDataType: firstNonEmpty(spec.Filter.FieldDataType, spec.Filter.DataType), StepInterval: spec.StepInterval,
		Disabled: spec.Disabled, Legend: spec.Legend, SourcePath: sourcePath, FieldDataType: spec.FieldDataType,
		GroupBy: groupBy, UnsupportedNodes: unsupported,
	}
}

func hasJSONContent(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	switch typed := value.(type) {
	case nil:
		return false
	case []any:
		return len(typed) > 0
	case map[string]any:
		for _, item := range typed {
			if item != nil && item != "" {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func mapBuilderQuery(wire builderQueryWire, basePath string) BuilderQuery {
	unsupported := make([]string, 0, 2)
	if len(wire.QueryFormulas) > 0 {
		unsupported = append(unsupported, "queryFormulas")
	}
	if len(wire.QueryTraceOperator) > 0 {
		unsupported = append(unsupported, "queryTraceOperator")
	}
	return BuilderQuery{
		QueryData:        mapQuerySpecsAt(wire.QueryData, basePath+".queryData", "builder_query"),
		UnsupportedNodes: uniqueStrings(unsupported),
	}
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type queryRequestWire struct {
	SchemaVersion  string                    `json:"schemaVersion"`
	Start          int64                     `json:"start"`
	End            int64                     `json:"end"`
	RequestType    string                    `json:"requestType"`
	CompositeQuery compositeQueryRequestWire `json:"compositeQuery"`
	FormatOptions  formatOptionsWire         `json:"formatOptions"`
}

type compositeQueryRequestWire struct {
	Queries []queryRequestItemWire `json:"queries"`
}

type queryRequestItemWire struct {
	Type string               `json:"type"`
	Spec queryRequestSpecWire `json:"spec"`
}

type queryRequestSpecWire struct {
	Name         string            `json:"name"`
	Signal       string            `json:"signal"`
	Aggregations []aggregationWire `json:"aggregations"`
	Filter       filterWire        `json:"filter"`
	StepInterval int               `json:"stepInterval"`
	Disabled     bool              `json:"disabled"`
}

type formatOptionsWire struct {
	FormatTableResultForUI bool `json:"formatTableResultForUI"`
	FillGaps               bool `json:"fillGaps"`
}

type queryWire struct {
	Type    string          `json:"type"`
	Meta    queryMetaWire   `json:"meta"`
	Data    queryDataWire   `json:"data"`
	Warning json.RawMessage `json:"warning"`
}

type queryMetaWire struct {
	RowsScanned   int64          `json:"rowsScanned"`
	BytesScanned  int64          `json:"bytesScanned"`
	DurationMs    int64          `json:"durationMs"`
	StepIntervals map[string]int `json:"stepIntervals"`
}

type queryDataWire struct {
	Results []querySeriesWire `json:"results"`
}

type querySeriesWire struct {
	QueryName    string                 `json:"queryName"`
	Aggregations []queryAggregationWire `json:"aggregations"`
}

type queryAggregationWire struct {
	Index  int                   `json:"index"`
	Alias  string                `json:"alias"`
	Series []queryTimeSeriesWire `json:"series"`
}

type queryTimeSeriesWire struct {
	Values []queryPointWire `json:"values"`
}

type queryPointWire struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

func (wire queryWire) result() QueryResult {
	results := make([]QuerySeries, 0, len(wire.Data.Results))
	for _, result := range wire.Data.Results {
		aggregations := make([]QueryAggregation, 0, len(result.Aggregations))
		for _, aggregation := range result.Aggregations {
			series := make([]QueryTimeSeries, 0, len(aggregation.Series))
			for _, points := range aggregation.Series {
				values := make([]QueryPoint, 0, len(points.Values))
				for _, point := range points.Values {
					values = append(values, QueryPoint{Timestamp: point.Timestamp, Value: point.Value})
				}
				series = append(series, QueryTimeSeries{Values: values})
			}
			aggregations = append(aggregations, QueryAggregation{Index: aggregation.Index, Alias: aggregation.Alias, Series: series})
		}
		results = append(results, QuerySeries{QueryName: result.QueryName, Aggregations: aggregations})
	}
	return QueryResult{
		Type: wire.Type, Meta: QueryMeta{RowsScanned: wire.Meta.RowsScanned, BytesScanned: wire.Meta.BytesScanned,
			DurationMs: wire.Meta.DurationMs, StepIntervals: wire.Meta.StepIntervals}, Results: results, Warning: warningText(wire.Warning),
	}
}

func warningText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var object struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return object.Message
	}
	return ""
}

type historyWire struct {
	Items      []historyItemWire `json:"items"`
	Total      int               `json:"total"`
	NextCursor string            `json:"nextCursor"`
	Data       *historyDataWire  `json:"data"`
}

type historyDataWire struct {
	Items      []historyItemWire `json:"items"`
	Total      int               `json:"total"`
	NextCursor string            `json:"nextCursor"`
}

type historyItemWire struct {
	ID        string          `json:"id"`
	RuleID    string          `json:"ruleId"`
	State     string          `json:"state"`
	Timestamp json.RawMessage `json:"timestamp"`
	UnixMilli json.RawMessage `json:"unixMilli"`
	CreatedAt string          `json:"createdAt"`
}

func (wire historyWire) history() (AlertHistory, error) {
	data := historyDataWire{Items: wire.Items, Total: wire.Total, NextCursor: wire.NextCursor}
	if wire.Data != nil {
		data = *wire.Data
	}
	items := make([]AlertHistoryItem, 0, len(data.Items))
	for _, item := range data.Items {
		timestampRaw := item.Timestamp
		if len(timestampRaw) == 0 || string(timestampRaw) == "null" {
			timestampRaw = item.UnixMilli
		}
		timestamp, err := parseTimestamp(timestampRaw)
		if err != nil {
			return AlertHistory{}, invalidResponse("GetAlertHistory", err)
		}
		id := item.ID
		if id == "" {
			id = item.RuleID
		}
		items = append(items, AlertHistoryItem{ID: id, State: item.State, Timestamp: timestamp, CreatedAt: item.CreatedAt})
	}
	return AlertHistory{Items: items, Total: data.Total, NextCursor: data.NextCursor}, nil
}

func parseTimestamp(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var number int64
	if json.Unmarshal(raw, &number) == nil {
		return number, nil
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, errors.New("invalid history timestamp")
	}
	if number, err := strconv.ParseInt(text, 10, 64); err == nil {
		return number, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return 0, errors.New("invalid history timestamp")
	}
	return parsed.UnixMilli(), nil
}
