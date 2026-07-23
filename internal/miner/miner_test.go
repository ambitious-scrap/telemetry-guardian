package miner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/signoz"
)

var fixtureConfig = Config{
	DashboardID: "dashboard-fixture-id",
	AlertID:     "alert-fixture-id",
	Service:     "checkout",
	Release:     "candidate",
}

func TestCanonicalMiningGolden(t *testing.T) {
	contract := mineFixture(t)
	if len(contract.Checks) != 4 {
		t.Fatalf("checks = %d, want 4", len(contract.Checks))
	}
	actual, err := contract.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "canonical-contract.yaml"))
	if err != nil {
		t.Fatalf("read golden: %v\nactual:\n%s", err, actual)
	}
	if string(actual) != string(golden) {
		t.Fatalf("canonical contract changed:\n%s", actual)
	}
}

func TestCanonicalMiningPreservesIDsPathsAndStableMappings(t *testing.T) {
	contract := mineFixture(t)
	if contract.APIVersion != contracts.APIVersion || contract.Service != "checkout" || contract.Release != "candidate" {
		t.Fatalf("contract header = %#v", contract)
	}
	if len(contract.Consumers) != 2 {
		t.Fatalf("consumers = %d, want 2", len(contract.Consumers))
	}
	checks := make(map[string]contracts.Requirement, len(contract.Checks))
	for _, check := range contract.Checks {
		checks[check.ID] = check
	}
	if checks[requiredCartValueID()].Field != RequiredCartValue || checks[requiredErrorTypeID()].Field != RequiredErrorType {
		t.Fatalf("field checks = %#v", checks)
	}
	if checks[requiredOperationID()].Operation != RequiredOperation || checks[requiredAlertID()].AlertID != RequiredAlertID {
		t.Fatalf("canonical checks = %#v", checks)
	}
	if checks[requiredCartValueID()].SourcePath != "$.data.data.widgets[0].query.builder.queryData[0].aggregations[0].expression" {
		t.Fatalf("cart source path = %q", checks[requiredCartValueID()].SourcePath)
	}
	if checks[requiredErrorTypeID()].SourcePath != "$.data.condition.compositeQuery.queries[0].spec.filter.expression" {
		t.Fatalf("error source path = %q", checks[requiredErrorTypeID()].SourcePath)
	}
	if checks[requiredCartValueID()].Filter != "service.name = 'checkout' AND run.id = '__RUN_ID__'" {
		t.Fatalf("cart filter = %q", checks[requiredCartValueID()].Filter)
	}
	if checks[requiredErrorTypeID()].Filter != "service.name = 'checkout' AND run.id = '__RUN_ID__' AND error.type = 'payment_timeout'" {
		t.Fatalf("error filter = %q", checks[requiredErrorTypeID()].Filter)
	}
	if len(checks[requiredCartValueID()].Consumers) != 1 || len(checks[requiredErrorTypeID()].Consumers) != 1 || len(checks[requiredAlertID()].Consumers) != 1 {
		t.Fatalf("consumer mappings = %#v", checks)
	}
	if len(checks[requiredOperationID()].Consumers) != 2 {
		t.Fatalf("operation consumers = %#v", checks[requiredOperationID()].Consumers)
	}
	for _, consumer := range contract.Consumers {
		if consumer.Source.DashboardID == "dashboard-fixture-id" && consumer.Source.PanelID != "cart-value-panel" {
			t.Fatalf("dashboard source = %#v", consumer.Source)
		}
		if consumer.Source.AlertID == "alert-fixture-id" && !strings.Contains(consumer.Name, "payment-timeout") {
			t.Fatalf("alert source/name = %#v", consumer)
		}
		for _, required := range consumer.Requires {
			if !contracts.IsJSONPath(required.SourcePath) {
				t.Fatalf("consumer source path = %#v", required)
			}
		}
	}
}

func TestCanonicalMiningIsByteStable(t *testing.T) {
	first := mineFixture(t)
	second := mineFixture(t)
	firstYAML, err := first.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	secondYAML, err := second.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if string(firstYAML) != string(secondYAML) {
		t.Fatal("identical fixture inputs produced different YAML")
	}
}

