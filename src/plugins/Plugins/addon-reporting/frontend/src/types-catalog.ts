// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Level, StoredReport, SubjectKind } from './types';

export type ReportOption = { key: string; default: boolean };

export type ReportType = {
  key: string;
  category: string;
  levels: Level[];
  sources: string[];
  icon: string;
  default_days: number;
  point_in_time: boolean;
  /**
   * True when at least one of this type's sources actually varies by
   * environment. Derived in the backend catalog from `sources`, so the
   * studio only offers the environment picker where the choice changes
   * the document — an access review or a platform inventory reads the
   * same whichever environment is selected.
   */
  environment_scoped: boolean;
  /**
   * The object kinds this report can be narrowed to, in the order the
   * studio offers them. Absent or empty means the report only ever
   * describes the whole estate.
   */
  subjects?: SubjectKind[];
  options?: ReportOption[];
  /**
   * The slug of the plugin that contributed this type, absent on the
   * built-in ones. Set means every label below resolves against that
   * plugin's own i18n namespace rather than `reporting` — see
   * pages/catalog-labels.ts.
   */
  provider?: string;
  /** The i18n namespace the four label keys resolve against. */
  label_ns?: string;
  label_key?: string;
  description_key?: string;
  category_label_key?: string;
  category_description_key?: string;
  /**
   * The English cover strings the provider declared. The last resort
   * for a label when the contributing plugin ships no translation —
   * far better than showing the raw type key.
   */
  title_en?: string;
  subtitle_en?: string;
};

/** The paper an export is laid out on. Mirrors the backend's `pageSize.Key`. */
export type PageSizeKey = 'a4' | 'legal';

/** One paper the operator may pick, as the catalog advertises it. */
export type PageSizeOption = {
  key: PageSizeKey;
  label: string;
  /** Points. 72 pt to the inch. */
  width: number;
  height: number;
};

export type Settings = {
  organisation: string;
  classification: string;
  default_days: number;
  retain_reports: number;
  ai_narrative: boolean;
  /**
   * Lets the `ad-hoc` report type send estate detail to the configured AI
   * provider so a model can author the whole document. Deliberately
   * separate from `ai_narrative` and off by default.
   */
  ai_reports: boolean;
  /** Paper for every PDF and Word export. Defaults to A4. */
  page_size: PageSizeKey;
  /**
   * The operator's own marks, base64 PNG with no `data:` prefix. Empty
   * means the built-in `logo-wordmark.svg` / `logo-mark.svg`. An uploaded
   * SVG is rasterised in the browser before it gets here, because the PDF
   * and Word writers embed raster images only.
   */
  logo_png: string;
  icon_png: string;
};

export type CatalogResponse = {
  types: ReportType[];
  levels: Level[];
  page_sizes: PageSizeOption[];
  settings: Settings;
};

/**
 * One AI connection the studio may offer in a "which AI should write
 * this report" selector. Mirrors the backend's `aiConnectionResponse`
 * field-for-field. Deliberately carries no credential, base URL or
 * anything else beyond what a human needs to pick one.
 */
export type AIConnection = {
  id: string;
  name: string;
  provider_type: string;
  is_default: boolean;
};

export type AIConnectionsResponse = {
  /**
   * Whether AI works on this host at all. Can be false even with
   * connections listed, if no provider plugin has registered an adapter
   * for any of them — so this is checked independently of an empty list.
   */
  available: boolean;
  connections: AIConnection[];
};

export type ListResponse = {
  items: StoredReport[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
};

export type GenerateRequest = {
  type: string;
  environment_id?: string;
  environment?: string;
  levels?: Level[];
  from?: string;
  to?: string;
  days?: number;
  options?: string[];
  subject_kind?: SubjectKind | '';
  subject_id?: string;
  subject_name?: string;
  /** The user's question. Sent only for the `ad-hoc` type. */
  prompt?: string;
  /**
   * The configured AI connection to generate with. Sent only when this
   * generation will actually call AI (the `ad-hoc` type, or any type
   * with `include_narrative` checked). Empty/omitted means "use the
   * host's favourite (default) connection".
   */
  ai_connection_id?: string;
};

/** Where a generation has got to. Mirrors `JobState` / `JobStage`. */
export type JobState = 'queued' | 'running' | 'done' | 'failed';

export type JobStage = 'queued' | 'collect' | 'analyze' | 'build' | 'store' | 'done';

export type GenerateJob = {
  id: string;
  state: JobState;
  stage: JobStage;
  percent: number;
  sources_done: number;
  sources_total: number;
  report_type: string;
  report_id?: string;
  code?: string;
  started_at: string;
  ended_at?: string;
};
