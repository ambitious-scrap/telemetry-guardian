package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ambitious-scrap/telemetry-guardian/internal/contracts"
	"github.com/ambitious-scrap/telemetry-guardian/internal/evidence"
)

var ErrInvalidReport = errors.New("invalid guardian report input")

type Node struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Subtitle string `json:"subtitle,omitempty"`
	State    string `json:"state"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
}

type Edge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Label    string `json:"label"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Width    int    `json:"width"`
	Vertical bool   `json:"vertical,omitempty"`
}

type Evidence struct {
	RequirementID string `json:"requirement_id"`
	State         string `json:"state"`
	Retrieval     string `json:"retrieval"`
	Window        string `json:"window"`
	SampleCount   int    `json:"sample_count"`
	Minimum       int    `json:"minimum_sample_count"`
	DataQuality   string `json:"data_quality"`
	Summary       string `json:"summary"`
	DeepLink      string `json:"deep_link,omitempty"`
}

type Document struct {
	State          string     `json:"state"`
	Classification string     `json:"classification"`
	Title          string     `json:"title"`
	Subtitle       string     `json:"subtitle"`
	RunID          string     `json:"run_id"`
	Service        string     `json:"service"`
	Release        string     `json:"release"`
	Window         string     `json:"window"`
	Nodes          []Node     `json:"nodes"`
	Edges          []Edge     `json:"edges"`
	Evidence       []Evidence `json:"evidence"`
}

type consumerInfo struct {
	consumer contracts.Consumer
	nodeID   string
}

func Build(verdict evidence.Verdict, contract contracts.Contract) (Document, error) {
	if err := contract.Validate(); err != nil {
		return Document{}, fmt.Errorf("%w: contract: %v", ErrInvalidReport, err)
	}
	consumerByID := make(map[string]contracts.Consumer, len(contract.Consumers))
	for _, consumer := range contract.Consumers {
		consumerByID[consumer.ID] = consumer
	}
	requirementByID := make(map[string]contracts.Requirement, len(contract.Checks))
	for _, requirement := range contract.Checks {
		requirementByID[requirement.ID] = requirement
	}
	if verdict.Overall != evidence.Pass && verdict.Overall != evidence.Fail && verdict.Overall != evidence.Inconclusive {
		return Document{}, fmt.Errorf("%w: unsupported verdict state %q", ErrInvalidReport, verdict.Overall)
	}

	checks := append([]evidence.CheckResult(nil), verdict.CheckResults...)
	if len(checks) != len(contract.Checks) {
		return Document{}, fmt.Errorf("%w: verdict has %d checks, contract requires %d", ErrInvalidReport, len(checks), len(contract.Checks))
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].RequirementID < checks[j].RequirementID })
	expectedState, err := aggregateReportState(checks)
	if err != nil {
		return Document{}, err
	}
	if verdict.Overall != expectedState {
		return Document{}, fmt.Errorf("%w: declared overall state %q does not match check states", ErrInvalidReport, verdict.Overall)
	}
	seenChecks := make(map[string]struct{}, len(checks))
	document := Document{
		State:          string(verdict.Overall),
		Classification: classificationForState(verdict.Overall),
		RunID:          safeText(verdict.RunID),
		Service:        safeText(verdict.Service),
		Release:        safeText(verdict.Release),
		Window:         formatWindow(verdict.Start, verdict.End),
	}
	switch verdict.Overall {
	case evidence.Pass:
		document.Title = "Telemetry contract healthy"
		document.Subtitle = "Sufficient evidence supports every required check."
	case evidence.Fail:
		document.Title = "Telemetry contract violation"
		document.Subtitle = "The following consumer dependencies are affected by verified failures."
	case evidence.Inconclusive:
		document.Title = "Verification inconclusive"
		document.Subtitle = "Guardian could not establish PASS or FAIL from sufficient evidence."
	}

	for _, check := range checks {
		requirement, ok := requirementByID[check.RequirementID]
		if !ok {
			return Document{}, fmt.Errorf("%w: check %q is not in the contract", ErrInvalidReport, check.RequirementID)
		}
		if _, seen := seenChecks[check.RequirementID]; seen {
			return Document{}, fmt.Errorf("%w: duplicate check %q", ErrInvalidReport, check.RequirementID)
		}
		seenChecks[check.RequirementID] = struct{}{}
		if check.State != evidence.Pass && check.State != evidence.Fail && check.State != evidence.Inconclusive {
			return Document{}, fmt.Errorf("%w: check %q has unsupported state %q", ErrInvalidReport, check.RequirementID, check.State)
		}
		evidenceItem := Evidence{
			RequirementID: safeText(check.RequirementID), State: string(check.State),
			Retrieval: safeText(check.Evidence.Retrieval), Window: formatWindow(check.Evidence.Start, check.Evidence.End),
			SampleCount: check.Evidence.SampleCount, Minimum: check.Evidence.MinimumSampleCount,
			DataQuality: safeText(string(check.Evidence.DataQuality)), Summary: safeText(check.Evidence.Summary),
			DeepLink: safeLink(check.Evidence.SigNozDeepLink),
		}
		document.Evidence = append(document.Evidence, evidenceItem)

		if check.State == evidence.Pass {
			continue
		}
		consumerIDs := append([]string(nil), check.AffectedConsumers...)
		if len(consumerIDs) == 0 {
			consumerIDs = append(consumerIDs, requirement.Consumers...)
		}
		if len(consumerIDs) == 0 {
			return Document{}, fmt.Errorf("%w: failed check %q has no consumer mapping", ErrInvalidReport, check.RequirementID)
		}
		requirementNodeID := "requirement:" + requirement.ID
		addNode(&document, Node{ID: requirementNodeID, Kind: "requirement", Label: safeText(requirementLabel(requirement)), Subtitle: "verified dependency", State: string(check.State)})
		for _, consumerID := range consumerIDs {
			consumer, ok := consumerByID[consumerID]
			if !ok {
				return Document{}, fmt.Errorf("%w: check %q references unknown consumer %q", ErrInvalidReport, check.RequirementID, consumerID)
			}
			info := consumerInfo{consumer: consumer, nodeID: "consumer:" + consumer.ID}
			addConsumerGraph(&document, info, requirementNodeID, check.State)
		}
	}
	finalizeNodes(&document)
	return document, nil
}

