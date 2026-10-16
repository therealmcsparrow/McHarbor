// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type HostLogSource = 'system' | 'kernel' | 'auth' | 'docker';

export type HostLogResult = {
  source: HostLogSource;
  tail: number;
  lines: string[];
  /**
   * Warning codes only. Discovery output — which log files exist, which one
   * was read as a fallback — arrives in availableFiles / fallbackFile, because
   * rendering it as a warning made every successful read look like a failure.
   */
  notices: string[];
  /** Log file names found under the host's /var/log. Empty for agent envs. */
  availableFiles?: string[];
  /** Set when no configured candidate matched and the largest log was read. */
  fallbackFile?: string;
  fetchedAt: string;
};

/**
 * Every warning code the host script (src/backend/modules/system/service.go)
 * and the agent (src/agent/host_log.go) can emit. The set is closed, so the
 * whole `detail.hostLogs.notices.*` key group exists in the bundle.
 */
const KNOWN_NOTICES: readonly string[] = [
  'permission_denied',
  'no_supported_log_source',
  'no_var_log_dir',
  'no_journalctl',
  'journalctl_failed',
];

type Translate = (key: string, options?: Record<string, unknown>) => string;

/**
 * Resolves one notice code to display text. An unrecognised code — a newer
 * agent, say — renders through `notices.unknown` carrying the raw code, so it
 * stays reportable instead of rendering a bare dotted key.
 */
export function hostLogNoticeText(t: Translate, code: string): string {
  if (KNOWN_NOTICES.includes(code)) {
    return t(`detail.hostLogs.notices.${code}`);
  }
  return t('detail.hostLogs.notices.unknown', { code });
}
