package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestVariantsAreFunctionallyEquivalent(t *testing.T) {
	tests := []struct {
		body string
		code int
	}{
		{`{"cart_value":42}`, http.StatusOK},
		{`{"cart_value":42,"fault":"payment-timeout"}`, http.StatusGatewayTimeout},
	}
	for _, test := range tests {
		var first string
		for _, variant := range []string{"healthy", "broken"} {
			app := server{variant: variant, emit: func(context.Context, telemetryEvent) error { return nil }}
			request := httptest.NewRequest(http.MethodPost, "/checkout", bytes.NewBufferString(test.body))
			response := httptest.NewRecorder()
			app.routes().ServeHTTP(response, request)
			if response.Code != test.code {
				t.Fatalf("%s: got status %d, want %d", variant, response.Code, test.code)
			}
			if first == "" {
				first = response.Body.String()
			} else if response.Body.String() != first {
				t.Fatalf("variant responses differ: %q != %q", response.Body.String(), first)
			}
		}
	}
}

func TestVariantsDifferOnlyInCanonicalTelemetry(t *testing.T) {
	t.Setenv("RUN_ID", "test-run")
	event := telemetryEvent{CartValue: 42, Fault: true, Started: time.Unix(1, 0), Ended: time.Unix(1, 25_000_000)}
	healthy := telemetryAttributes("healthy", event)
	broken := telemetryAttributes("broken", event)
	if len(healthy) != 3 || healthy[1].Key != "cart.value" || healthy[2].Key != "error.type" {
		t.Fatalf("unexpected healthy attributes: %#v", healthy)
	}
	if len(broken) != 3 || broken[1].Key != "cart.amount" || broken[2].Key != "error.kind" {
		t.Fatalf("unexpected broken attributes: %#v", broken)
	}
	if os.Getenv("RUN_ID") != "test-run" {
		t.Fatal("run ID changed")
	}
}
