package miner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/signoz"
)

const (
	DefaultService        = "checkout"
	DefaultRelease        = "candidate"
	RequiredCartValue     = "cart.value"
	RequiredErrorType     = "error.type"
	RequiredOperation     = "payment.authorize"
	RequiredAlertID       = "payment-timeout"
	AlertCheckTimeout     = "60s"
	DashboardPanelType    = "dashboard_panel"
	AlertConsumerType     = "alert"
	RequiredFieldType     = "required_field"
	RequiredOperationType = "required_operation"
	AlertMustFireType     = "alert_must_fire"
)

var (
	ErrInvalidInput = errors.New("invalid miner input")
	ErrUnsupported  = errors.New("unsupported miner input")
)

type Config struct {
	DashboardID string
	AlertID     string
	Service     string
	Release     string
}

type Error struct {
	Kind    error
	Path    string
	Message string
}

func (e *Error) Error() string {
	if e.Path == "" {
		return "miner: " + e.Message
	}
	return "miner: " + e.Path + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Kind }

func Mine(ctx context.Context, client signoz.SigNozClient, config Config) (contracts.Contract, error) {
	if ctx == nil {
		return contracts.Contract{}, invalid("context", "context is required")
	}
	if client == nil {
		return contracts.Contract{}, invalid("client", "SigNoz client is required")
	}
	if config.DashboardID == "" {
		return contracts.Contract{}, invalid("dashboard_id", "dashboard id is required")
	}
	if config.AlertID == "" {
		return contracts.Contract{}, invalid("alert_id", "alert id is required")
	}
	if config.Service == "" {
		return contracts.Contract{}, invalid("service", "service is required")
	}
	if config.Release == "" {
		return contracts.Contract{}, invalid("release", "release is required")
	}

	dashboard, err := client.GetDashboard(ctx, config.DashboardID)
	if err != nil {
		return contracts.Contract{}, fmt.Errorf("mine dashboard: %w", err)
	}
	alert, err := client.GetAlert(ctx, config.AlertID)
	if err != nil {
		return contracts.Contract{}, fmt.Errorf("mine alert: %w", err)
	}

	collector := newCollector()
	if err := collector.dashboard(dashboard); err != nil {
		return contracts.Contract{}, err
	}
	if err := collector.alert(alert); err != nil {
		return contracts.Contract{}, err
	}
	if !collector.hasRequirement(requiredOperationID()) {
		return contracts.Contract{}, invalid("requirements", "required operation payment.authorize was not found")
	}

	contract := contracts.New(config.Service, config.Release)
	contract.Consumers = collector.consumers
	for _, requirement := range collector.requirements {
		contract.Checks = append(contract.Checks, *requirement)
	}
	contract.Normalize()
	if err := contract.Validate(); err != nil {
		return contracts.Contract{}, err
	}
	return contract, nil
}

// MineContract is a descriptive alias for callers that prefer the result name.
func MineContract(ctx context.Context, client signoz.SigNozClient, config Config) (contracts.Contract, error) {
	return Mine(ctx, client, config)
}

type collector struct {
	consumers    []contracts.Consumer
	consumerIDs  map[string]struct{}
	requirements map[string]*contracts.Requirement
}

func newCollector() *collector {
	return &collector{
		consumerIDs:  make(map[string]struct{}),
		requirements: make(map[string]*contracts.Requirement),
	}
}

