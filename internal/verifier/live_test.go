package verifier

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/evidence"
	"github.com/ambitious-scrap/telemetry-guardian/internal/signoz"
)

func TestLiveVerifier(t *testing.T) {
	required := []string{
		"SIGNOZ_URL", "SIGNOZ_TOKEN", "SIGNOZ_ALERT_ID", "GUARDIAN_CONTRACT",
		"GUARDIAN_RUN_ID", "GUARDIAN_START", "GUARDIAN_END", "GUARDIAN_FAULT_INJECTED_AT", "GUARDIAN_EXPECT",
	}
	values := make(map[string]string, len(required))
	for _, name := range required {
		values[name] = os.Getenv(name)
		if values[name] == "" {
			t.Skip("set Phase 4 live verification environment")
		}
	}
	file, err := os.Open(values["GUARDIAN_CONTRACT"])
	if err != nil {
		t.Fatal(err)
	}
	contract, err := contracts.LoadYAML(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	parse := func(name string) time.Time {
		value, err := time.Parse(time.RFC3339Nano, values[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return value
	}
	client, err := signoz.NewHTTPClient(signoz.Config{
		BaseURL: values["SIGNOZ_URL"], Token: values["SIGNOZ_TOKEN"], Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := Verify(context.Background(), client, contract, Config{
		RunID: values["GUARDIAN_RUN_ID"], AlertResourceID: values["SIGNOZ_ALERT_ID"],
		Start: parse("GUARDIAN_START"), End: parse("GUARDIAN_END"),
		FaultInjectedAt: parse("GUARDIAN_FAULT_INJECTED_AT"),
		MinimumSamples:  5, PollInterval: 100 * time.Millisecond,
		CompletenessTimeout: 10 * time.Second, QueryTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string][]evidence.State{
		"healthy": {evidence.Pass, evidence.Pass, evidence.Pass, evidence.Pass},
		"broken":  {evidence.Fail, evidence.Fail, evidence.Pass, evidence.Fail},
		"no-load": {evidence.Inconclusive, evidence.Inconclusive, evidence.Inconclusive, evidence.Inconclusive},
	}[values["GUARDIAN_EXPECT"]]
	if expected == nil {
		t.Fatalf("unknown GUARDIAN_EXPECT %q", values["GUARDIAN_EXPECT"])
	}
	for i, state := range expected {
		if verdict.CheckResults[i].State != state {
			t.Fatalf("check %d = %s, want %s", i, verdict.CheckResults[i].State, state)
		}
	}
}