func aggregateReportState(checks []evidence.CheckResult) (evidence.State, error) {
	if len(checks) == 0 {
		return "", fmt.Errorf("%w: verdict has no check states", ErrInvalidReport)
	}
	state := evidence.Pass
	for _, check := range checks {
		switch check.State {
		case evidence.Pass:
		case evidence.Fail:
			if state != evidence.Inconclusive {
				state = evidence.Fail
			}
		case evidence.Inconclusive:
			state = evidence.Inconclusive
		default:
			return "", fmt.Errorf("%w: check %q has unsupported state", ErrInvalidReport, check.RequirementID)
		}
	}
	return state, nil
}

func RenderHTML(writer io.Writer, document Document) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode report data: %w", err)
	}
	page := struct {
		Document Document
		Data     template.JS
	}{Document: document, Data: template.JS(data)}
	var output bytes.Buffer
	if err := htmlTemplate.Execute(&output, page); err != nil {
		return fmt.Errorf("render report: %w", err)
	}
	_, err = writer.Write(output.Bytes())
	return err
}

func Markdown(document Document) string {
	var output strings.Builder
	output.WriteString(document.Classification + "\n\n")
	output.WriteString("### Telemetry Guardian\n\n")
	output.WriteString("- State: `" + document.State + "`\n")
	output.WriteString("- Service: `" + document.Service + "`\n")
	output.WriteString("- Release: `" + document.Release + "`\n")
	output.WriteString("- Verification window: `" + document.Window + "`\n")
	if document.State == string(evidence.Pass) {
		output.WriteString("\nAll required telemetry checks passed with sufficient evidence.\n")
	} else {
		output.WriteString("\n| Requirement | State | Samples | Data quality | Evidence |\n")
		output.WriteString("|---|---|---:|---|---|\n")
		for _, item := range document.Evidence {
			link := "not provided"
			if item.DeepLink != "" {
				link = "[SigNoz evidence](" + item.DeepLink + ")"
			}
			output.WriteString("| `" + item.RequirementID + "` | **" + item.State + "** | " + fmt.Sprint(item.SampleCount) + " | `" + item.DataQuality + "` | " + link + " |\n")
		}
	}
	return output.String()
}

