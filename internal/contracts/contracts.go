package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const APIVersion = "telemetry.guardian/v1"

var ErrInvalidContract = errors.New("invalid telemetry contract")

var jsonPathPattern = regexp.MustCompile(`^\$(?:\.[A-Za-z_][A-Za-z0-9_]*|\[[0-9]+\])+$`)

// Contract is the stable, transport-independent mined contract.
type Contract struct {
	APIVersion string        `yaml:"apiVersion"`
	Service    string        `yaml:"service"`
	Release    string        `yaml:"release"`
	Consumers  []Consumer    `yaml:"consumers"`
	Checks     []Requirement `yaml:"checks"`
}

type Consumer struct {
	ID          string           `yaml:"id"`
	Type        string           `yaml:"type"`
	Name        string           `yaml:"name"`
	Owner       string           `yaml:"owner,omitempty"`
	Criticality string           `yaml:"criticality,omitempty"`
	Source      Source           `yaml:"source"`
	Requires    []RequirementRef `yaml:"requires"`
}

type Source struct {
	DashboardID string `yaml:"dashboard_id,omitempty"`
	PanelID     string `yaml:"panel_id,omitempty"`
	AlertID     string `yaml:"alert_id,omitempty"`
}

type RequirementRef struct {
	ID         string `yaml:"id"`
	SourcePath string `yaml:"source_path"`
}

