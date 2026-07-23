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

	"github.com/ambitious-scrap/telemetry-guardian/internal/miner"
	"github.com/ambitious-scrap/telemetry-guardian/internal/signoz"
)

const defaultContractPath = "contracts/telemetry.guardian.yaml"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "guardian:", err)
		os.Exit(2)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "mine" {
		return errors.New("usage: guardian mine [flags]")
	}
	return runMine(args[1:], stdout, stderr)
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

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
