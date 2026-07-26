package contracts

import (
	"bytes"
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

func TestLoadYAMLValidatesStrictly(t *testing.T) {
	const valid = `apiVersion: telemetry.guardian/v1
service: checkout
release: candidate
consumers:
  - id: panel
    type: dashboard_panel
    name: Cart
    source:
      dashboard_id: dashboard
      panel_id: panel
    requires:
      - id: cart
        source_path: "$.data.panel"
checks:
  - id: cart
    type: required_field
    signal: traces
    field: cart.value
    source_path: "$.data.panel"
    consumers:
      - panel
`
	loaded, err := LoadYAML(bytes.NewBufferString(valid))
	if err != nil || len(loaded.Checks) != 1 {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
	invalid := strings.Replace(valid, "apiVersion:", "unknown:\napiVersion:", 1)
	if _, err := LoadYAML(strings.NewReader(invalid)); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestAlertTimeoutMustBePositiveDuration(t *testing.T) {
	contract := New("checkout", "candidate")
	contract.Consumers = []Consumer{{
		ID: "alert", Type: "alert", Name: "Timeout", Source: Source{AlertID: "alert"},
		Requires: []RequirementRef{{ID: "must-fire", SourcePath: "$.data.alert"}},
	}}
	contract.Checks = []Requirement{{
		ID: "must-fire", Type: "alert_must_fire", AlertID: "payment-timeout", Timeout: "eventually",
		SourcePath: "$.data.alert", Consumers: []string{"alert"},
	}}
	if err := contract.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("timeout error = %v", err)
	}
}