func classificationForState(state evidence.State) string {
	switch state {
	case evidence.Pass:
		return "PASS"
	case evidence.Fail:
		return "TELEMETRY_CONTRACT_VIOLATION"
	default:
		return "VERIFICATION_INCONCLUSIVE"
	}
}

func Classification(exitCode int) (string, bool) {
	switch exitCode {
	case 0:
		return "PASS", true
	case 1:
		return "TELEMETRY_CONTRACT_VIOLATION", true
	case 2:
		return "VERIFICATION_INCONCLUSIVE", true
	case 3:
		return "INVALID_GUARDIAN_CONFIGURATION", true
	default:
		return "", false
	}
}

func requirementLabel(requirement contracts.Requirement) string {
	switch requirement.Type {
	case "required_field":
		return "Required field: " + requirement.Field
	case "required_operation":
		return "Required operation: " + requirement.Operation
	case "alert_must_fire":
		return "Alert must fire: " + requirement.AlertID
	default:
		return requirement.ID
	}
}

func addConsumerGraph(document *Document, info consumerInfo, requirementID string, state evidence.State) {
	consumer := info.consumer
	if consumer.Type == "dashboard_panel" {
		panelID := info.nodeID
		dashboardID := "dashboard:" + consumer.Source.DashboardID
		addNode(document, Node{ID: dashboardID, Kind: "dashboard", Label: safeText(consumer.Source.DashboardID), Subtitle: "dashboard", State: string(state)})
		addNode(document, Node{ID: panelID, Kind: "panel", Label: safeText(consumer.Name), Subtitle: "dashboard panel " + safeText(consumer.Source.PanelID), State: string(state)})
		addEdge(document, Edge{From: panelID, To: dashboardID, Label: "PART_OF"})
		addEdge(document, Edge{From: panelID, To: requirementID, Label: "REQUIRED_BY"})
		addEdge(document, Edge{From: requirementID, To: panelID, Label: "BREAKS"})
		return
	}
	alertID := info.nodeID
	addNode(document, Node{ID: alertID, Kind: "alert", Label: safeText(consumer.Name), Subtitle: "alert " + safeText(consumer.Source.AlertID), State: string(state)})
	addEdge(document, Edge{From: alertID, To: requirementID, Label: "REQUIRED_BY"})
	addEdge(document, Edge{From: requirementID, To: alertID, Label: "BREAKS"})
}

func addNode(document *Document, node Node) {
	for _, existing := range document.Nodes {
		if existing.ID == node.ID {
			return
		}
	}
	document.Nodes = append(document.Nodes, node)
}

func addEdge(document *Document, edge Edge) {
	for _, existing := range document.Edges {
		if existing == edge {
			return
		}
	}
	document.Edges = append(document.Edges, edge)
}

