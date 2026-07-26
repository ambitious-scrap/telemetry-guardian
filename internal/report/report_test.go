package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/evidence"
)

func fixtureContract() contracts.Contract {
	c := contracts.New("checkout", "candidate")
	c.Consumers = []contracts.Consumer{
		{ID: "panel", Type: "dashboard_panel", Name: "Revenue by region", Source: contracts.Source{DashboardID: "checkout-overview", PanelID: "revenue-region"}, Requires: []contracts.RequirementRef{{ID: "cart", SourcePath: "$.data.panel"}}},
		{ID: "alert", Type: "alert", Name: "Payment timeout", Source: contracts.Source{AlertID: "payment-timeout"}, Requires: []contracts.RequirementRef{{ID: "error", SourcePath: "$.data.alert"}}},
	}
	c.Checks = []contracts.Requirement{
		{ID: "cart", Type: "required_field", Signal: "traces", Field: "cart.value", SourcePath: "$.data.panel", Consumers: []string{"panel"}},
		{ID: "error", Type: "required_field", Signal: "traces", Field: "error.type", SourcePath: "$.data.alert", Consumers: []string{"alert"}},
	}
	return c
}

func fixtureVerdict(state evidence.State) evidence.Verdict {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	results := []evidence.CheckResult{
		{State: state, RequirementID: "cart", RunID: "run", AffectedConsumers: []string{"panel"}, Evidence: evidence.Record{Retrieval: "SearchTraces for cart.value", Start: now, End: now.Add(time.Minute), SampleCount: 5, MinimumSampleCount: 5, Summary: "verified", DataQuality: evidence.Complete, SigNozDeepLink: "https://signoz.example/explore"}},
		{State: state, RequirementID: "error", RunID: "run", AffectedConsumers: []string{"alert"}, Evidence: evidence.Record{Retrieval: "SearchTraces for error.type", Start: now, End: now.Add(time.Minute), SampleCount: 1, MinimumSampleCount: 1, Summary: "verified", DataQuality: evidence.Complete}},
	}
	return evidence.NewVerdict("run", "checkout", "candidate", now, now.Add(time.Minute), results)
}

func TestBuildBrokenGraphIsDeterministicAndComplete(t *testing.T) {
	contract := fixtureContract()
	first, err := Build(fixtureVerdict(evidence.Fail), contract)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(fixtureVerdict(evidence.Fail), contract)
	if err != nil {
		t.Fatal(err)
	}
	firstHTML, _ := render(first)
	secondHTML, _ := render(second)
	if firstHTML != secondHTML {
		t.Fatal("identical verdicts produced different reports")
	}
	for _, expected := range []string{"checkout-overview", "Revenue by region", "Payment timeout", "BREAKS", "REQUIRED_BY", "PART_OF", "SearchTraces for cart.value", "Sample count"} {
		if !strings.Contains(firstHTML, expected) {
			t.Fatalf("report omitted %q", expected)
		}
	}
	if !strings.Contains(firstHTML, "data-state=\"FAIL\"") || !strings.Contains(firstHTML, "data-open-consumer") || !strings.Contains(firstHTML, "aria-expanded=\"false\"") || strings.Contains(firstHTML, "Math.random") || strings.Contains(firstHTML, "forceSimulation") {
		t.Fatal("report state or deterministic layout contract missing")
	}
}

func TestBrokenGraphContainsExpectedNodesAndEdges(t *testing.T) {
	document, err := Build(fixtureVerdict(evidence.Fail), fixtureContract())
	if err != nil {
		t.Fatal(err)
	}
	nodes := make(map[string]bool, len(document.Nodes))
	for _, node := range document.Nodes {
		nodes[node.ID] = true
	}
	for _, expected := range []string{"dashboard:checkout-overview", "consumer:panel", "requirement:cart", "consumer:alert", "requirement:error"} {
		if !nodes[expected] {
			t.Fatalf("missing node %q", expected)
		}
	}
	edges := make(map[string]bool, len(document.Edges))
	for _, edge := range document.Edges {
		edges[edge.From+"|"+edge.To+"|"+edge.Label] = true
	}
	for _, expected := range []string{
		"consumer:panel|dashboard:checkout-overview|PART_OF",
		"consumer:panel|requirement:cart|REQUIRED_BY",
		"requirement:cart|consumer:panel|BREAKS",
		"consumer:alert|requirement:error|REQUIRED_BY",
		"requirement:error|consumer:alert|BREAKS",
	} {
		if !edges[expected] {
			t.Fatalf("missing edge %q", expected)
		}
	}
}

func TestHealthyAndInconclusiveStatesAreDistinct(t *testing.T) {
	healthy, err := Build(fixtureVerdict(evidence.Pass), fixtureContract())
	if err != nil {
		t.Fatal(err)
	}
	healthyHTML, _ := render(healthy)
	if !strings.Contains(healthyHTML, "contract healthy") || strings.Contains(healthyHTML, "BREAKS") {
		t.Fatal("healthy report is not calm")
	}
	inconclusive, err := Build(fixtureVerdict(evidence.Inconclusive), fixtureContract())
	if err != nil {
		t.Fatal(err)
	}
	inconclusiveHTML, _ := render(inconclusive)
	if !strings.Contains(inconclusiveHTML, "VERIFICATION_INCONCLUSIVE") || strings.Contains(inconclusiveHTML, "contract healthy") {
		t.Fatal("inconclusive report was presented as healthy")
	}
}

func TestUnsafeLinksAndSecretLikeValuesAreOmitted(t *testing.T) {
	verdict := fixtureVerdict(evidence.Fail)
	verdict.CheckResults[0].Evidence.SigNozDeepLink = "https://signoz.example/explore?token=do-not-print"
	verdict.CheckResults[0].Evidence.Summary = "authorization=do-not-print /Users/private/file"
	document, err := Build(verdict, fixtureContract())
	if err != nil {
		t.Fatal(err)
	}
	html, _ := render(document)
	for _, secret := range []string{"do-not-print", "/Users/private/file", "authorization="} {
		if strings.Contains(html, secret) {
			t.Fatalf("unsafe value leaked: %q", secret)
		}
	}
}

func TestBuildRejectsIncompleteVerdict(t *testing.T) {
	verdict := fixtureVerdict(evidence.Fail)
	verdict.CheckResults = verdict.CheckResults[:1]
	if _, err := Build(verdict, fixtureContract()); err == nil {
		t.Fatal("incomplete verdict accepted")
	}
}

func TestClassifications(t *testing.T) {
	for code, expected := range map[int]string{0: "PASS", 1: "TELEMETRY_CONTRACT_VIOLATION", 2: "VERIFICATION_INCONCLUSIVE", 3: "INVALID_GUARDIAN_CONFIGURATION"} {
		actual, ok := Classification(code)
		if !ok || actual != expected {
			t.Fatalf("code %d = %q/%v, want %q", code, actual, ok, expected)
		}
	}
	if _, ok := Classification(9); ok {
		t.Fatal("unknown exit code classified")
	}
}

func render(document Document) (string, error) {
	var output bytes.Buffer
	err := RenderHTML(&output, document)
	return output.String(), err
}