func (c *collector) dashboard(dashboard signoz.Dashboard) error {
	if dashboard.ID == "" {
		return invalid("$.data.id", "dashboard id is required")
	}
	if dashboard.Title == "" {
		return invalid("$.data.data.title", "dashboard title is required")
	}
	if len(dashboard.Widgets) == 0 {
		return invalid("$.data.data.widgets", "dashboard has no panels")
	}
	for index, widget := range dashboard.Widgets {
		widgetPath := widget.SourcePath
		if !contracts.IsJSONPath(widgetPath) {
			return invalid(fmt.Sprintf("dashboard.widgets[%d].source_path", index), "malformed JSON path")
		}
		if widget.ID == "" {
			return invalid(widgetPath+".id", "panel id is required")
		}
		if widget.Title == "" {
			return invalid(widgetPath+".title", "panel title is required")
		}
		if widget.Query.QueryType != "builder" {
			return unsupported(widgetPath+".query.queryType", "query type is not builder")
		}
		if len(widget.Query.UnsupportedNodes) > 0 {
			return unsupported(widgetPath+".query", "unsupported query node")
		}
		builder := widget.Query.Builder
		if len(builder.UnsupportedNodes) > 0 {
			return unsupported(widgetPath+".query.builder", "unsupported query node")
		}
		if len(builder.QueryData) != 1 {
			return unsupported(widgetPath+".query.builder.queryData", "exactly one builder query is supported")
		}
		query := builder.QueryData[0]
		if !contracts.IsJSONPath(query.SourcePath) {
			return invalid(widgetPath+".query.builder.queryData[0].source_path", "malformed JSON path")
		}
		if len(query.UnsupportedNodes) > 0 {
			return unsupported(query.SourcePath, "unsupported query node")
		}
		if query.NodeType != "" && query.NodeType != "builder_query" {
			return unsupported(query.SourcePath, "unsupported query node")
		}
		if query.Disabled {
			return unsupported(query.SourcePath+".disabled", "disabled query cannot provide a requirement")
		}
		if (query.Signal != "" && query.Signal != "traces") || query.DataSource != "traces" {
			return unsupported(query.SourcePath+".signal", "only traces builder queries are supported")
		}
		if query.FieldDataType != "" && !numericType(query.FieldDataType) {
			return invalid(query.SourcePath+".fieldDataType", "cart.value must be numeric")
		}
		if len(query.Aggregations) != 1 {
			return unsupported(query.SourcePath+".aggregations", "exactly one aggregation is supported")
		}
		aggregation := query.Aggregations[0]
		if !contracts.IsJSONPath(aggregation.SourcePath) {
			return invalid(query.SourcePath+".aggregations[0].source_path", "malformed JSON path")
		}
		field, ok := parseCall(aggregation.Expression, "sum")
		if !ok {
			return unsupported(aggregation.SourcePath, "unsupported aggregation expression")
		}
		if field != RequiredCartValue {
			return invalid(aggregation.SourcePath, "unknown required field")
		}
		if aggregation.FieldDataType != "" && !numericType(aggregation.FieldDataType) {
			return invalid(aggregation.SourcePath, "cart.value has an unsupported field type")
		}
		if query.FilterDataType != "" && strings.ToLower(strings.TrimSpace(query.FilterDataType)) != "string" {
			return invalid(query.FilterSourcePath, "dashboard filter must be a string")
		}
		filter, err := scopedFilter(query, []string{"service.name", "run.id"}, []string{"service.name", "run.id"}, query.SourcePath+".filter.expression")
		if err != nil {
			return err
		}

		consumer := contracts.Consumer{
			ID:          dashboardConsumerID(dashboard.Title, widget.ID),
			Type:        DashboardPanelType,
			Name:        widget.Title,
			Criticality: "required",
			Source:      contracts.Source{DashboardID: dashboard.ID, PanelID: widget.ID},
		}
		requirements := []contracts.Requirement{{
			ID:         requiredCartValueID(),
			Type:       RequiredFieldType,
			Signal:     "traces",
			Field:      RequiredCartValue,
			Filter:     filter,
			SourcePath: aggregation.SourcePath,
		}}
		if strings.Contains(widget.Description, RequiredOperation) {
			requirements = append(requirements, contracts.Requirement{
				ID:         requiredOperationID(),
				Type:       RequiredOperationType,
				Signal:     "traces",
				Operation:  RequiredOperation,
				SourcePath: widgetPath + ".description",
			})
		}
		if err := c.add(consumer, requirements); err != nil {
			return err
		}
	}
	return nil
}

