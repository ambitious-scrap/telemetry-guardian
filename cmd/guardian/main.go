package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/evidence"
	"github.com/ambitious-scrap/telemetry-guardian/internal/miner"
	"github.com/ambitious-scrap/telemetry-guardian/internal/signoz"
	"github.com/ambitious-scrap/telemetry-guardian/internal/verifier"
)

const defaultContractPath = "contracts/telemetry.guardian.yaml"
const defaultVerdictPath = "verdict.json"

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}

func execute(args []string, stdout, stderr io.Writer) int {
	if err := run(args, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "guardian:", err)
		var exit *exitError
		if errors.As(err, &exit) {
			return exit.code
		}
		return 2
	}
	return 0
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: guardian mine|verify [flags]")
	}
	switch args[0] {
	case "mine":
		return runMine(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	default:
		return errors.New("usage: guardian mine|verify [flags]")
	}
}

func runMine(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("guardian mine", flag.ContinueOnError)
	flags.SetOutput(stderr)
	baseURL := flags.String("base-url", envOr("SIGNOZ_URL", ""), "SigNoz base URL")
	token := flags.String("token", envOr("SIGNOZ_TOKEN", ""), "SigNoz access token")
	dashboardID := flags.String("dashboard-id", envOr("SIGNOZ_DASHBOARD_ID", ""), "SigNoz dashboard ID")
	alertID := flags.String("alert-id", envOr("SIGNOZ_ALERT_ID", ""), "SigNoz alert ID")
	output := flags.String("output", envOr("GUARDIAN_OUTPUT", defaultContractPath), "contract output path")
	service := flags.String("service", envOr("GUARDIAN_SERVICE", miner.DefaultService), "contract service")
	release := flags.String("release", envOr("GUARDIAN_RELEASE", miner.DefaultRelease), "contract release")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *baseURL == "" {
		return errors.New("SIGNOZ_URL or --base-url is required")
	}
	if *token == "" {
		return errors.New("SIGNOZ_TOKEN or --token is required")
	}
	if *dashboardID == "" {
		return errors.New("SIGNOZ_DASHBOARD_ID or --dashboard-id is required")
	}
	if *alertID == "" {
		return errors.New("SIGNOZ_ALERT_ID or --alert-id is required")
	}
	if *output == "" {
		return errors.New("output path is required")
	}

	client, err := signoz.NewHTTPClient(signoz.Config{BaseURL: *baseURL, Token: *token, Timeout: 15 * time.Second})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	contract, err := miner.Mine(ctx, client, miner.Config{
		DashboardID: *dashboardID,
		AlertID:     *alertID,
		Service:     *service,
		Release:     *release,
	})
	if err != nil {
		return err
	}
	payload, err := contract.MarshalYAML()
	if err != nil {
		return err
	}
	parent := filepath.Dir(*output)
	if parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	if err := os.WriteFile(*output, payload, 0o644); err != nil {
		return fmt.Errorf("write contract: %w", err)
	}
	fmt.Fprintln(stdout, "contract written:", filepath.Clean(*output))
	return nil
}

