// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The document model, mirrored from backend/model.go.
//
// This is a deliberate hand-mirror rather than a generated type: the Go
// side is the source of truth, and keeping the TypeScript small and
// readable matters more here than keeping it automatically in step. Any
// field added there and not here simply does not render.

export type Level = 'strategic' | 'tactical' | 'operational' | 'summary' | 'annex';

export type Status = '' | 'good' | 'warn' | 'bad' | 'info' | 'muted';

export type Band = 'strong' | 'stable' | 'watch' | 'at_risk' | 'critical' | 'not_assessed';

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info';

export type ChartKind = 'bar' | 'hbar' | 'stackedbar' | 'donut' | 'line' | 'area' | 'gauge';

export type BlockKind =
  | 'heading'
  | 'paragraph'
  | 'kpis'
  | 'table'
  | 'chart'
  | 'callout'
  | 'list'
  | 'keyvalue'
  | 'findings'
  | 'scorecard'
  | 'divider'
  | 'pagebreak';

/** The object kinds a report can be narrowed to. Mirrors `SubjectKind`. */
export type SubjectKind = 'container' | 'stack' | 'image' | 'volume' | 'network' | 'user' | 'group';

/** One row in the studio's scope picker. */
export type SubjectOption = {
  id: string;
  name: string;
  hint?: string;
};

export type SubjectsResponse = {
  kind: SubjectKind;
  items: SubjectOption[];
};

export type Meta = {
  title: string;
  subtitle: string;
  report_type: string;
  organisation: string;
  environment: string;
  environment_id: string;
  /** Set only when the report was narrowed to one object. */
  subject?: SubjectKind;
  subject_label?: string;
  period_start: string;
  period_end: string;
  generated_at: string;
  generated_by: string;
  classification: string;
  product: string;
  version: string;
  levels: Level[];
};

export type SourceStatus = {
  key: string;
  label: string;
  ok: boolean;
  note?: string;
  duration_ms: number;
};

export type DomainRate = {
  key: string;
  label: string;
  score: number;
  band: Band;
  weight: number;
  note?: string;
  basis?: string;
};

export type Scorecard = {
  overall: number;
  band: Band;
  domains: DomainRate[];
  weighting: Record<string, number>;
};

export type KPI = {
  label: string;
  value: string;
  unit?: string;
  target?: string;
  delta?: string;
  trend?: string;
  status?: Status;
  hint?: string;
};

export type Column = {
  label: string;
  align?: 'right' | 'center';
  width?: number;
  mono?: boolean;
};

export type Cell = {
  text: string;
  status?: Status;
  mono?: boolean;
  pill?: boolean;
};

export type Pair = { k: string; v: string; mono?: boolean };

export type Finding = {
  severity: Severity;
  title: string;
  detail?: string;
  recommendation?: string;
  owner?: string;
  evidence?: string;
};

export type ChartPoint = { label: string; value: number; status?: Status };

export type ChartSeries = { name?: string; status?: Status; points: ChartPoint[] };

export type Chart = {
  kind: ChartKind;
  title?: string;
  series: ChartSeries[];
  unit?: string;
  max?: number;
  target?: number;
  target_label?: string;
  has_target?: boolean;
  show_legend?: boolean;
  show_values?: boolean;
  caption?: string;
  height?: number;
};

export type Block = {
  kind: BlockKind;
  text?: string;
  depth?: number;
  lead?: boolean;
  kpis?: KPI[];
  columns?: Column[];
  rows?: Cell[][];
  caption?: string;
  note?: string;
  chart?: Chart;
  tone?: Status;
  title?: string;
  ordered?: boolean;
  items?: string[];
  pairs?: Pair[];
  findings?: Finding[];
};

export type Section = {
  level: Level;
  title: string;
  intent: string;
  blocks: Block[];
};

export type ReportDocument = {
  meta: Meta;
  scorecard?: Scorecard;
  sections: Section[];
  sources: SourceStatus[];
};

export type StoredReport = {
  id: string;
  report_type: string;
  title: string;
  environment_id: string;
  environment: string;
  period_start: string;
  period_end: string;
  levels: Level[];
  status: string;
  overall_score: number;
  overall_band: Band;
  headline: string;
  generated_by: string;
  size_bytes: number;
  created_at: string;
  document?: ReportDocument;
};

export * from './types-catalog';
export * from './types-schedule';