func (c *collector) alert(alert signoz.Alert) error {
	if alert.ID == "" {
		return invalid("$.data.id", "alert id is required")
	}
	if alert.Name == "" {
		return invalid("$.data.alert", "alert name is required")
	}
	if alert.Condition.CompositeQuery.QueryType != "builder" {
		return unsupported("$.data.condition.compositeQuery.queryType", "alert query type is not builder")
	}
	if alert.Condition.CompositeQuery.PanelType != "" && alert.Condition.CompositeQuery.PanelType != "graph" {
		return unsupported("$.data.condition.compositeQuery.panelType", "unsupported alert panel type")
	}
	if len(alert.Condition.CompositeQuery.UnsupportedNodes) > 0 {
		return unsupported("$.data.condition.compositeQuery", "unsupported query node")
	}
	if len(alert.Condition.CompositeQuery.Queries) != 1 {
		return unsupported("$.data.condition.compositeQuery.queries", "exactly one alert query is supported")
	}
	query := alert.Condition.CompositeQuery.Queries[0]
	if !contracts.IsJSONPath(query.SourcePath) {
		return invalid("$.data.condition.compositeQuery.queries[0].spec.source_path", "malformed JSON path")
	}
	if query.NodeType != "" && query.NodeType != "builder_query" {
		return unsupported(query.SourcePath, "unsupported query node")
	}
	if len(query.UnsupportedNodes) > 0 {
		return unsupported(query.SourcePath, "unsupported query node")
	}
	if query.Disabled {
		return unsupported(query.SourcePath+".disabled", "disabled query cannot provide a requirement")
	}
	if query.Signal != "traces" || (query.DataSource != "" && query.DataSource != "traces") {
		return unsupported(query.SourcePath+".signal", "only traces builder queries are supported")
	}
	if query.FieldDataType != "" && strings.ToLower(strings.TrimSpace(query.FieldDataType)) != "string" {
		return invalid(query.SourcePath+".fieldDataType", "error.type must be a string")
	}
	if query.FilterDataType != "" && strings.ToLower(strings.TrimSpace(query.FilterDataType)) != "string" {
		return invalid(query.FilterSourcePath, "alert filter must be a string")
	}
	if query.Filter == "" {
		return invalid(query.SourcePath+".filter.expression", "alert filter is required")
	}
	filter, err := scopedFilter(query, []string{"service.name", "run.id", RequiredErrorType}, []string{"service.name", "run.id", RequiredErrorType}, query.FilterSourcePath)
	if err != nil {
		return err
	}
	terms := filterTerms(query.Filter)
	if terms[RequiredErrorType] != "payment_timeout" {
		return invalid(query.FilterSourcePath, "error.type must equal payment_timeout")
	}
	if alert.Condition.SelectedQueryName != "" && query.Name != "" && alert.Condition.SelectedQueryName != query.Name {
		return invalid("$.data.condition.selectedQueryName", "selected query does not match the alert query")
	}
	if len(query.Aggregations) != 1 || query.Aggregations[0].Expression != "count()" {
		return unsupported(query.SourcePath+".aggregations", "count() is required for the alert")
	}
	if len(alert.Condition.Thresholds) == 0 {
		return invalid("$.data.condition.thresholds.spec", "alert threshold is required")
	}

	logicalAlertID := alertLogicalID(alert.Name)
	if logicalAlertID == "" {
		return invalid("$.data.alert", "alert name has no stable identity")
	}
	consumer := contracts.Consumer{
		ID:          "alert-" + slug(logicalAlertID),
		Type:        AlertConsumerType,
		Name:        alert.Name,
		Criticality: "required",
		Source:      contracts.Source{AlertID: alert.ID},
	}
	requirements := []contracts.Requirement{
		{
			ID:         requiredErrorTypeID(),
			Type:       RequiredFieldType,
			Signal:     "traces",
			Field:      RequiredErrorType,
			Filter:     filter,
			SourcePath: query.FilterSourcePath,
		},
		{
			ID:         requiredAlertID(),
			Type:       AlertMustFireType,
			AlertID:    logicalAlertID,
			Timeout:    AlertCheckTimeout,
			SourcePath: "$.data.alert",
		},
	}
	if strings.Contains(alert.Annotations.Description, RequiredOperation) {
		requirements = append(requirements, contracts.Requirement{
			ID:         requiredOperationID(),
			Type:       RequiredOperationType,
			Signal:     "traces",
			Operation:  RequiredOperation,
			SourcePath: "$.data.annotations.description",
		})
	}
	return c.add(consumer, requirements)
}

