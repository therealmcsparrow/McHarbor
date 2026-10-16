// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { api } from '@core/api/client';
import {
  usePluginLoader,
  type PluginCapability,
  type PluginPickerSourceRef,
  type PluginStatus,
} from '@core/plugins/loader';

/** Must stay in sync with the `Kind` constants in core/plugincaps/capabilities.go. */
export type PluginCapabilityKind =
  | 'runbook-provider'
  | 'backup-row-actions'
  | 'estate-source'
  | 'lifecycle-subject'
  | 'document-export'
  | 'login-endpoint'
  | 'picker-source'
  | 'workflow-node-alias'
  | 'email-channel'
  | 'search-source'
  | 'search-action';

/** The field each kind is looked up by, mirroring `discriminator()` on the backend. */
const KEY_FIELD: Partial<Record<PluginCapabilityKind, keyof PluginCapability>> = {
  'runbook-provider': 'domain',
  'backup-row-actions': 'sourceKind',
  'estate-source': 'metric',
  'lifecycle-subject': 'subjectType',
  'login-endpoint': 'providerType',
  'picker-source': 'target',
  'workflow-node-alias': 'alias',
  'email-channel': 'channelType',
};

/**
 * The backend registry only drops a plugin when it unloads, but a disabled plugin
 * keeps running until restart; the host has always treated it as gone, so this does too.
 */
export function activeCapabilities(
  capabilities: PluginCapability[],
  installed: PluginStatus[],
  kind: PluginCapabilityKind,
): PluginCapability[] {
  const active = new Set(installed.filter((p) => p.enabled && p.loaded).map((p) => p.slug));
  return capabilities.filter((c) => c.kind === kind && active.has(c.plugin));
}

/** Every active capability of `kind`, sorted by plugin like the backend registry. */
export function usePluginCapabilities(kind: PluginCapabilityKind): PluginCapability[] {
  const capabilities = usePluginLoader((s) => s.capabilities);
  const installed = usePluginLoader((s) => s.installed);
  return useMemo(() => activeCapabilities(capabilities, installed, kind), [capabilities, installed, kind]);
}

/** The active capability of `kind` keyed by `key`, or undefined when no plugin provides it. */
export function usePluginCapability(kind: PluginCapabilityKind, key: string): PluginCapability | undefined {
  const all = usePluginCapabilities(kind);
  const field = KEY_FIELD[kind];
  return field ? all.find((c) => c[field] === key) : all[0];
}

/**
 * The picker-source a `plugin-select` field names. Matched on plugin and target
 * together, since two plugins may each declare a target of the same name.
 */
export function usePickerSource(ref: PluginPickerSourceRef | undefined): PluginCapability | undefined {
  const all = usePluginCapabilities('picker-source');
  return ref ? all.find((c) => c.plugin === ref.plugin && c.target === ref.target && c.picker) : undefined;
}

/**
 * The `plugin:<slug>:<node>` action a loaded plugin claims for a bare stored action
 * (a node the host once shipped), or undefined when no active plugin claims it.
 */
export function resolveNodeAliasAction(
  action: string,
  capabilities: PluginCapability[],
  installed: PluginStatus[],
): string | undefined {
  if (action.startsWith('plugin:')) return undefined;
  const claim = activeCapabilities(capabilities, installed, 'workflow-node-alias').find((c) => c.alias === action);
  return claim?.node ? `plugin:${claim.plugin}:${claim.node}` : undefined;
}

/** Must stay in sync with `ChannelAliasNode` in core/plugincaps/channel.go. */
export const CHANNEL_ALIAS_NODE = 'send-communication';

/**
 * The channel type a loaded comm plugin claims for a bare stored action (a per-provider
 * send node the host once shipped); the canvas shows it as the generic send node.
 */
export function resolveChannelAlias(
  action: string,
  capabilities: PluginCapability[],
  installed: PluginStatus[],
): string | undefined {
  if (action.startsWith('plugin:')) return undefined;
  return activeCapabilities(capabilities, installed, 'workflow-node-alias').find((c) => c.alias === action)
    ?.channelType;
}

/** The channel types active plugins declare as email transports. */
export function useEmailChannelTypes(): string[] {
  const claims = usePluginCapabilities('email-channel');
  return useMemo(() => claims.flatMap((c) => (c.channelType ? [c.channelType] : [])), [claims]);
}

/** A manifest route (`/api/things`) as the api client takes it (`/things`). */
export function apiClientPath(manifestPath: string): string {
  return manifestPath.startsWith('/api/') ? manifestPath.slice(4) : manifestPath;
}

/** Fills `{name}` placeholders, each value path-escaped so it can never add a segment. */
export function fillPath(template: string, values: Record<string, string>): string {
  return template.replace(/\{([^}]+)\}/g, (whole, placeholder: string) => {
    const value = values[placeholder];
    return value === undefined ? whole : encodeURIComponent(value);
  });
}

export type LoginEndpoint = { providerType: string; path: string };

/**
 * The login screen runs before a session exists, so it reads the one public slice
 * of the registry instead of the loader store.
 */
export async function fetchLoginEndpoint(providerType: string): Promise<LoginEndpoint | undefined> {
  const res = await api.get<LoginEndpoint[]>('/plugins/loader/login-endpoints');
  return (res.data ?? []).find((e) => e.providerType === providerType);
}