func runVerify(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("guardian verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	baseURL := flags.String("base-url", envOr("SIGNOZ_URL", ""), "SigNoz base URL")
	token := flags.String("token", envOr("SIGNOZ_TOKEN", ""), "SigNoz access token")
	alertID := flags.String("alert-id", envOr("SIGNOZ_ALERT_ID", ""), "SigNoz alert resource ID")
	contractPath := flags.String("contract", envOr("GUARDIAN_CONTRACT", defaultContractPath), "contract input path")
	output := flags.String("output", envOr("GUARDIAN_VERDICT", defaultVerdictPath), "verdict output path")
	runID := flags.String("run-id", envOr("GUARDIAN_RUN_ID", ""), "unique candidate run ID")
	startText := flags.String("start", envOr("GUARDIAN_START", ""), "verification start time (RFC3339)")
	endText := flags.String("end", envOr("GUARDIAN_END", ""), "verification end time (RFC3339)")
	faultText := flags.String("fault-injected-at", envOr("GUARDIAN_FAULT_INJECTED_AT", ""), "fault injection time (RFC3339)")
	minimum := flags.Int("minimum-samples", 5, "minimum expected candidate samples")
	pollInterval := flags.Duration("poll-interval", 2*time.Second, "completeness polling interval")
	completenessTimeout := flags.Duration("completeness-timeout", 10*time.Second, "telemetry completeness deadline")
	queryTimeout := flags.Duration("query-timeout", 10*time.Second, "per-query timeout")
	if err := flags.Parse(args); err != nil {
		return &exitError{code: 3, err: err}
	}
	if *baseURL == "" || *token == "" || *alertID == "" || *runID == "" ||
		*startText == "" || *endText == "" || *faultText == "" || *contractPath == "" || *output == "" {
		return &exitError{code: 3, err: errors.New("base URL, token, alert ID, contract, output, run ID, start, end, and fault injection time are required")}
	}
	start, err := parseTime("start", *startText)
	if err != nil {
		return &exitError{code: 3, err: err}
	}
	end, err := parseTime("end", *endText)
	if err != nil {
		return &exitError{code: 3, err: err}
	}
	faultAt, err := parseTime("fault injection", *faultText)
	if err != nil {
		return &exitError{code: 3, err: err}
	}
	file, err := os.Open(*contractPath)
	if err != nil {
		return &exitError{code: 3, err: fmt.Errorf("open contract: %w", err)}
	}
	contract, loadErr := contracts.LoadYAML(file)
	closeErr := file.Close()
	if loadErr != nil {
		return &exitError{code: 3, err: loadErr}
	}
	if closeErr != nil {
		return &exitError{code: 3, err: fmt.Errorf("close contract: %w", closeErr)}
	}
	client, err := signoz.NewHTTPClient(signoz.Config{BaseURL: *baseURL, Token: *token, Timeout: *queryTimeout})
	if err != nil {
		return &exitError{code: 3, err: err}
	}
	commandTimeout := maxAlertTimeout(contract) + *completenessTimeout + *queryTimeout
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	verdict, err := verifier.Verify(ctx, client, contract, verifier.Config{
		RunID: *runID, AlertResourceID: *alertID, Start: start, End: end, FaultInjectedAt: faultAt,
		MinimumSamples: *minimum, PollInterval: *pollInterval,
		CompletenessTimeout: *completenessTimeout, QueryTimeout: *queryTimeout,
	})
	if err != nil {
		code := 2
		if errors.Is(err, contracts.ErrInvalidContract) {
			code = 3
		}
		return &exitError{code: code, err: err}
	}
	if err := writeVerdict(*output, verdict); err != nil {
		return &exitError{code: 2, err: err}
	}
	fmt.Fprintf(stdout, "verdict written: %s state=%s\n", filepath.Clean(*output), verdict.Overall)
	if code := verdict.ExitCode(); code != 0 {
		return &exitError{code: code, err: fmt.Errorf("verification result is %s", verdict.Overall)}
	}
	return nil
}

func parseTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s time", name)
	}
	return parsed, nil
}

func maxAlertTimeout(contract contracts.Contract) time.Duration {
	maximum := time.Second
	for _, check := range contract.Checks {
		if check.Type != "alert_must_fire" {
			continue
		}
		timeout, err := time.ParseDuration(check.Timeout)
		if err == nil && timeout > maximum {
			maximum = timeout
		}
	}
	return maximum
}

func writeVerdict(path string, verdict evidence.Verdict) error {
	parent := filepath.Dir(path)
	if parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create verdict directory: %w", err)
		}
	}
	temp := path + ".tmp"
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create verdict: %w", err)
	}
	if err := evidence.WriteJSON(file, verdict); err != nil {
		file.Close()
		os.Remove(temp)
		return fmt.Errorf("write verdict: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return fmt.Errorf("close verdict: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return fmt.Errorf("publish verdict: %w", err)
	}
	return nil
}

type exitError struct {
	code int
	err  error
}

func (err *exitError) Error() string { return err.err.Error() }
func (err *exitError) Unwrap() error { return err.err }

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
