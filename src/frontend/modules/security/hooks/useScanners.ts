// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * One config field, as declared by the adapter's Go ConfigSchema and
 * rendered generically by the Scanners tab.
 *
 * `value` is always empty for `secret` fields — the backend never sends
 * a stored secret back. `hasValue` is how the form knows to say "leave
 * blank to keep the stored value" instead of showing what looks like an
 * unconfigured field.
 */
export type ScannerField = {
  key: string;
  type: 'string' | 'int' | 'bool' | 'secret';
  label: string;
  required?: boolean;
  help?: string;
  placeholder?: string;
  default?: string | number | boolean;
  value: string;
  hasValue: boolean;
};

/**
 * One installed scanner engine — a tile in the Add picker and the
 * schema its form renders. An adapter backs any number of instances.
 */
export type ScannerAdapter = {
  scannerType: string;
  slug: string;
  displayName: string;
  icon?: string;
  description?: string;
  /** Whether the engine could run right now with its declared defaults. */
  available: boolean;
  /**
   * True for engines that shell out to a CLI (Trivy, Grype). It is what
   * lets the tab explain an unavailable row correctly: a missing binary
   * and an unreachable server are different problems with different
   * fixes.
   */
  requiresLocalBinary: boolean;
  supportsTest: boolean;
  encryptedCredentials: boolean;
  fields: ScannerField[];
};

/**
 * One configured scanner: a named engine with its own settings.
 * Several may share a `scannerType` — a strict Trivy and a lenient one,
 * a production Clair and a staging one — each with its own config and
 * enable state.
 */
export type ScannerConfig = {
  id: string;
  name: string;
  scannerType: string;
  slug?: string;
  displayName: string;
  icon?: string;
  description?: string;
  /**
   * False when the plugin providing this engine was uninstalled with
   * the scanner still configured. The row stays in the list, greyed and
   * deletable, rather than vanishing.
   */
  installed: boolean;
  /**
   * False when the engine is installed but cannot run with this row's
   * config — the CLI went missing, or the server URL is blank.
   */
  available: boolean;
  enabled: boolean;
  isDefault: boolean;
  requiresLocalBinary: boolean;
  supportsTest: boolean;
  encryptedCredentials: boolean;
  fields: ScannerField[];
};

export type SaveScannerInput = {
  name: string;
  enabled: boolean;
  makeDefault: boolean;
  config: Record<string, string>;
};

export type CreateScannerInput = SaveScannerInput & {
  scannerType: string;
};

const configsKey = ['scannerConfigs'] as const;
const adaptersKey = ['scannerAdapters'] as const;

/**
 * Installed scanner engines. Empty when no `scanner-` plugin is
 * installed, which is the tab's normal empty state rather than an
 * error.
 */
export function useScannerAdapters() {
  return useQuery({
    queryKey: adaptersKey,
    queryFn: () => api.get<ScannerAdapter[]>('/scans/adapters').then(assertSuccess),
  });
}

/** Configured scanners, oldest first. */
export function useScannerConfigs() {
  return useQuery({
    queryKey: configsKey,
    queryFn: () => api.get<ScannerConfig[]>('/scans/configs').then(assertSuccess),
  });
}

/**
 * Invalidate everything a scanner write can change.
 *
 * `['scans', 'scanners']` is the picker the container detail page and
 * the App Store install flow read. Adding a scanner and not seeing it
 * offered until a reload is the kind of gap that makes an operator
 * think the save failed, so it is refreshed alongside the tab's own
 * lists.
 */
function useInvalidateScanners() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: configsKey });
    void queryClient.invalidateQueries({ queryKey: ['scans', 'scanners'] });
  };
}

export function useCreateScanner() {
  const invalidate = useInvalidateScanners();
  return useMutation({
    mutationFn: (body: CreateScannerInput) => api.post<{ id: string }>('/scans/configs', body).then(assertSuccess),
    onSuccess: invalidate,
  });
}

export function useUpdateScanner() {
  const invalidate = useInvalidateScanners();
  return useMutation({
    mutationFn: ({ id, ...body }: SaveScannerInput & { id: string }) =>
      api.put(`/scans/configs/${id}`, body).then(assertSuccess),
    onSuccess: invalidate,
  });
}

/**
 * Remove one configured scanner. The plugin stays installed, and scans
 * this engine already ran keep their findings — only this configuration
 * goes.
 */
export function useDeleteScanner() {
  const invalidate = useInvalidateScanners();
  return useMutation({
    mutationFn: (id: string) => api.del(`/scans/configs/${id}`).then(assertSuccess),
    onSuccess: invalidate,
  });
}

/** One phase of a scanner test. Same shape the storage and communication tests use. */
export type ScannerTestStep = {
  name: string;
  status: 'pass' | 'warn' | 'fail' | 'skip';
  detail?: string;
  latency?: string;
};

/**
 * The per-step report `POST /scans/configs/{id}/test` returns.
 *
 * A failing test is a **200** with `overallStatus: "fail"`, not an
 * error status — the request succeeded and produced a diagnosis. That
 * is what lets the modal render the failure instead of a bare toast.
 */
export type ScannerTestResult = {
  scannerId: string;
  scannerName: string;
  scannerType: string;
  overallStatus: 'pass' | 'warn' | 'fail' | 'skip';
  startedAt: string;
  completedAt: string;
  duration: string;
  steps: ScannerTestStep[];
};

export function useTestScanner() {
  return useMutation({
    mutationFn: (id: string) => api.post<ScannerTestResult>(`/scans/configs/${id}/test`, {}).then(assertSuccess),
  });
}
