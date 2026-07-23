package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRunRejectsLaterPhaseCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"report"}, &stdout, &stderr); err == nil {
		t.Fatal("unsupported command was accepted")
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
