// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Level, ReportDocument, SubjectKind } from './types';

/**
 * Scheduled reports.
 *
 * A schedule is a saved generation request plus a cron and at least one
 * delivery target. It keeps exactly one run — its most recent — and that
 * run never enters the report library.
 */
export type ScheduleTargetKind = 'comm' | 'storage';

export type ScheduleTarget = {
  id: string;
  kind: ScheduleTargetKind;
  ref_id: string;
  /**
   * Storage targets only: the destination folder, relative to the
   * location's own configured base path. Empty means the plugin's
   * default, `mcharbor-reports/<schedule-slug>`.
   */
  path?: string;

  /**
   * Storage targets only: the name pattern the file is written under,
   * before the format's extension. Tokens: {name} {type} {env} {date}
   * {time}. Empty means the exporter's own dated name.
   */
  filename?: string;

  /**
   * Comm targets only: an alternative destination for this schedule,
   * leaving the channel itself untouched. Empty means the channel's own
   * destination, and is the only possibility where the adapter declared
   * no overridable field.
   */
  recipient?: string;
  /** Resolved from the host at read time, so a rename shows through. */
  name?: string;
  /**
   * Comm targets only. False means the host drops the document and the
   * recipient gets the summary text alone — which the UI says out loud
   * rather than promising an attachment that never arrives.
   */
  supports_files: boolean;
  /**
   * Comm targets only, resolved from the host at read time. False with a
   * non-empty `recipient` means a stored override the adapter no longer
   * honours.
   */
  supports_recipient: boolean;
  /** The channel or location this target names no longer exists. */
  missing?: boolean;
};

export type ScheduleRunStatus = 'running' | 'success' | 'partial' | 'failed';

export type DeliveryOutcome = {
  kind: ScheduleTargetKind;
  ref_id: string;
  name: string;
  ok: boolean;
  /** An i18n code, never a raw transport error. */
  code?: string;
  /** False when the message went but the document could not travel with it. */
  attached: boolean;
};

export type ScheduleRun = {
  schedule_id: string;
  status: ScheduleRunStatus;
  title: string;
  code?: string;
  deliveries: DeliveryOutcome[];
  started_at: string;
  finished_at?: string;
  /** Populated by the single-run read only, never by the list. */
  document?: ReportDocument;
};

export type Schedule = {
  id: string;
  name: string;
  report_type: string;
  subject_kind?: SubjectKind;
  subject_id?: string;
  subject_name?: string;
  environment_id: string;
  environment: string;
  period_days: number;
  levels: Level[];
  options: string[];
  format: 'pdf' | 'docx';
  prompt?: string;
  ai_connection_id?: string;
  cron: string;
  enabled: boolean;
  created_by: string;
  last_run_at?: string;
  next_run_at?: string;
  created_at: string;
  updated_at: string;
  targets: ScheduleTarget[];
  last_run?: ScheduleRun;
};

export type ScheduleListResponse = {
  items: Schedule[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
};

/** The create/update body. Mirrors the backend's `scheduleRequest`. */
export type ScheduleRequest = {
  name: string;
  type: string;
  environment_id?: string;
  environment?: string;
  levels?: Level[];
  days?: number;
  options?: string[];
  subject_kind?: SubjectKind | '';
  subject_id?: string;
  subject_name?: string;
  prompt?: string;
  ai_connection_id?: string;
  format: 'pdf' | 'docx';
  cron: string;
  enabled?: boolean;
  targets: {
    kind: ScheduleTargetKind;
    ref_id: string;
    path?: string;
    filename?: string;
    recipient?: string;
  }[];
};

/** One destination a schedule may deliver to. Carries no credential. */
export type DeliveryTarget = {
  kind: ScheduleTargetKind;
  id: string;
  name: string;
  /** Channel type or location type, so a picker can name the provider. */
  type: string;
  supports_files: boolean;
  /**
   * True for a channel whose adapter declared an overridable destination
   * — a mail `to`, a Telegram chat. False for a webhook-bound channel
   * where the URL is both destination and secret, and for every storage
   * location, which is retargeted with a folder instead.
   */
  supports_recipient: boolean;
  /** "email" | "phone" | "chat_id" | "topic", or absent for free text. */
  recipient_kind?: string;
  /** The adapter's own English label, used when this bundle has none. */
  recipient_label?: string;
  recipient_placeholder?: string;
  /** Whether a comma-separated list is accepted. */
  recipient_multiple?: boolean;
};

export type DeliveryTargetsResponse = {
  channels: DeliveryTarget[];
  locations: DeliveryTarget[];
};
