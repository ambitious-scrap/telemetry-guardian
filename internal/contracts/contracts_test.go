package contracts

import (
	"errors"
	"strings"
	"testing"
)

func TestContractValidationRejectsMissingSourceAndUnknownMappings(t *testing.T) {
	contract := New("checkout", "candidate")
	contract.Consumers = []Consumer{{
		ID: "panel", Type: "dashboard_panel", Name: "Cart", Source: Source{DashboardID: "dashboard", PanelID: "panel"},
		Requires: []RequirementRef{{ID: "cart", SourcePath: "$.data.widgets[0].query"}},
	}}
	contract.Checks = []Requirement{{
		ID: "cart", Type: "required_field", Signal: "traces", Field: "cart.value",
		SourcePath: "$.data.widgets[0].query", Consumers: []string{"missing"},
	}}
	if err := contract.Validate(); err == nil || !errors.Is(err, ErrInvalidContract) || !strings.Contains(err.Error(), "unknown consumer") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestContractYAMLIsDeterministicAndQuotesUnsafeScalars(t *testing.T) {
	contract := New("checkout", "candidate")
	contract.Consumers = []Consumer{{
		ID: "panel", Type: "dashboard_panel", Name: "Cart value", Criticality: "required",
		Source:   Source{DashboardID: "dashboard", PanelID: "panel"},
		Requires: []RequirementRef{{ID: "cart", SourcePath: "$.data.widgets[0].query"}},
	}}
	contract.Checks = []Requirement{{
		ID: "cart", Type: "required_field", Signal: "traces", Field: "cart.value",
		Filter: "service.name = 'checkout'", SourcePath: "$.data.widgets[0].query", Consumers: []string{"panel"},
	}}
	first, err := contract.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	second, err := contract.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("YAML output changed between identical serializations")
	}
	output := string(first)
	if !strings.Contains(output, "field: cart.value\n") || !strings.Contains(output, "filter: \"service.name = 'checkout'\"\n") {
		t.Fatalf("unexpected YAML:\n%s", output)
	}
}

func TestIsJSONPath(t *testing.T) {
	if !IsJSONPath("$.data.widgets[0].query.builder.queryData[0].aggregations[0].expression") {
		t.Fatal("valid JSON path rejected")
	}
	for _, path := range []string{"", "data.widgets[0]", "$.data..widgets", "$.data.widgets[bad]"} {
		if IsJSONPath(path) {
			t.Fatalf("malformed JSON path accepted: %q", path)
		}
	}
}
