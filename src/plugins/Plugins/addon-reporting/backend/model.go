// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "time"

// The document model.
//
// One structure, three renderers: React draws it on screen, the PDF
// exporter lays it out as vectors, the DOCX exporter emits it as
// WordprocessingML. Everything a report can say has to be expressible
// here, and nothing a renderer does may depend on which report type
// produced the block — that is what keeps the three outputs honest.

// Level is the management altitude a section is written for.
type Level string

const (
	Strategic   Level = "strategic"
	Tactical    Level = "tactical"
	Operational Level = "operational"
)

// Status tints a value, cell or KPI.
type Status string

const (
	// StatusNeutral means "no judgement" — the renderer picks whatever
	// its default is, which for a chart series is the next colour in the
	// categorical ramp.
	StatusNeutral Status = ""
	StatusGood    Status = "good"
	StatusWarn    Status = "warn"
	StatusBad     Status = "bad"
	StatusInfo    Status = "info"
	// StatusMuted is a judgement: this datum is deliberately
	// unremarkable. It is distinct from neutral because a chart slice
	// that means "everything else" needs a colour of its own, and
	// letting it take the next ramp colour puts it next to a slice that
	// means something.
	StatusMuted Status = "muted"
)

// Band is a posture rating. The thresholds are published in every
// report's Methodology annex so a reader can challenge a score.
type Band string

const (
	BandStrong      Band = "strong"       // >= 85
	BandStable      Band = "stable"       // 70..84
	BandWatch       Band = "watch"        // 55..69
	BandAtRisk      Band = "at_risk"      // 35..54
	BandCritical    Band = "critical"     // < 35
	BandNotAssessed Band = "not_assessed" // no data, deliberately excluded
)

// Severity orders findings.
type Severity string

const (
	SevCritical Severity = "critical"
	SevHigh     Severity = "high"
	SevMedium   Severity = "medium"
	SevLow      Severity = "low"
	SevInfo     Severity = "info"
)

// Document is one generated report.
type Document struct {
	Meta      Meta       `json:"meta"`
	Scorecard *Scorecard `json:"scorecard,omitempty"`
	Sections  []Section  `json:"sections"`
	// Sources records what was consulted and whether it answered, so the
	// Methodology annex can be explicit about gaps instead of letting a
	// missing module read as a clean bill of health.
	Sources []SourceStatus `json:"sources"`
}

// Meta is the cover-page and running-header material.
type Meta struct {
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle"`
	ReportType    string `json:"report_type"`
	Organisation  string `json:"organisation"`
	Environment   string `json:"environment"`
	EnvironmentID string `json:"environment_id"`
	// Subject and SubjectLabel record the object the report was narrowed
	// to. A scoped report that does not say so on its cover is a trap:
	// every figure in it is a subset, and a reader who missed that reads
	// it as an estate-wide number.
	Subject        SubjectKind `json:"subject,omitempty"`
	SubjectLabel   string      `json:"subject_label,omitempty"`
	PeriodStart    time.Time   `json:"period_start"`
	PeriodEnd      time.Time   `json:"period_end"`
	GeneratedAt    time.Time   `json:"generated_at"`
	GeneratedBy    string      `json:"generated_by"`
	Classification string      `json:"classification"`
	Product        string      `json:"product"`
	Version        string      `json:"version"`
	Levels         []Level     `json:"levels"`
}

// SourceStatus is one data source's outcome during collection.
type SourceStatus struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	OK         bool   `json:"ok"`
	Note       string `json:"note,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// Scorecard is the strategic posture summary.
type Scorecard struct {
	Overall int          `json:"overall"`
	Band    Band         `json:"band"`
	Domains []DomainRate `json:"domains"`
	// Weighting is shown in the report so the overall figure is not a
	// black box.
	Weighting map[string]int `json:"weighting"`
}

// DomainRate is one scored domain.
type DomainRate struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Score  int    `json:"score"`
	Band   Band   `json:"band"`
	Weight int    `json:"weight"`
	Note   string `json:"note,omitempty"`
	// Basis states, in one line, what the score was computed from.
	Basis string `json:"basis,omitempty"`
}

// Section is one altitude's worth of content.
type Section struct {
	Level  Level   `json:"level"`
	Title  string  `json:"title"`
	Intent string  `json:"intent"`
	Blocks []Block `json:"blocks"`
}

// BlockKind discriminates the block union.
type BlockKind string

const (
	BlockHeading   BlockKind = "heading"
	BlockParagraph BlockKind = "paragraph"
	BlockKPIs      BlockKind = "kpis"
	BlockTable     BlockKind = "table"
	BlockChart     BlockKind = "chart"
	BlockCallout   BlockKind = "callout"
	BlockList      BlockKind = "list"
	BlockKeyValue  BlockKind = "keyvalue"
	BlockFindings  BlockKind = "findings"
	BlockScorecard BlockKind = "scorecard"
	BlockDivider   BlockKind = "divider"
	BlockPageBreak BlockKind = "pagebreak"
)

// Block is a tagged union. Only the fields relevant to Kind are set;
// JSON omits the rest so the payload the frontend receives stays small.
type Block struct {
	Kind BlockKind `json:"kind"`

	// heading
	Text  string `json:"text,omitempty"`
	Depth int    `json:"depth,omitempty"`

	// paragraph
	Lead bool `json:"lead,omitempty"`

	// kpis
	KPIs []KPI `json:"kpis,omitempty"`

	// table
	Columns []Column `json:"columns,omitempty"`
	Rows    [][]Cell `json:"rows,omitempty"`
	Caption string   `json:"caption,omitempty"`
	Note    string   `json:"note,omitempty"`

	// chart
	Chart *Chart `json:"chart,omitempty"`

	// callout
	Tone  Status `json:"tone,omitempty"`
	Title string `json:"title,omitempty"`

	// list
	Ordered bool     `json:"ordered,omitempty"`
	Items   []string `json:"items,omitempty"`

	// keyvalue
	Pairs []Pair `json:"pairs,omitempty"`

	// findings
	Findings []Finding `json:"findings,omitempty"`
}

// KPI is one headline number.
type KPI struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Unit   string `json:"unit,omitempty"`
	Target string `json:"target,omitempty"`
	Delta  string `json:"delta,omitempty"`
	// Trend is "up", "down" or "flat" — the direction of change, not
	// whether that change is good. Status carries the judgement.
	Trend  string `json:"trend,omitempty"`
	Status Status `json:"status,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