func finalizeNodes(document *Document) {
	rank := map[string]int{"dashboard": 0, "panel": 1, "requirement": 2, "alert": 3}
	sort.Slice(document.Nodes, func(i, j int) bool {
		if rank[document.Nodes[i].Kind] == rank[document.Nodes[j].Kind] {
			return document.Nodes[i].ID < document.Nodes[j].ID
		}
		return rank[document.Nodes[i].Kind] < rank[document.Nodes[j].Kind]
	})
	positions := make(map[string]Node, len(document.Nodes))
	counts := make(map[string]int)
	for i := range document.Nodes {
		node := &document.Nodes[i]
		index := counts[node.Kind]
		counts[node.Kind] = index + 1
		switch node.Kind {
		case "dashboard":
			node.X, node.Y = 40, 72
		case "panel":
			node.X, node.Y = 40, 200+(index*82)
		case "requirement":
			node.X, node.Y = 510, 144+(index*88)
		case "alert":
			node.X, node.Y = 944, 200+(index*82)
		}
		positions[node.ID] = *node
	}
	for i := range document.Edges {
		from, fromOK := positions[document.Edges[i].From]
		to, toOK := positions[document.Edges[i].To]
		if !fromOK || !toOK {
			continue
		}
		fromWidth := 250
		if from.Kind == "requirement" {
			fromWidth = 300
		}
		toWidth := 250
		if to.Kind == "requirement" {
			toWidth = 300
		}
		fromRight, toRight := from.X+fromWidth, to.X+toWidth
		if fromRight <= to.X {
			document.Edges[i].X = fromRight
			document.Edges[i].Width = to.X - fromRight
		} else if toRight <= from.X {
			document.Edges[i].X = toRight
			document.Edges[i].Width = from.X - toRight
		} else {
			document.Edges[i].Vertical = true
			document.Edges[i].X = from.X + (fromWidth / 2)
			document.Edges[i].Y = min(from.Y, to.Y) + 72
			document.Edges[i].Width = 1
		}
		if !document.Edges[i].Vertical {
			document.Edges[i].Y = (from.Y + to.Y) / 2
		}
	}
	sort.Slice(document.Edges, func(i, j int) bool {
		if document.Edges[i].From == document.Edges[j].From {
			if document.Edges[i].To == document.Edges[j].To {
				return document.Edges[i].Label < document.Edges[j].Label
			}
			return document.Edges[i].To < document.Edges[j].To
		}
		return document.Edges[i].From < document.Edges[j].From
	})
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func formatWindow(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return "not available"
	}
	return start.UTC().Format(time.RFC3339) + " → " + end.UTC().Format(time.RFC3339)
}