func (c *collector) add(consumer contracts.Consumer, requirements []contracts.Requirement) error {
	if consumer.ID == "" {
		return invalid("consumer.id", "consumer identity is required")
	}
	if _, exists := c.consumerIDs[consumer.ID]; exists {
		return invalid("consumer.id", "duplicate consumer identity")
	}
	for _, requirement := range requirements {
		if !contracts.IsJSONPath(requirement.SourcePath) {
			return invalid("requirement.source_path", "malformed JSON path")
		}
		if requirement.ID == "" {
			return invalid("requirement.id", "requirement identity is required")
		}
		current := c.requirements[requirement.ID]
		if current == nil {
			copy := requirement
			copy.Consumers = []string{consumer.ID}
			c.requirements[requirement.ID] = &copy
		} else {
			if !sameRequirement(*current, requirement) {
				return invalid("requirements."+requirement.ID, "duplicate requirement has conflicting definitions")
			}
			current.SourcePaths = append(current.SourcePaths, requirement.SourcePath)
			current.Consumers = append(current.Consumers, consumer.ID)
			if requirement.Filter != "" {
				current.Filters = append(current.Filters, requirement.Filter)
			}
		}
		consumer.Requires = append(consumer.Requires, contracts.RequirementRef{ID: requirement.ID, SourcePath: requirement.SourcePath})
	}
	c.consumerIDs[consumer.ID] = struct{}{}
	c.consumers = append(c.consumers, consumer)
	return nil
}

func (c *collector) hasRequirement(id string) bool {
	_, exists := c.requirements[id]
	return exists
}

func sameRequirement(left, right contracts.Requirement) bool {
	return left.ID == right.ID && left.Type == right.Type && left.Signal == right.Signal && left.Field == right.Field &&
		left.Operation == right.Operation && left.AlertID == right.AlertID && left.Timeout == right.Timeout
}

func scopedFilter(query signoz.QuerySpec, allowed, required []string, sourcePath string) (string, error) {
	if query.Filter == "" {
		return "", invalid(sourcePath, "filter is required")
	}
	if !contracts.IsJSONPath(sourcePath) {
		return "", invalid("filter.source_path", "malformed JSON path")
	}
	terms := filterTerms(query.Filter)
	if len(terms) == 0 {
		return "", unsupported(sourcePath, "unsupported filter expression")
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range terms {
		if _, ok := allowedSet[field]; !ok {
			return "", invalid(sourcePath, "unknown filter field")
		}
	}
	for _, field := range required {
		if terms[field] == "" {
			return "", invalid(sourcePath, "required filter is missing")
		}
	}
	order := make([]string, 0, len(allowed))
	for _, field := range allowed {
		if terms[field] != "" {
			order = append(order, field)
		}
	}
	parts := make([]string, 0, len(order))
	for _, field := range order {
		value := terms[field]
		if field == "run.id" {
			value = "__RUN_ID__"
		}
		parts = append(parts, field+" = '"+value+"'")
	}
	return strings.Join(parts, " AND "), nil
}

func filterTerms(expression string) map[string]string {
	terms := make(map[string]string)
	for _, rawTerm := range strings.Split(expression, " AND ") {
		term := strings.TrimSpace(rawTerm)
		parts := strings.Split(term, " = ")
		if len(parts) != 2 {
			continue
		}
		field := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if len(value) < 2 || value[0] != '\'' || value[len(value)-1] != '\'' {
			continue
		}
		if _, exists := terms[field]; exists {
			continue
		}
		terms[field] = value[1 : len(value)-1]
	}
	return terms
}

func parseCall(expression, name string) (string, bool) {
	prefix := name + "("
	value := strings.TrimSpace(expression)
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, ")") || strings.Count(value, "(") != 1 || strings.Count(value, ")") != 1 {
		return "", false
	}
	field := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, prefix), ")"))
	if field == "" || strings.ContainsAny(field, "() '") {
		return "", false
	}
	return field, true
}

func numericType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "float64", "float", "double", "number", "int64", "int":
		return true
	default:
		return false
	}
}

func dashboardConsumerID(title, panelID string) string {
	return "dashboard-panel-" + slug(title) + "-" + slug(panelID)
}

func alertLogicalID(name string) string {
	value := slug(name)
	value = strings.TrimPrefix(value, "telemetry-guardian-")
	return value
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			b.WriteRune(character)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func requiredCartValueID() string { return "required-field-cart-value" }
func requiredErrorTypeID() string { return "required-field-error-type" }
func requiredOperationID() string { return "required-operation-payment-authorize" }
func requiredAlertID() string     { return "alert-must-fire-payment-timeout" }

func invalid(path, message string) error {
	return &Error{Kind: ErrInvalidInput, Path: path, Message: message}
}
func unsupported(path, message string) error {
	return &Error{Kind: ErrUnsupported, Path: path, Message: message}
}
