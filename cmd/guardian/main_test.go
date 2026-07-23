package main

import (
	"bytes"
	"strings"
	"testing"
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
	if err := run([]string{"verify"}, &stdout, &stderr); err == nil {
		t.Fatal("unsupported command was accepted")
	}
}