var sensitiveText = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~-]+|(?i)(token|secret|authorization|api[_-]?key)(\s*[:=]\s*)[^\s,;]+`)

func safeText(value string) string {
	value = sensitiveText.ReplaceAllString(value, "$1[redacted]")
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(value, "/") || strings.Contains(value, "/Users/") || strings.Contains(value, "/home/") {
		return "[redacted path]"
	}
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	return strings.TrimSpace(value)
}

func safeLink(link string) string {
	link = strings.TrimSpace(link)
	parsed, err := url.Parse(link)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	if sensitiveText.MatchString(link) || strings.Contains(strings.ToLower(parsed.Path), "token") || strings.Contains(strings.ToLower(parsed.Path), "secret") {
		return ""
	}
	return link
}

var htmlTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Telemetry Guardian report</title>
<style>
:root{color-scheme:dark;--bg:#0b1020;--surface:#121a2b;--surface-2:#19243a;--line:#2b3a55;--text:#edf3ff;--muted:#9eacc5;--pass:#79d6a2;--fail:#ff8d91;--warn:#f4c66b;--focus:#8ab4ff;--mono:"JetBrains Mono",ui-monospace,SFMono-Regular,Menlo,monospace;--sans:"IBM Plex Sans",Inter,system-ui,sans-serif}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:15px/1.5 var(--sans)}main{width:1280px;min-height:720px;margin:0 auto;padding:32px 40px}.skip{position:absolute;left:-999px}.skip:focus{left:16px;top:16px;z-index:5;background:var(--surface);padding:8px 12px}.eyebrow,.mono{font-family:var(--mono);letter-spacing:.04em}.eyebrow{font-size:12px;color:var(--muted);text-transform:uppercase}.header{display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:20px}.header h1{font:700 28px/1.2 var(--mono);margin:6px 0}.subtitle{color:var(--muted);margin:0}.state{border:1px solid var(--line);border-radius:999px;padding:8px 14px;font:700 13px var(--mono)}.state-pass{color:var(--pass)}.state-fail{color:var(--fail)}.state-inconclusive{color:var(--warn)}.meta{display:flex;gap:28px;color:var(--muted);font-size:13px;margin-bottom:20px}.meta strong{color:var(--text);font-family:var(--mono);font-weight:500}.graph{position:relative;height:414px;border:1px solid var(--line);border-radius:12px;background:linear-gradient(135deg,var(--surface),#0f1728);overflow:hidden}.graph:before{content:"DEPENDENCY VIEW · RELATIONSHIPS ONLY";position:absolute;top:14px;left:18px;color:var(--muted);font:11px var(--mono);letter-spacing:.08em}.node{position:absolute;width:250px;min-height:72px;padding:12px 14px;border:1px solid var(--line);border-radius:8px;background:var(--surface-2);box-shadow:0 8px 20px #0003}.node.requirement{width:300px;border-color:var(--fail)}.node-label{font-weight:700}.node-sub{color:var(--muted);font:12px var(--mono);margin-top:4px}.consumer-node{cursor:pointer;text-align:left;font:inherit;color:inherit}.consumer-node:hover,.consumer-node:focus-visible{border-color:var(--focus)}.consumer-node:focus-visible{outline:3px solid var(--focus);outline-offset:3px}.edge{position:absolute;color:var(--muted);font:10px var(--mono);letter-spacing:.06em;white-space:nowrap}.edge-line{height:1px;background:var(--line);width:100%;margin-bottom:4px}.edge span{position:absolute;left:50%;top:4px;transform:translateX(-50%);padding:0 4px;background:var(--surface)}.edge-vertical .edge-line{height:56px;width:1px;margin-left:0}.edge-vertical span{left:12px;top:16px;transform:none}.edge-vertical{transform:translateX(-50%)}.legend{display:flex;gap:18px;margin:10px 0;color:var(--muted);font:11px var(--mono);flex-wrap:wrap}.legend span{color:var(--text)}.controls{display:flex;justify-content:space-between;align-items:center;margin-top:16px}.button{min-height:44px;padding:10px 14px;border:1px solid var(--line);border-radius:7px;background:var(--surface-2);color:var(--text);font:600 13px var(--mono);cursor:pointer}.button:hover{border-color:var(--focus)}.button:focus-visible{outline:3px solid var(--focus);outline-offset:3px}.evidence-drawer{position:fixed;right:0;top:0;width:420px;height:100vh;overflow:auto;padding:24px;background:var(--surface);border-left:1px solid var(--line);box-shadow:-12px 0 32px #0008;z-index:4}.evidence-drawer[hidden]{display:none}.evidence-item{border-top:1px solid var(--line);padding:16px 0}.evidence-item h3{font:600 14px var(--mono);margin:0 0 10px}.evidence-item dt{color:var(--muted);font-size:12px}.evidence-item dd{margin:0 0 8px;overflow-wrap:anywhere}.evidence-link{color:var(--focus)}.healthy{display:flex;align-items:center;justify-content:center;height:414px;border:1px solid var(--line);border-radius:12px;background:var(--surface);text-align:center}.healthy strong{display:block;font:700 24px var(--mono);color:var(--pass)}.healthy p{color:var(--muted)}@media(max-width:1280px){main{width:100%;overflow-x:auto}}@media(prefers-reduced-motion:reduce){*,*:before,*:after{scroll-behavior:auto!important;transition-duration:.01ms!important;animation-duration:.01ms!important;animation-iteration-count:1!important}}
</style></head><body data-state="{{.Document.State}}" class="state-{{.Document.State}}">
<a class="skip" href="#main">Skip to report</a><main id="main" tabindex="-1"><header class="header"><div><div class="eyebrow">Telemetry Guardian · consumer evidence console</div><h1>{{.Document.Title}}</h1><p class="subtitle">{{.Document.Subtitle}}</p></div><div class="state" aria-label="Verification state">{{.Document.State}} · {{.Document.Classification}}</div></header>
<div class="meta"><span>service <strong>{{.Document.Service}}</strong></span><span>release <strong>{{.Document.Release}}</strong></span><span>window <strong>{{.Document.Window}}</strong></span></div>
{{if eq .Document.State "PASS"}}<section class="healthy" aria-labelledby="healthy-title"><div><strong id="healthy-title">PASS · contract healthy</strong><p>No required telemetry dependency is reported as broken.</p></div></section>{{else}}<section class="graph" aria-label="Deterministic consumer dependency graph">{{range .Document.Nodes}}<article class="node {{.Kind}}{{if or (eq .Kind "panel") (eq .Kind "alert")}} consumer-node{{end}}" style="left:{{.X}}px;top:{{.Y}}px" data-node-id="{{.ID}}"{{if or (eq .Kind "panel") (eq .Kind "alert")}} role="button" tabindex="0" data-open-consumer aria-controls="evidence-drawer" aria-label="Inspect evidence for {{.Label}}"{{end}}><div class="node-label">{{.Label}}</div><div class="node-sub">{{.Subtitle}}</div></article>{{end}}{{range .Document.Edges}}<div class="edge{{if .Vertical}} edge-vertical{{end}}" style="left:{{.X}}px;top:{{.Y}}px;width:{{.Width}}px" data-from="{{.From}}" data-to="{{.To}}"><div class="edge-line"></div><span>{{.Label}}</span></div>{{end}}</section><div class="legend"><span>Failed dependency</span><span>BREAKS known dependency</span><span>REQUIRED_BY consumer mapping</span><span>PART_OF resource nesting</span></div>{{end}}
<div class="controls"><span class="mono">run {{.Document.RunID}}</span><button class="button" type="button" data-open-evidence aria-controls="evidence-drawer" aria-expanded="false">Inspect evidence</button></div></main>
<aside id="evidence-drawer" class="evidence-drawer" hidden role="dialog" aria-modal="true" aria-labelledby="evidence-title" tabindex="-1"><div class="controls"><h2 id="evidence-title">Evidence</h2><button class="button" type="button" data-close-evidence aria-label="Close evidence drawer">Close</button></div>{{range .Document.Evidence}}<section class="evidence-item"><h3>{{.RequirementID}} · {{.State}}</h3><dl><dt>Query or retrieval</dt><dd>{{.Retrieval}}</dd><dt>Verification window</dt><dd class="mono">{{.Window}}</dd><dt>Sample count</dt><dd>{{.SampleCount}} / minimum {{.Minimum}}</dd><dt>Data quality</dt><dd>{{.DataQuality}}</dd><dt>Result summary</dt><dd>{{.Summary}}</dd>{{if .DeepLink}}<dt>SigNoz evidence</dt><dd><a class="evidence-link" href="{{.DeepLink}}" rel="noreferrer">Open safe deep link</a></dd>{{end}}</dl></section>{{end}}</aside>
<script type="application/json" id="report-data">{{.Data}}</script><script>(function(){var drawer=document.getElementById('evidence-drawer'),open=document.querySelector('[data-open-evidence]'),close=document.querySelector('[data-close-evidence]'),consumerNodes=document.querySelectorAll('[data-open-consumer]'),previous;function show(trigger){previous=trigger||document.activeElement;drawer.hidden=false;open.setAttribute('aria-expanded','true');drawer.focus();}function hide(){drawer.hidden=true;open.setAttribute('aria-expanded','false');if(previous){previous.focus();}}open.addEventListener('click',function(){show(open);});consumerNodes.forEach(function(node){node.addEventListener('click',function(){show(node);});node.addEventListener('keydown',function(event){if(event.key==='Enter'||event.key===' '){event.preventDefault();show(node);}});});close.addEventListener('click',hide);document.addEventListener('keydown',function(event){if(event.key==='Escape'&&!drawer.hidden){hide();}});})();</script></body></html>`))