func TestMiningDeduplicatesRequirementsAndRetainsConsumers(t *testing.T) {
	fake := fixtureFake(t)
	duplicate := fake.DashboardResult.Widgets[0]
	duplicate.Query.Builder.QueryData = append([]signoz.QuerySpec(nil), duplicate.Query.Builder.QueryData...)
	duplicate.Query.Builder.QueryData[0].Aggregations = append([]signoz.Aggregation(nil), duplicate.Query.Builder.QueryData[0].Aggregations...)
	duplicate.ID = "cart-value-panel-copy"
	duplicate.Title = "Cart value copy"
	duplicate.SourcePath = "$.data.data.widgets[1]"
	duplicate.Query.Builder.QueryData[0].SourcePath = "$.data.data.widgets[1].query.builder.queryData[0]"
	duplicate.Query.Builder.QueryData[0].FilterSourcePath = duplicate.Query.Builder.QueryData[0].SourcePath + ".filter.expression"
	duplicate.Query.Builder.QueryData[0].Aggregations[0].SourcePath = duplicate.Query.Builder.QueryData[0].SourcePath + ".aggregations[0].expression"
	fake.DashboardResult.Widgets = append(fake.DashboardResult.Widgets, duplicate)
	contract, err := Mine(context.Background(), fake, fixtureConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Checks) != 4 {
		t.Fatalf("checks = %d, want deduplicated 4", len(contract.Checks))
	}
	cart := findCheck(contract, requiredCartValueID())
	if len(cart.Consumers) != 2 || len(cart.SourcePaths) != 2 {
		t.Fatalf("deduplicated cart check = %#v", cart)
	}
	if len(findConsumer(contract, "cart-value-panel-copy").Requires) == 0 {
		t.Fatal("duplicate consumer lost its requirement mapping")
	}
}

func TestMiningMutationMatrixFailsExplicitly(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*signoz.FakeClient)
	}{
		{name: "renamed field", mutate: func(fake *signoz.FakeClient) {
			fake.DashboardResult.Widgets[0].Query.Builder.QueryData[0].Aggregations[0].Expression = "sum(cart.amount)"
		}},
		{name: "removed filter", mutate: func(fake *signoz.FakeClient) {
			fake.DashboardResult.Widgets[0].Query.Builder.QueryData[0].Filter = ""
		}},
		{name: "nested formula", mutate: func(fake *signoz.FakeClient) {
			fake.DashboardResult.Widgets[0].Query.Builder.UnsupportedNodes = []string{"queryFormulas"}
		}},
		{name: "missing panel title", mutate: func(fake *signoz.FakeClient) {
			fake.DashboardResult.Widgets[0].Title = ""
		}},
		{name: "unsupported query node", mutate: func(fake *signoz.FakeClient) {
			fake.DashboardResult.Widgets[0].Query.QueryType = "promql"
		}},
		{name: "changed field type", mutate: func(fake *signoz.FakeClient) {
			fake.DashboardResult.Widgets[0].Query.Builder.QueryData[0].Aggregations[0].FieldDataType = "string"
		}},
		{name: "malformed source path", mutate: func(fake *signoz.FakeClient) {
			fake.DashboardResult.Widgets[0].Query.Builder.QueryData[0].Aggregations[0].SourcePath = "widgets[0]"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := fixtureFake(t)
			test.mutate(fake)
			_, err := Mine(context.Background(), fake, fixtureConfig)
			if err == nil || (!errors.Is(err, ErrInvalidInput) && !errors.Is(err, ErrUnsupported)) {
				t.Fatalf("mutation error = %v", err)
			}
		})
	}
}

func TestMiningEmptyInputsAndFakeErrors(t *testing.T) {
	t.Run("empty dashboard", func(t *testing.T) {
		fake := fixtureFake(t)
		fake.DashboardResult.Widgets = nil
		assertMiningError(t, fake)
	})
	t.Run("empty alert query", func(t *testing.T) {
		fake := fixtureFake(t)
		fake.AlertResult.Condition.CompositeQuery.Queries = nil
		assertMiningError(t, fake)
	})
	t.Run("dashboard fake error", func(t *testing.T) {
		fake := fixtureFake(t)
		fake.DashboardError = signoz.ErrForbidden
		_, err := Mine(context.Background(), fake, fixtureConfig)
		if !errors.Is(err, signoz.ErrForbidden) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("alert fake error", func(t *testing.T) {
		fake := fixtureFake(t)
		fake.AlertError = signoz.ErrUnauthorized
		_, err := Mine(context.Background(), fake, fixtureConfig)
		if !errors.Is(err, signoz.ErrUnauthorized) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid input", func(t *testing.T) {
		fake := fixtureFake(t)
		_, err := Mine(context.Background(), fake, Config{AlertID: fixtureConfig.AlertID, Service: "checkout", Release: "candidate"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("error = %v", err)
		}
	})
}

func mineFixture(t *testing.T) contracts.Contract {
	t.Helper()
	contract, err := Mine(context.Background(), fixtureFake(t), fixtureConfig)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func fixtureFake(t *testing.T) *signoz.FakeClient {
	t.Helper()
	fake, err := signoz.NewFixtureFake()
	if err != nil {
		t.Fatal(err)
	}
	return fake
}

func assertMiningError(t *testing.T, fake *signoz.FakeClient) {
	t.Helper()
	if _, err := Mine(context.Background(), fake, fixtureConfig); err == nil {
		t.Fatal("malformed input produced a contract")
	}
}

func findCheck(contract contracts.Contract, id string) contracts.Requirement {
	for _, check := range contract.Checks {
		if check.ID == id {
			return check
		}
	}
	return contracts.Requirement{}
}

func findConsumer(contract contracts.Contract, panelID string) contracts.Consumer {
	for _, consumer := range contract.Consumers {
		if consumer.Source.PanelID == panelID {
			return consumer
		}
	}
	return contracts.Consumer{}
}
