package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const serviceName = "telemetry-guardian-checkout"

type checkoutRequest struct {
	CartValue float64 `json:"cart_value"`
	Fault     string  `json:"fault"`
}

type telemetryEvent struct {
	CartValue float64
	Fault     bool
	Started   time.Time
	Ended     time.Time
}

type server struct {
	variant   string
	emit      func(context.Context, telemetryEvent) error
	alertFile string
}

func (s server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"status\":\"ok\"}\n")
	})
	mux.HandleFunc("POST /checkout", s.checkout)
	mux.HandleFunc("POST /alerts", s.alert)
	return mux
}

func (s server) checkout(w http.ResponseWriter, r *http.Request) {
	var request checkoutRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if request.CartValue <= 0 || (request.Fault != "" && request.Fault != "payment-timeout") {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	started := time.Now()
	fault := request.Fault == "payment-timeout"
	ended := started.Add(25 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.emit(ctx, telemetryEvent{request.CartValue, fault, started, ended}); err != nil {
		log.Printf("telemetry export failed: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if fault {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = io.WriteString(w, "{\"error\":\"payment timeout\"}\n")
		return
	}
	_, _ = io.WriteString(w, "{\"order_id\":\"order-demo\",\"status\":\"approved\"}\n")
}

func (s server) alert(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || !json.Valid(body) {
		http.Error(w, `{"error":"invalid alert"}`, http.StatusBadRequest)
		return
	}
	file, err := os.OpenFile(s.alertFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		http.Error(w, `{"error":"alert storage unavailable"}`, http.StatusInternalServerError)
		return
	}
	defer file.Close()
	if _, err := file.Write(append(body, '\n')); err != nil {
		http.Error(w, `{"error":"alert storage unavailable"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type exporter struct {
	endpoint string
	runID    string
	variant  string
	client   *http.Client
}

func (e exporter) emit(ctx context.Context, event telemetryEvent) error {
	traceID, err := randomHex(16)
	if err != nil {
		return err
	}
	spanID, err := randomHex(8)
	if err != nil {
		return err
	}
	attributes := telemetryAttributes(e.variant, event)
	resource := []anyValue{
		{Key: "service.name", Value: stringValue(serviceName)},
		{Key: "run.id", Value: stringValue(e.runID)},
	}

	span := map[string]any{
		"traceId": traceID, "spanId": spanID, "name": "payment.authorize", "kind": 3,
		"startTimeUnixNano": fmt.Sprint(event.Started.UnixNano()),
		"endTimeUnixNano":   fmt.Sprint(event.Ended.UnixNano()),
		"attributes":        attributes,
		"status":            map[string]any{"code": map[bool]int{false: 1, true: 2}[event.Fault]},
	}
	trace := map[string]any{"resourceSpans": []any{map[string]any{
		"resource":   map[string]any{"attributes": resource},
		"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": "telemetry-guardian"}, "spans": []any{span}}},
	}}}

	duration := float64(event.Ended.Sub(event.Started).Microseconds()) / 1000
	metric := map[string]any{"resourceMetrics": []any{map[string]any{
		"resource": map[string]any{"attributes": resource},
		"scopeMetrics": []any{map[string]any{"scope": map[string]any{"name": "telemetry-guardian"}, "metrics": []any{map[string]any{
			"name": "checkout.duration", "unit": "ms", "gauge": map[string]any{"dataPoints": []any{map[string]any{
				"timeUnixNano": fmt.Sprint(event.Ended.UnixNano()), "asDouble": duration,
			}}},
		}}}},
	}}}

	payloads := []struct {
		path string
		body any
	}{{"/v1/traces", trace}, {"/v1/metrics", metric}}
	if event.Fault {
		logAttributes := append([]anyValue{}, attributes...)
		logPayload := map[string]any{"resourceLogs": []any{map[string]any{
			"resource": map[string]any{"attributes": resource},
			"scopeLogs": []any{map[string]any{"scope": map[string]any{"name": "telemetry-guardian"}, "logRecords": []any{map[string]any{
				"timeUnixNano": fmt.Sprint(event.Ended.UnixNano()), "severityNumber": 17, "severityText": "ERROR",
				"body": stringValue("payment authorization timed out"), "traceId": traceID, "spanId": spanID,
				"attributes": logAttributes,
			}}}},
		}}}
		payloads = append(payloads, struct {
			path string
			body any
		}{"/v1/logs", logPayload})
	}

	for _, payload := range payloads {
		if err := e.post(ctx, payload.path, payload.body); err != nil {
			return err
		}
	}
	return nil
}

type anyValue struct {
	Key   string         `json:"key"`
	Value map[string]any `json:"value"`
}

func stringValue(value string) map[string]any  { return map[string]any{"stringValue": value} }
func doubleValue(value float64) map[string]any { return map[string]any{"doubleValue": value} }

func telemetryAttributes(variant string, event telemetryEvent) []anyValue {
	attributes := []anyValue{{Key: "run.id", Value: stringValue(os.Getenv("RUN_ID"))}}
	if variant == "healthy" {
		attributes = append(attributes, anyValue{Key: "cart.value", Value: doubleValue(event.CartValue)})
		if event.Fault {
			attributes = append(attributes, anyValue{Key: "error.type", Value: stringValue("payment_timeout")})
		}
	} else {
		attributes = append(attributes, anyValue{Key: "cart.amount", Value: doubleValue(event.CartValue)})
		if event.Fault {
			attributes = append(attributes, anyValue{Key: "error.kind", Value: stringValue("timeout")})
		}
	}
	return attributes
}

func (e exporter) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.endpoint, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := e.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("OTLP %s returned %s: %s", path, response.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func randomHex(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func main() {
	variant := requiredEnv("RELEASE_VARIANT")
	if variant != "healthy" && variant != "broken" {
		log.Fatal(errors.New("RELEASE_VARIANT must be healthy or broken"))
	}
	runID := requiredEnv("RUN_ID")
	alertFile := requiredEnv("ALERT_EVENTS_FILE")
	exporter := exporter{
		endpoint: requiredEnv("OTEL_EXPORTER_OTLP_ENDPOINT"), runID: runID, variant: variant,
		client: &http.Client{Timeout: 5 * time.Second},
	}
	app := server{variant: variant, emit: exporter.emit, alertFile: alertFile}
	httpServer := &http.Server{
		Addr:              requiredEnv("LISTEN_ADDR"),
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("checkout variant=%s listening=%s", variant, httpServer.Addr)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
