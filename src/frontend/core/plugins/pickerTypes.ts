// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** Where a `plugin-select` field lists from: a picker-source the declaring plugin owns. */
export type PluginPickerSourceRef = { plugin: string; target: string };

/**
 * A picker-source's list, resolved server-side from the plugin's runbook-provider
 * target: `permission` is the key the list route itself enforces.
 */
export type PluginPickerList = {
  path: string;
  query?: Record<string, string>;
  items?: string;
  id?: string;
  idJoin?: string[];
  name: string[];
  detail?: string[];
  labelKey: string;
  i18nNs: string;
  permission: string;
};

/**
 * One fact a loaded plugin declares under `contributes.capabilities`, validated
 * server-side by `core/plugincaps`: only the fields its kind uses are present.
 */
export type PluginCapability = {
  plugin: string;
  kind: string;
  domain?: string;
  sourceKind?: string;
  subjectType?: string;
  metric?: string;
  providerType?: string;
  path?: string;
  page?: string;
  environmentField?: string;
  formats?: string[];
  permission?: string;
  generatePath?: string;
  exportPath?: string;
  pages?: Record<string, string>;
  routes?: Record<string, string>;
  permissions?: Record<string, string>;
  target?: string;
  alias?: string;
  node?: string;
  channelType?: string;
  picker?: PluginPickerList;
  sources?: PluginSearchSource[];
  actions?: PluginSearchAction[];
  /** A backup-row-actions or estate-source label: `labelKey` in the plugin's namespace `labelNs`. */
  labelKey?: string;
  labelNs?: string;
  /** Estate-source card presentation, present only together with `labelKey`. */
  icon?: string;
  envScoped?: boolean;
  runningField?: string;
  runningValues?: string[];
  link?: string;
};

/**
 * One list a `search-source` declares for the Ctrl+K palette. `permission` and `i18nNs`
 * are resolved server-side from the route's own gate and the capability.
 */
export type PluginSearchSource = {
  key: string;
  labelKey: string;
  path: string;
  envScoped: boolean;
  items?: string;
  id: string;
  name: string[];
  detail?: string[];
  link: string;
  icon: string;
  permission: string;
  i18nNs: string;
  /** Repeat the request once per item of another list the plugin owns (one per cluster). */
  fanOut?: PluginSearchFanOut;
};

/**
 * A source's fan-out list. Each item's `id` is sent as `?<param>=` and becomes the
 * `<param>` field of every row it returns, so a link can carry `?cluster={cluster}`.
 */
export type PluginSearchFanOut = {
  path: string;
  items?: string;
  id: string;
  param: string;
  label?: string;
  permission: string;
};

/** One navigation-only palette action a `search-action` declares. */
export type PluginSearchAction = {
  key: string;
  labelKey: string;
  keywordsKey?: string;
  icon: string;
  link: string;
  permission: string;
  i18nNs: string;
};
