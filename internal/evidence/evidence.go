package evidence

import (
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"time"
)

type State string

const (
	Pass         State = "PASS"
	Fail         State = "FAIL"
	Inconclusive State = "INCONCLUSIVE"
)

type DataQuality string

const (
	Complete     DataQuality = "complete"
	Insufficient DataQuality = "insufficient"
	Stale        DataQuality = "stale"
	Error        DataQuality = "error"
)

type Record struct {
	Retrieval          string      `json:"retrieval"`
	Start              time.Time   `json:"start"`
	End                time.Time   `json:"end"`
	FaultInjectedAt    *time.Time  `json:"fault_injected_at"`
	SampleCount        int         `json:"sample_count"`
	MinimumSampleCount int         `json:"minimum_sample_count"`
	Summary            string      `json:"summary"`
	SigNozDeepLink     string      `json:"signoz_deep_link"`
	DataQuality        DataQuality `json:"data_quality"`
}

type CheckResult struct {
	State             State    `json:"state"`
	RequirementID     string   `json:"requirement_id"`
	RunID             string   `json:"run_id"`
	AffectedConsumers []string `json:"affected_consumers"`
	Evidence          Record   `json:"evidence"`
}

type Verdict struct {
	RunID        string        `json:"run_id"`
	Service      string        `json:"service"`
	Release      string        `json:"release"`
	Start        time.Time     `json:"start"`
	End          time.Time     `json:"end"`
	Overall      State         `json:"overall_state"`
	CheckResults []CheckResult `json:"checks"`
}

func NewVerdict(runID, service, release string, start, end time.Time, results []CheckResult) Verdict {
	return Verdict{
		RunID: runID, Service: service, Release: release, Start: start, End: end,
		Overall: Aggregate(results), CheckResults: results,
	}
}

func Aggregate(results []CheckResult) State {
	state := Pass
	for _, result := range results {
		if result.State == Inconclusive {
			return Inconclusive
		}
		if result.State == Fail {
			state = Fail
		}
	}
	return state
}

func (v Verdict) ExitCode() int {
	switch v.Overall {
	case Pass:
		return 0
	case Fail:
		return 1
	default:
		return 2
	}
}

func WriteJSON(writer io.Writer, verdict Verdict) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(verdict)
}

func SafeDeepLink(link string) string {
	if link == "" {
		return ""
	}
	parsed, err := url.Parse(link)
	if err != nil || parsed.User != nil {
		return ""
	}
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "auth") {
			return ""
		}
	}
	return link
}
