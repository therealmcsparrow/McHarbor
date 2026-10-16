// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ThemeVariant } from '../../src/themes';
import type { PluginCapability, PluginPickerSourceRef } from './pickerTypes';

export type { PluginCapability } from './pickerTypes';

export type PluginRoute = {
  plugin_slug: string;
  method: string;
  path: string;
  requires_permission?: string;
  public?: boolean;
};

export type PluginMenuItem = {
  plugin_slug: string;
  parent: string;
  label_ns: string;
  label_key: string;
  path: string;
  icon?: string;
  required_permission?: string;
};

/**
 * The descriptor's free-form settings, as the plugin's `RegisterWidgets`
 * declared them. Every field is optional; the bridge has a fallback for each.
 */
export type PluginWidgetSettings = {
  /** i18n key (`ns:path`) or plain text for the picker description. */
  description?: string;
  /** Tabler component name, resolved through `resolvePluginIcon`. */
  icon?: string;
  min_width?: number;
  min_height?: number;
};

export type PluginWidget = {
  plugin_slug: string;
  key: string;
  /** i18n key (`ns:path`) or plain text for the picker label. */
  title: string;
  category?: string;
  /** `WxH` grid spec, e.g. `6x3`. */
  default_size?: string;
  settings?: PluginWidgetSettings;
};

export type PluginTheme = {
  plugin_slug: string;
  id: string;
  name: string;
  description: string;
  css_path: string;
  swatch_hue: number;
  default_variant: ThemeVariant;
  has_dark: boolean;
  has_light: boolean;
};

export type PluginNode = {
  plugin_slug: string;
  key: string;
  category: string;
  display_name: string;
  icon?: string;
  /**
   * Per-node configSchema descriptors. The loader passes through
   * whatever the plugin declared; the workflow canvas's
   * NodeConfigPanel renders each entry the same way it renders
   * built-in nodes (key/label/type/required/secret/options/default).
   */
  config_schema?: ConfigField[];
  /** Port names for the node's input handles. */
  input_ports?: string[];
  /** Port names for the node's output handles. */
  output_ports?: string[];
  /** Permission gate the host applies before exposing the node in the palette. */
  requires_permission?: string;
};

// Lightweight subset of the workflow canvas's ConfigField. Re-declared
// here (instead of importing from @modules/workflows/types) so the
// loader stays a leaf module — workflows/types imports context-mode
// plugins that don't apply, and the contract is small enough to mirror.
// The runtime treats unknown extra keys as opaque.
export type ConfigFieldType =
  | 'text'
  | 'textarea'
  | 'number'
  | 'select'
  | 'toggle'
  | 'json'
  | 'key-value'
  | 'expression'
  | 'cron'
  | 'container-select'
  | 'environment-select'
  | 'metric-conditions'
  | 'code'
  | 'link-output-select'
  | 'email-server-select'
  | 'communication-channel-select'
  | 'webhook-select'
  | 'registry-select'
  | `storage-location-select`
  | 'stack-select'
  | 'image-select'
  | 'volume-select'
  | 'network-select'
  | 'backup-plan-select'
  | 'backup-run-select'
  | 'multiselect'
  | 'timezone-select'
  | 'container-mount-multiselect'
  | 'backup-restore-item-multiselect'
  | 'stack-service-select'
  | 'plugin-select'
  | 'image-scanner-select';

export type ConfigFieldOption = { value: string; label: string };

export type ConfigField = {
  key: string;
  label: string;
  type: ConfigFieldType;
  required: boolean;
  secret?: boolean;
  options?: ConfigFieldOption[];
  default?: unknown;
  channelType?: string;
  allowCustom?: boolean;
  showWhen?: Record<string, string | string[]>;
  source?: PluginPickerSourceRef;
};

export type PluginStatus = {
  slug: string;
  version: string;
  enabled: boolean;
  loaded: boolean;
  error?: string;
  // Unix-nanosecond mtime of the plugin's UMD bundle on disk.
  // Appended to bundle fetch URLs as a cache buster so a tarball
  // reinstall (same version, fresh file) invalidates the browser
  // cache automatically — the version-pinned buster served stale
  // bundles after a fix until the user hard-reloaded.
  bundle_mtime?: number;
};

export type PluginI18n = {
  lang: string;
  namespaces: Record<string, Record<string, string>>;
};

export type PluginLoaderStatus = 'idle' | 'loading' | 'ready' | 'error';

export type PluginLoaderState = {
  status: PluginLoaderStatus;
  routes: PluginRoute[];
  menu: PluginMenuItem[];
  widgets: PluginWidget[];
  themes: PluginTheme[];
  nodes: PluginNode[];
  tabs: PluginTabItem[];
  contextMenus: PluginContextMenuItem[];
  topbar: PluginTopbarItem[];
  forms: PluginFormItem[];
  capabilities: PluginCapability[];
  installed: PluginStatus[];
  i18n: PluginI18n | null;
  init: (opts?: { force?: boolean }) => Promise<void>;
  refresh: (opts?: { force?: boolean }) => Promise<void>;
  refreshI18n: () => Promise<void>;
};

export type PluginTabItem = {
  plugin_slug: string;
  page_id: string;
  key: string;
  label_ns: string;
  label_key: string;
  icon?: string;
  sort_order?: number;
  required_permission?: string;
};
export type PluginContextMenuItem = {
  plugin_slug: string;
  page_id: string;
  key: string;
  label_ns: string;
  label_key: string;
  icon?: string;
  sort_order?: number;
  required_permission?: string;
};
export type PluginTopbarItem = {
  plugin_slug: string;
  slot: 'left' | 'right';
  label_ns: string;
  label_key: string;
  icon?: string;
  path?: string;
  visibility?: string;
};

// Plugin-owned form extension. The host looks these up by
// (resource, kind) when rendering an edit dialog; on match it
// lazy-loads the plugin's UMD bundle and mounts its exported
// `mount(el, ctx)` into the dialog.
export type PluginFormItem = {
  plugin_slug: string;
  resource: 'channel' | 'email_server' | 'identity_provider' | 'storage_location' | 'ai_provider';
  kind: 'config_form' | 'setup_guide' | 'selector';
  key?: string;
};

// Mounts a plugin-supplied component into the host's edit dialog.
// Plugins publish these on window.McharborPlugin_<slug_underscored>.
// mount(el, ctx) returns a cleanup fn the host calls on unmount.
export type PluginFormMount = (el: HTMLElement, ctx: PluginFormContext) => () => void;

export type PluginFormContext = {
  /** Current resource value map (from the host edit dialog). */
  value: Record<string, string>;
  /** Live setter — call to update the host's form state. */
  onChange: (next: Record<string, string>) => void;
  /** Validation hook the plugin can call to flag a field. */
  onError: (field: string, message: string | null) => void;
  /** Host-provided translation. t('key') returns the active-locale string. */
  t: (key: string, vars?: Record<string, unknown>) => string;
  /** Locked fields the plugin cannot edit (e.g. id, created_at). */
  readOnlyFields?: string[];
};
