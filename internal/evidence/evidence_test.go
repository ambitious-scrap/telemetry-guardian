package evidence

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestExitCodePrecedence(t *testing.T) {
	result := func(state State) CheckResult { return CheckResult{State: state} }
	tests := []struct {
		results []CheckResult
		state   State
		code    int
	}{
		{[]CheckResult{result(Pass), result(Pass)}, Pass, 0},
		{[]CheckResult{result(Pass), result(Fail)}, Fail, 1},
		{[]CheckResult{result(Fail), result(Inconclusive)}, Inconclusive, 2},
	}
	for _, test := range tests {
		verdict := NewVerdict("run", "checkout", "candidate", time.Now(), time.Now().Add(time.Second), test.results)
		if verdict.Overall != test.state || verdict.ExitCode() != test.code {
			t.Fatalf("aggregate = %s/%d, want %s/%d", verdict.Overall, verdict.ExitCode(), test.state, test.code)
		}
	}
}

func TestSecretBearingDeepLinkIsOmitted(t *testing.T) {
	const secret = "phase4-super-secret"
	result := CheckResult{
		State: Pass, RequirementID: "required-field-cart-value", RunID: "run",
		Evidence: Record{
			Retrieval: "query", Start: time.Now(), End: time.Now().Add(time.Second),
			Summary: "complete", DataQuality: Complete,
			SigNozDeepLink: SafeDeepLink("https://signoz.example/alert?token=" + secret),
		},
	}
	var output bytes.Buffer
	if err := WriteJSON(&output, NewVerdict("run", "checkout", "candidate", result.Evidence.Start, result.Evidence.End, []CheckResult{result})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), secret) || result.Evidence.SigNozDeepLink != "" {
		t.Fatal("secret-bearing deep link was written")
	}
}
