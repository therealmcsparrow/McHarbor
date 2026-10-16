// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// no polling: forwarders are configuration, not live data. Every list here is
// invalidated by the mutations below, which is what actually changes it.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * One config field, as declared by the adapter's Go ConfigSchema and
 * rendered generically by the Logging tab.
 *
 * `value` is always empty for `secret` fields — the backend never sends
 * a stored secret back. `hasValue` is how the form knows to say "leave
 * blank to keep the stored value" instead of showing what looks like an
 * unconfigured field.
 */
export type LogForwarderField = {
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
 * One installed adapter type — a tile in the Add picker and the schema
 * its form renders. An adapter backs any number of forwarders.
 */
export type LogForwarderAdapter = {
  forwarderType: string;
  slug: string;
  displayName: string;
  icon?: string;
  description?: string;
  supportsDeliveryHistory: boolean;
  encryptedCredentials: boolean;
  fields: LogForwarderField[];
};

/**
 * One configured forwarder: a named destination of some adapter type.
 * Several may share a `forwarderType` — two SIEM collectors, a local
 * and an off-site syslog — each with its own config and enable state.
 */
export type LogForwarder = {
  id: string;
  name: string;
  forwarderType: string;
  slug?: string;
  displayName: string;
  icon?: string;
  description?: string;
  /**
   * False when the plugin providing this type was uninstalled with the
   * forwarder still configured. The row stays in the list, greyed and
   * deletable, rather than vanishing.
   */
  available: boolean;
  enabled: boolean;
  enabledSince?: string;
  supportsDeliveryHistory: boolean;
  encryptedCredentials: boolean;
  fields: LogForwarderField[];
};

/** One recorded delivery attempt. */
export type LogForwarderDelivery = {
  id: string;
  eventId: string;
  status: 'success' | 'failed';
  attempt: number;
  statusCode: number;
  protocol: string;
  error: string;
  createdAt: string;
};

export type SaveLogForwarderInput = {
  name: string;
  enabled: boolean;
  config: Record<string, string>;
};

export type CreateLogForwarderInput = SaveLogForwarderInput & {
  forwarderType: string;
};

const forwardersKey = ['audit-log-forwarders'] as const;
const adaptersKey = ['audit-log-forwarder-adapters'] as const;

/**
 * Installed adapter types. Empty when no `logging-` plugin is
 * installed, which is the tab's normal empty state rather than an
 * error.
 */
export function useLogForwarderAdapters() {
  return useQuery({
    queryKey: adaptersKey,
    queryFn: () => api.get<LogForwarderAdapter[]>('/audit/logging/adapters').then(assertSuccess),
  });
}

/** Configured forwarders, oldest first. */
export function useLogForwarders() {
  return useQuery({
    queryKey: forwardersKey,
    queryFn: () => api.get<LogForwarder[]>('/audit/logging').then(assertSuccess),
  });
}

export function useLogForwarderHistory(id: string, enabled: boolean) {
  return useQuery({
    queryKey: ['audit-log-forwarder-history', id],
    queryFn: () => api.get<LogForwarderDelivery[]>(`/audit/logging/${id}/history`).then(assertSuccess),
    enabled,
  });
}

export function useCreateLogForwarder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateLogForwarderInput) => api.post<{ id: string }>('/audit/logging', body).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: forwardersKey });
    },
  });
}

export function useUpdateLogForwarder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: SaveLogForwarderInput & { id: string }) =>
      api.put(`/audit/logging/${id}`, body).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: forwardersKey });
    },
  });
}

/**
 * Remove one forwarder. The plugin stays installed and its delivery
 * history is kept — only this destination goes.
 */
export function useDeleteLogForwarder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/audit/logging/${id}`).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: forwardersKey });
    },
  });
}

export function useTestLogForwarder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post(`/audit/logging/${id}/test`, {}).then(assertSuccess),
    onSuccess: (_data, id) => {
      // A test writes a delivery-history row, so the history panel is
      // stale the moment the test returns.
      void queryClient.invalidateQueries({ queryKey: ['audit-log-forwarder-history', id] });
    },
  });
}