// Column describes a table column.
type Column struct {
	Label string `json:"label"`
	Align string `json:"align,omitempty"` // "", "right", "center"
	// Width is a relative weight, not an absolute size, so the same
	// table works at PDF and screen widths.
	Width float64 `json:"width,omitempty"`
	Mono  bool    `json:"mono,omitempty"`
}

// Cell is one table cell.
type Cell struct {
	Text   string `json:"text"`
	Status Status `json:"status,omitempty"`
	Mono   bool   `json:"mono,omitempty"`
	// Pill draws the text as a filled status chip rather than tinted
	// text — used for states, bands and severities.
	Pill bool `json:"pill,omitempty"`
}

// Pair is one key/value row.
type Pair struct {
	K    string `json:"k"`
	V    string `json:"v"`
	Mono bool   `json:"mono,omitempty"`
}

// Finding is an issue with an owner and a recommendation. A report that
// names a problem without saying what to do about it wastes the reader's
// time, so Recommendation is not optional in practice.
type Finding struct {
	Severity       Severity `json:"severity"`
	Title          string   `json:"title"`
	Detail         string   `json:"detail,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
	Owner          string   `json:"owner,omitempty"`
	Evidence       string   `json:"evidence,omitempty"`
}

// ChartKind mirrors the charts package without importing it here, so the
// frontend's TypeScript union has a single source it can copy.
type ChartKind string

const (
	ChartBar        ChartKind = "bar"
	ChartHBar       ChartKind = "hbar"
	ChartStackedBar ChartKind = "stackedbar"
	ChartDonut      ChartKind = "donut"
	ChartLine       ChartKind = "line"
	ChartArea       ChartKind = "area"
	ChartGauge      ChartKind = "gauge"
)

// Chart is a chart block's payload.
type Chart struct {
	Kind        ChartKind     `json:"kind"`
	Title       string        `json:"title,omitempty"`
	Series      []ChartSeries `json:"series"`
	Unit        string        `json:"unit,omitempty"`
	Max         float64       `json:"max,omitempty"`
	Target      float64       `json:"target,omitempty"`
	TargetLabel string        `json:"target_label,omitempty"`
	HasTarget   bool          `json:"has_target,omitempty"`
	ShowLegend  bool          `json:"show_legend,omitempty"`
	ShowValues  bool          `json:"show_values,omitempty"`
	Caption     string        `json:"caption,omitempty"`
	// Height is in points; 0 lets each renderer pick its default.
	Height float64 `json:"height,omitempty"`
}

// ChartSeries is one named series.
type ChartSeries struct {
	Name   string       `json:"name,omitempty"`
	Status Status       `json:"status,omitempty"`
	Points []ChartPoint `json:"points"`
}

// ChartPoint is one datum; Status overrides the categorical colour when
// the datum means something (a severity, a health state).
type ChartPoint struct {
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Status Status  `json:"status,omitempty"`
}

// --- small constructors, so builders read as content, not plumbing ---

func heading(depth int, text string) Block {
	return Block{Kind: BlockHeading, Depth: depth, Text: text}
}

func para(text string) Block { return Block{Kind: BlockParagraph, Text: text} }

func lead(text string) Block { return Block{Kind: BlockParagraph, Text: text, Lead: true} }

func bullets(items ...string) Block {
	return Block{Kind: BlockList, Items: compact(items)}
}

func numbered(items ...string) Block {
	return Block{Kind: BlockList, Ordered: true, Items: compact(items)}
}

func callout(tone Status, title, text string) Block {
	return Block{Kind: BlockCallout, Tone: tone, Title: title, Text: text}
}

func kpis(items ...KPI) Block { return Block{Kind: BlockKPIs, KPIs: items} }

func keyvalue(pairs ...Pair) Block { return Block{Kind: BlockKeyValue, Pairs: pairs} }

func divider() Block { return Block{Kind: BlockDivider} }

func pageBreak() Block { return Block{Kind: BlockPageBreak} }

func table(cols []Column, rows [][]Cell, caption, note string) Block {
	return Block{Kind: BlockTable, Columns: cols, Rows: rows, Caption: caption, Note: note}
}

func chartBlock(c Chart) Block { return Block{Kind: BlockChart, Chart: &c} }

func findings(items ...Finding) Block {
	return Block{Kind: BlockFindings, Findings: items}
}

func cell(text string) Cell { return Cell{Text: text} }

func cellStatus(text string, s Status) Cell { return Cell{Text: text, Status: s} }

func pill(text string, s Status) Cell { return Cell{Text: text, Status: s, Pill: true} }

func mono(text string) Cell { return Cell{Text: text, Mono: true} }

func compact(items []string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// hasContent reports whether a section has anything worth printing, so
// an empty section can be dropped rather than shipped as a bare heading.
func (s Section) hasContent() bool {
	for _, b := range s.Blocks {
		switch b.Kind {
		case BlockDivider, BlockPageBreak, BlockHeading:
			continue
		default:
			return true
		}
	}
	return false
}
