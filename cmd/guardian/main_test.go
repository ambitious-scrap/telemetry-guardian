package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/evidence"
)

func TestRunMineRequiresConfiguredSigNoz(t *testing.T) {
	for _, name := range []string{"SIGNOZ_URL", "SIGNOZ_TOKEN", "SIGNOZ_DASHBOARD_ID", "SIGNOZ_ALERT_ID"} {
		t.Setenv(name, "")
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"mine"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "SIGNOZ_URL") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(stdout.String()+stderr.String(), "Bearer") {
		t.Fatal("configuration error exposed authentication details")
	}
}

func TestRunReportRequiresInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"report"}, &stdout, &stderr); err == nil {
		t.Fatal("report accepted missing inputs")
	}
}

func TestVerifyInvalidContractExitsThree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(path, []byte("apiVersion: wrong\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var stdout, stderr bytes.Buffer
	code := execute([]string{
		"verify",
		"--base-url", "http://127.0.0.1:1",
		"--token", "fixture",
		"--alert-id", "alert",
		"--contract", path,
		"--output", filepath.Join(t.TempDir(), "verdict.json"),
		"--run-id", "phase4-invalid",
		"--start", now.Add(-time.Minute).Format(time.RFC3339Nano),
		"--fault-injected-at", now.Add(-30 * time.Second).Format(time.RFC3339Nano),
		"--end", now.Format(time.RFC3339Nano),
	}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "fixture") {
		t.Fatal("CLI exposed authentication material")
	}
}

func TestMineDoesNotWriteContractAfterInvalidQueryResponse(t *testing.T) {
	dashboard := loadGuardianTestJSON(t, filepath.Join("..", "..", "internal", "signoz", "testdata", "dashboard-success.json"))
	data := dashboard["data"].(map[string]any)
	dashboardData := data["data"].(map[string]any)
	widget := dashboardData["widgets"].([]any)[0].(map[string]any)
	query := widget["query"].(map[string]any)
	query["unknownQueryNode"] = true
	dashboardPayload, err := json.Marshal(dashboard)
	if err != nil {
		t.Fatal(err)
	}
	alertPayload, err := os.ReadFile(filepath.Join("..", "..", "internal", "signoz", "testdata", "alert-success.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/dashboards/dashboard-fixture-id" {
			_, _ = response.Write(dashboardPayload)
			return
		}
		_, _ = response.Write(alertPayload)
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "contract.yaml")
	var stdout, stderr bytes.Buffer
	code := execute([]string{
		"mine", "--base-url", server.URL, "--token", "fixture-token",
		"--dashboard-id", "dashboard-fixture-id", "--alert-id", "alert-fixture-id",
		"--service", "checkout", "--release", "candidate", "--output", output,
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("invalid query unexpectedly mined contract: stdout=%s", stdout.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("contract output exists after failure: stat error=%v", err)
	}
}

func TestVerifyCanonicalTupleFailureExitsThreeWithoutVerdict(t *testing.T) {
	contract := loadGuardianTestContract(t)
	contract.Checks[0].Signal = "logs"
	contractPath := filepath.Join(t.TempDir(), "contract.yaml")
	payload, err := contract.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "verdict.json")
	now := time.Now().UTC()
	var stdout, stderr bytes.Buffer
	code := execute([]string{
		"verify", "--base-url", "http://127.0.0.1:1", "--token", "fixture-token", "--alert-id", "alert-fixture-id",
		"--contract", contractPath, "--output", output, "--run-id", "phase7-invalid",
		"--start", now.Add(-time.Minute).Format(time.RFC3339Nano),
		"--fault-injected-at", now.Add(-30 * time.Second).Format(time.RFC3339Nano),
		"--end", now.Format(time.RFC3339Nano),
	}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("verdict output exists after invalid configuration: stat error=%v", err)
	}
}

func TestReportContradictoryVerdictExitsThreeWithoutHTML(t *testing.T) {
	contract := loadGuardianTestContract(t)
	now := time.Now().UTC()
	results := make([]evidence.CheckResult, 0, len(contract.Checks))
	for _, check := range contract.Checks {
		results = append(results, evidence.CheckResult{
			State: evidence.Pass, RequirementID: check.ID, RunID: "phase7-report",
			AffectedConsumers: check.Consumers,
			Evidence:          evidence.Record{Retrieval: "fixture query", Start: now, End: now.Add(time.Minute), SampleCount: 1, MinimumSampleCount: 1, Summary: "fixture", DataQuality: evidence.Complete},
		})
	}
	verdict := evidence.NewVerdict("phase7-report", contract.Service, contract.Release, now, now.Add(time.Minute), results)
	verdict.Overall = evidence.Pass
	verdict.CheckResults[0].State = evidence.Fail
	verdictPath := filepath.Join(t.TempDir(), "verdict.json")
	var verdictJSON bytes.Buffer
	if err := evidence.WriteJSON(&verdictJSON, verdict); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(verdictPath, verdictJSON.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(t.TempDir(), "contract.yaml")
	contractYAML, err := contract.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, contractYAML, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "report.html")
	var stdout, stderr bytes.Buffer
	code := execute([]string{"report", "--contract", contractPath, "--verdict", verdictPath, "--output", output}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("report output exists after contradictory verdict: stat error=%v", err)
	}
}

func TestReportRejectsTrailingJSONWithoutWritingHTML(t *testing.T) {
	contractPath := filepath.Join("..", "..", "contracts", "telemetry.guardian.yaml")
	fixturePath := filepath.Join("..", "..", "internal", "report", "testdata", "healthy.json")
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	verdictPath := filepath.Join(t.TempDir(), "verdict.json")
	if err := os.WriteFile(verdictPath, append(payload, []byte("\n{}\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "report.html")
	var stdout, stderr bytes.Buffer
	code := execute([]string{"report", "--contract", contractPath, "--verdict", verdictPath, "--output", output}, &stdout, &stderr)
	if code != 3 || !strings.Contains(stderr.String(), "trailing JSON") {
		t.Fatalf("exit/stderr = %d/%s, want exit 3 with trailing JSON error", code, stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("report output exists after trailing JSON: stat error=%v", err)
	}
}

func TestReportRejectsUnknownVerdictStateWithoutWritingHTML(t *testing.T) {
	value := loadGuardianTestJSON(t, filepath.Join("..", "..", "internal", "report", "testdata", "healthy.json"))
	value["overall_state"] = "UNKNOWN"
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	verdictPath := filepath.Join(t.TempDir(), "verdict.json")
	if err := os.WriteFile(verdictPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "report.html")
	var stdout, stderr bytes.Buffer
	code := execute([]string{"report", "--contract", filepath.Join("..", "..", "contracts", "telemetry.guardian.yaml"), "--verdict", verdictPath, "--output", output}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("report output exists for unknown state: stat error=%v", err)
	}
}

func loadGuardianTestJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func loadGuardianTestContract(t *testing.T) contracts.Contract {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "contracts", "telemetry.guardian.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contract, err := contracts.LoadYAML(file)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