type Requirement struct {
	ID          string   `yaml:"id"`
	Type        string   `yaml:"type"`
	Signal      string   `yaml:"signal,omitempty"`
	Field       string   `yaml:"field,omitempty"`
	Operation   string   `yaml:"operation,omitempty"`
	AlertID     string   `yaml:"alert_id,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Filter      string   `yaml:"filter,omitempty"`
	Filters     []string `yaml:"filters,omitempty"`
	SourcePath  string   `yaml:"source_path,omitempty"`
	SourcePaths []string `yaml:"source_paths,omitempty"`
	Consumers   []string `yaml:"consumers"`
}

func New(service, release string) Contract {
	return Contract{APIVersion: APIVersion, Service: service, Release: release}
}

func LoadYAML(reader io.Reader) (Contract, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	var contract Contract
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, fmt.Errorf("%w: decode YAML: %v", ErrInvalidContract, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Contract{}, fmt.Errorf("%w: multiple YAML documents", ErrInvalidContract)
		}
		return Contract{}, fmt.Errorf("%w: decode YAML: %v", ErrInvalidContract, err)
	}
	contract.Normalize()
	if err := contract.Validate(); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func (c Contract) Validate() error {
	if c.APIVersion != APIVersion {
		return contractError("apiVersion", "must be "+APIVersion)
	}
	if c.Service == "" || c.Release == "" {
		return contractError("contract", "service and release are required")
	}
	if len(c.Consumers) == 0 {
		return contractError("consumers", "at least one consumer is required")
	}
	if len(c.Checks) == 0 {
		return contractError("checks", "at least one check is required")
	}

	consumerIDs := make(map[string]struct{}, len(c.Consumers))
	for i, consumer := range c.Consumers {
		path := fmt.Sprintf("consumers[%d]", i)
		if consumer.ID == "" || consumer.Name == "" || consumer.Type == "" {
			return contractError(path, "id, type, and name are required")
		}
		if _, exists := consumerIDs[consumer.ID]; exists {
			return contractError(path+".id", "duplicate consumer id")
		}
		consumerIDs[consumer.ID] = struct{}{}
		switch consumer.Type {
		case "dashboard_panel":
			if consumer.Source.DashboardID == "" || consumer.Source.PanelID == "" {
				return contractError(path+".source", "dashboard_id and panel_id are required")
			}
		case "alert":
			if consumer.Source.AlertID == "" {
				return contractError(path+".source", "alert_id is required")
			}
		default:
			return contractError(path+".type", "unsupported consumer type")
		}
		if len(consumer.Requires) == 0 {
			return contractError(path+".requires", "at least one requirement is required")
		}
		seenRequirements := make(map[string]struct{}, len(consumer.Requires))
		for j, required := range consumer.Requires {
			if required.ID == "" {
				return contractError(fmt.Sprintf("%s.requires[%d].id", path, j), "required")
			}
			if _, exists := seenRequirements[required.ID]; exists {
				return contractError(fmt.Sprintf("%s.requires[%d].id", path, j), "duplicate requirement reference")
			}
			seenRequirements[required.ID] = struct{}{}
			if !IsJSONPath(required.SourcePath) {
				return contractError(fmt.Sprintf("%s.requires[%d].source_path", path, j), "malformed JSON path")
			}
		}
	}

	checkIDs := make(map[string]struct{}, len(c.Checks))
	for i, check := range c.Checks {
		path := fmt.Sprintf("checks[%d]", i)
		if check.ID == "" || check.Type == "" {
			return contractError(path, "id and type are required")
		}
		if _, exists := checkIDs[check.ID]; exists {
			return contractError(path+".id", "duplicate requirement id")
		}
		checkIDs[check.ID] = struct{}{}
		switch check.Type {
		case "required_field":
			if check.Signal == "" || check.Field == "" {
				return contractError(path, "signal and field are required")
			}
		case "required_operation":
			if check.Signal == "" || check.Operation == "" {
				return contractError(path, "signal and operation are required")
			}
		case "alert_must_fire":
			if check.AlertID == "" || check.Timeout == "" {
				return contractError(path, "alert_id and timeout are required")
			}
			timeout, err := time.ParseDuration(check.Timeout)
			if err != nil || timeout <= 0 {
				return contractError(path+".timeout", "must be a positive duration")
			}
		default:
			return contractError(path+".type", "unsupported requirement type")
		}
		paths := check.paths()
		if len(paths) == 0 {
			return contractError(path+".source_path", "at least one source path is required")
		}
		for j, sourcePath := range paths {
			if !IsJSONPath(sourcePath) {
				return contractError(fmt.Sprintf("%s.source_paths[%d]", path, j), "malformed JSON path")
			}
		}
		if len(check.Consumers) == 0 {
			return contractError(path+".consumers", "at least one consumer is required")
		}
		for j, consumerID := range check.Consumers {
			if _, exists := consumerIDs[consumerID]; !exists {
				return contractError(fmt.Sprintf("%s.consumers[%d]", path, j), "unknown consumer")
			}
		}
		for j, filter := range check.filters() {
			if filter == "" {
				return contractError(fmt.Sprintf("%s.filters[%d]", path, j), "empty filter")
			}
		}
	}

	for i, consumer := range c.Consumers {
		for j, required := range consumer.Requires {
			if _, exists := checkIDs[required.ID]; !exists {
				return contractError(fmt.Sprintf("consumers[%d].requires[%d].id", i, j), "unknown requirement")
			}
		}
	}
	return nil
}

func (c Contract) MarshalYAML() ([]byte, error) {
	normalized := c.clone()
	normalized.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}

	var b strings.Builder
	line := func(indent int, text string) {
		b.WriteString(strings.Repeat("  ", indent))
		b.WriteString(text)
		b.WriteByte('\n')
	}
	scalar := func(value string) string { return yamlScalar(value) }

	line(0, "apiVersion: "+scalar(normalized.APIVersion))
	line(0, "service: "+scalar(normalized.Service))
	line(0, "release: "+scalar(normalized.Release))
	line(0, "")
	line(0, "consumers:")
	for _, consumer := range normalized.Consumers {
		line(1, "- id: "+scalar(consumer.ID))
		line(2, "type: "+scalar(consumer.Type))
		line(2, "name: "+scalar(consumer.Name))
		if consumer.Owner != "" {
			line(2, "owner: "+scalar(consumer.Owner))
		}
		if consumer.Criticality != "" {
			line(2, "criticality: "+scalar(consumer.Criticality))
		}
		line(2, "source:")
		if consumer.Source.DashboardID != "" {
			line(3, "dashboard_id: "+scalar(consumer.Source.DashboardID))
		}
		if consumer.Source.PanelID != "" {
			line(3, "panel_id: "+scalar(consumer.Source.PanelID))
		}
		if consumer.Source.AlertID != "" {
			line(3, "alert_id: "+scalar(consumer.Source.AlertID))
		}
		line(2, "requires:")
		for _, required := range consumer.Requires {
			line(3, "- id: "+scalar(required.ID))
			line(4, "source_path: "+scalar(required.SourcePath))
		}
	}
	line(0, "")
	line(0, "checks:")
	for _, check := range normalized.Checks {
		line(1, "- id: "+scalar(check.ID))
		line(2, "type: "+scalar(check.Type))
		if check.Signal != "" {
			line(2, "signal: "+scalar(check.Signal))
		}
		if check.Field != "" {
			line(2, "field: "+scalar(check.Field))
		}
		if check.Operation != "" {
			line(2, "operation: "+scalar(check.Operation))
		}
		if check.AlertID != "" {
			line(2, "alert_id: "+scalar(check.AlertID))
		}
		if check.Timeout != "" {
			line(2, "timeout: "+scalar(check.Timeout))
		}
		if check.SourcePath != "" {
			line(2, "source_path: "+scalar(check.SourcePath))
		}
		if len(check.SourcePaths) > 1 {
			line(2, "source_paths:")
			for _, sourcePath := range check.SourcePaths {
				line(3, "- "+scalar(sourcePath))
			}
		}
		filters := check.filters()
		if len(filters) == 1 {
			line(2, "filter: "+scalar(filters[0]))
		} else if len(filters) > 1 {
			line(2, "filters:")
			for _, filter := range filters {
				line(3, "- "+scalar(filter))
			}
		}
		line(2, "consumers:")
		for _, consumerID := range check.Consumers {
			line(3, "- "+scalar(consumerID))
		}
	}
	return []byte(b.String()), nil
}

func (c Contract) YAML() ([]byte, error) { return c.MarshalYAML() }

func WriteYAML(w io.Writer, c Contract) error {
	payload, err := c.MarshalYAML()
	if err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func (c *Contract) Normalize() {
	if c.APIVersion == "" {
		c.APIVersion = APIVersion
	}
	for i := range c.Consumers {
		sort.Slice(c.Consumers[i].Requires, func(left, right int) bool {
			if c.Consumers[i].Requires[left].ID == c.Consumers[i].Requires[right].ID {
				return c.Consumers[i].Requires[left].SourcePath < c.Consumers[i].Requires[right].SourcePath
			}
			return c.Consumers[i].Requires[left].ID < c.Consumers[i].Requires[right].ID
		})
	}
	sort.Slice(c.Consumers, func(left, right int) bool { return c.Consumers[left].ID < c.Consumers[right].ID })
	for i := range c.Checks {
		paths := c.Checks[i].paths()
		sort.Strings(paths)
		c.Checks[i].SourcePaths = unique(paths)
		if len(paths) > 0 {
			c.Checks[i].SourcePath = paths[0]
		}
		filters := c.Checks[i].filters()
		sort.Strings(filters)
		filters = unique(filters)
		c.Checks[i].Filter = ""
		if len(filters) == 1 {
			c.Checks[i].Filter = filters[0]
		}
		c.Checks[i].Filters = filters
		sort.Strings(c.Checks[i].Consumers)
	}
	sort.SliceStable(c.Checks, func(left, right int) bool {
		leftRank, rightRank := checkRank(c.Checks[left]), checkRank(c.Checks[right])
		if leftRank == rightRank {
			return c.Checks[left].ID < c.Checks[right].ID
		}
		return leftRank < rightRank
	})
}

func (r Requirement) paths() []string {
	paths := make([]string, 0, len(r.SourcePaths)+1)
	if r.SourcePath != "" {
		paths = append(paths, r.SourcePath)
	}
	paths = append(paths, r.SourcePaths...)
	return unique(paths)
}

func (r Requirement) filters() []string {
	filters := make([]string, 0, len(r.Filters)+1)
	if r.Filter != "" {
		filters = append(filters, r.Filter)
	}
	filters = append(filters, r.Filters...)
	return unique(filters)
}

func (c Contract) clone() Contract {
	copy := c
	copy.Consumers = append([]Consumer(nil), c.Consumers...)
	for i := range copy.Consumers {
		copy.Consumers[i].Requires = append([]RequirementRef(nil), c.Consumers[i].Requires...)
	}
	copy.Checks = append([]Requirement(nil), c.Checks...)
	for i := range copy.Checks {
		copy.Checks[i].Filters = append([]string(nil), c.Checks[i].Filters...)
		copy.Checks[i].SourcePaths = append([]string(nil), c.Checks[i].SourcePaths...)
		copy.Checks[i].Consumers = append([]string(nil), c.Checks[i].Consumers...)
	}
	return copy
}

func IsJSONPath(path string) bool { return jsonPathPattern.MatchString(path) }

func yamlScalar(value string) string {
	if value != "" {
		valid := true
		for _, character := range value {
			if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && !strings.ContainsRune("-_. /", character) {
				valid = false
				break
			}
		}
		if valid && !strings.HasPrefix(value, "-") && !strings.HasPrefix(value, ".") {
			return value
		}
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func checkRank(requirement Requirement) int {
	switch requirement.Type {
	case "required_field":
		if requirement.Field == "cart.value" {
			return 10
		}
		return 20
	case "required_operation":
		return 30
	case "alert_must_fire":
		return 40
	default:
		return 50
	}
}

func unique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func contractError(path, message string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalidContract, path, message)
}
