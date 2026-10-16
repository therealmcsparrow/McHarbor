// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export function list(value: unknown): string {
  if (Array.isArray(value)) return value.join(', ');
  if (value === null || value === undefined || value === '') return '-';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value) ?? '-';
}

/**
 * A route *inside* the plugin, relative to its root.
 *
 * The core module returned absolute `/kubernetes/...` paths. Everything here
 * is plugin-relative instead, because the absolute form is now
 * `/plugins/addon-kubernetes/...` and only `pluginHref` in `nav-context` should know
 * that prefix — the router these paths are handed to is the host's, reached
 * through the nav context, not a react-router instance inside this bundle.
 */
export function resourcePath(path = ''): string {
  return path.replace(/^\/+/, '');
}
