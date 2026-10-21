// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState, type ReactElement } from 'react';
import i18n from 'i18next';
import { usePluginForms, usePluginInstalled, type PluginFormItem } from './loader';
import { readPluginGlobal } from './pluginComponent';

/**
 * PluginFormOutlet renders the plugin-contributed UI for a single
 * (resource, kind) slot. Hosts embed one outlet per slot they want to
 * expose — e.g. the channel-edit dialog renders
 *
 *   <PluginFormOutlet resource="channel" kind="config_form" ... />
 *   <PluginFormOutlet resource="channel" kind="setup_guide" ... />
 *
 * The outlet looks up the matching plugin from /plugins/loader/forms,
 * lazy-loads the plugin's UMD bundle via <script>, and calls the
 * bundle's exported `mount(el, ctx)`. The returned cleanup fn runs
 * on unmount and when the resolved plugin changes.
 *
 * Props are passed straight through as the plugin's read-only
 * context. The host's edit dialog owns the value state and updates
 * it via `onChange`; the plugin renders fields, validation, and
 * selectors but does not own the value map.
 */

export type PluginFormOutletProps = {
  resource: PluginFormItem['resource'];
  kind: PluginFormItem['kind'];
  keyName?: string;
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  /** Override the auto-derived plugin match by slug. */
  pluginSlug?: string;
  /** i18n key prefix passed to the plugin as t(). */
  i18nPrefix?: string;
  /** Fields the plugin cannot edit. */
  readOnlyFields?: string[];
  /**
   * Whether the host is creating a new record or editing an existing one.
   * On edit the host strips secret values from `value`, so a plugin form
   * should treat its secret fields as optional (blank keeps the stored value).
   */
  mode?: 'create' | 'edit';
  /** Fallback when no plugin is installed for this resource. */
  children?: ReactElement | null;
};

export function PluginFormOutlet({
  resource,
  kind,
  keyName,
  value,
  onChange,
  onError,
  pluginSlug,
  i18nPrefix,
  readOnlyFields,
  mode,
  children,
}: PluginFormOutletProps): ReactElement | null {
  const forms = usePluginForms();
  const installed = usePluginInstalled();
  const containerRef = useRef<HTMLDivElement | null>(null);
  // The bundle's mount() may return either:
  //   - a bare cleanup function (legacy API), or
  //   - { cleanup, update } where update(patch) lets the host push
  //     new value/onChange snapshots without remounting.
  // We normalize to a single handle so the update-effect below can
  // push value changes without unmounting the bundle (which would
  // lose input focus on every keystroke).
  type BundleHandle = {
    cleanup: () => void;
    update?: (patch: { value?: Record<string, string> }) => void;
  };
  const handleRef = useRef<BundleHandle | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Pick the highest-priority form for this slot. Plugins may
  // register multiple entries; we use the first match. Hosts can
  // pass `pluginSlug` to force a specific plugin.
  //
  // The keyName check is strict: when the host passes a keyName,
  // an entry must (a) declare a key and (b) match it. An entry
  // without a key is treated as "this slot is conditional on no
  // host key" and does NOT match any keyed host request. The
  // previous `f.key && f.key !== keyName` short-circuited to false
  // on the unkeyed case (because `f.key` is undefined), so every
  // unkeyed entry matched every host keyName — first one wins —
  // and the wrong plugin's form appeared in the channel picker.
  const match =
    forms.find((f) => {
      if (f.resource !== resource) return false;
      if (f.kind !== kind) return false;
      if (pluginSlug && f.plugin_slug !== pluginSlug) return false;
      if (keyName && (!f.key || f.key !== keyName)) return false;
      return true;
    }) ?? null;

  // Resolve the installed plugin's UMD bundle mtime so the bundle
  // URL changes whenever the on-disk file is replaced (tarball
  // reinstall, version upgrade). The backend's bundle endpoint
  // ignores unknown query params; `v` is purely a cache buster so
  // a stale <script> tag from a previous install does not hold the
  // old bundle in the browser cache. The mtime key makes a same-
  // version reinstall (same `version`, fresh file) also invalidate
  // the cache — the old version-pinned buster served stale bundles
  // until the user hard-reloaded.
  const installedEntry = installed.find((i) => i.slug === match?.plugin_slug);
  const cacheKey = String(installedEntry?.bundle_mtime ?? installedEntry?.version ?? '');

  useEffect(() => {
    if (!match || !containerRef.current) return;
    let cancelled = false;
    const el = containerRef.current;
    // Narrowed once here rather than asserted at each use: the guard above
    // does not follow `match` into the async closure below.
    const active = match;

    async function loadAndMount() {
      try {
        // Lazy-load the UMD bundle. The loader caches bundles by
        // mtime so a reinstall forces a fresh fetch.
        await loadPluginUmd(active.plugin_slug, cacheKey);
        if (cancelled) return;
        const bundle = readPluginGlobal(active.plugin_slug) as
          | {
              mount?: (el: HTMLElement, ctx: unknown) => (() => void) | BundleHandle;
            }
          | undefined;
        if (!bundle || typeof bundle.mount !== 'function') {
          // The bundle script either didn't ship a mount function
          // or the global was wiped (e.g. the plugin was uninstalled
          // after we first loaded it). Drop the cache entry so the
          // next attempt re-fetches the script instead of trusting
          // a stale "already loaded" flag.
          bundleCache.delete(`${active.plugin_slug}@${cacheKey}`);
          setError(`plugin ${active.plugin_slug} did not export a mount() function`);
          return;
        }
        const ctx = {
          value,
          onChange,
          onError: onError ?? (() => {}),
          readOnlyFields,
          kind,
          mode: mode ?? 'create',
          t: makeT(active.plugin_slug, i18nPrefix ?? 'plugin'),
        };
        const result = bundle.mount(el, ctx);
        // Normalize the legacy and new return shapes into a single
        // handle. Plugins that only return a cleanup function stay
        // working — the value-update effect below becomes a no-op
        // for them, and the bundle's controlled inputs will revert
        // to their initial values on every host re-render (which is
        // the long-standing behavior those plugins ship against).
        if (typeof result === 'function') {
          handleRef.current = { cleanup: result };
        } else if (result && typeof result.cleanup === 'function') {
          handleRef.current = {
            cleanup: result.cleanup,
            update: result.update,
          };
        } else {
          setError(`plugin ${active.plugin_slug} returned an invalid mount handle`);
          return;
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err));
        }
      }
    }

    void loadAndMount();
    return () => {
      cancelled = true;
      const h = handleRef.current;
      handleRef.current = null;
      if (h) {
        try {
          h.cleanup();
        } catch {
          /* a bundle's cleanup may throw once its DOM is gone, and the outlet is unmounting regardless */
        }
      }
    };
    // oxlint-disable-next-line react/exhaustive-deps -- mount once per plugin identity; value, onChange and the other props are read at mount and later value changes are pushed through handle.update
  }, [match?.plugin_slug, match?.kind, match?.resource, keyName, i18nPrefix, cacheKey, mode]);

  // Push `value` updates into the bundle whenever the host's value
  // changes. The mount effect above intentionally does not depend
  // on `value` so it only fires on plugin/identity changes — without
  // this separate effect, controlled inputs inside the bundle would
  // keep reading from the stale closure captured at mount time and
  // reset to their initial values on every keystroke.
  useEffect(() => {
    const h = handleRef.current;
    if (h?.update) {
      try {
        h.update({ value });
      } catch {
        /* swallow — bundle may have unmounted */
      }
    }
  }, [value]);

  if (!match) return children ?? null;
  return (
    <div
      ref={containerRef}
      data-plugin-form-slot={resource}
      data-plugin-form-kind={kind}
      data-plugin-form-slug={match.plugin_slug}
      className="plugin-form-outlet"
    >
      {error && (
        <div
          role="alert"
          className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive"
        >
          {error}
        </div>
      )}
    </div>
  );
}

const bundleCache = new Set<string>();

function loadPluginUmd(slug: string, version: string): Promise<void> {
  const key = version ? `${slug}@${version}` : slug;
  // The bundleCache only remembers "we issued a script tag for this
  // slug@version". It does NOT prove the bundle is actually loaded —
  // plugin uninstall+reinstall within the same SPA session drops the
  // window.<key> global but leaves the cache entry behind, so a
  // subsequent mount call sees `bundle.mount === undefined` even
  // though loadPluginUmd resolved immediately. Verify the global
  // exists before trusting the cache; otherwise drop the entry and
  // re-fetch.
  if (bundleCache.has(key)) {
    if (typeof readPluginGlobal(slug) !== 'undefined') {
      return Promise.resolve();
    }
    bundleCache.delete(key);
  }
  return new Promise((resolve, reject) => {
    const params = new URLSearchParams({ slug });
    if (version) params.set('v', version);
    const url = `/api/plugins/loader/bundle?${params.toString()}`;
    const script = document.createElement('script');
    script.src = url;
    script.async = true;
    script.onload = () => {
      bundleCache.add(key);
      resolve();
    };
    script.onerror = () => reject(new Error(`failed to load plugin UMD bundle for ${slug}`));
    document.head.appendChild(script);
  });
}

// A value no translation will ever legitimately equal, used as the
// `defaultValue` so a miss is detectable. Comparing the result to the
// key instead — which this shim used to do — reports a false miss
// whenever a translation happens to equal its own key.
const I18N_MISS = '__mcharbor_plugin_i18n_miss__';

/**
 * Minimal i18n shim handed to plugins. Resolves keys through the
 * host's i18next instance; falls back to the raw key so a missing
 * translation never crashes the form.
 *
 * The plugin's ConfigForm takes `externalT ?? localT`, so whatever this
 * returns is what renders — a shim that silently misses shows the operator
 * a form labelled `fields.tenant_id`. It therefore asks for the key the
 * way the backend actually publishes it, and only then falls back.
 *
 * `LoadI18n` (modules/plugin_runtime/service.go) registers each plugin's
 * bundle under a namespace named by the **plugin slug**, flattened to
 * dotted keys — `identity-entra-id` / `fields.tenant_id`. The slug lookup
 * is therefore first, and the two `<prefix>.<key>` forms trail it: one
 * inside the plugin's namespace for a plugin that nests under the host's
 * prefix, one in the default namespace, which is the contract this shim
 * originally declared and is kept so any caller that did satisfy it keeps
 * working.
 */
function makeT(slug: string, prefix: string): (key: string, vars?: Record<string, unknown>) => string {
  return (key, vars) => {
    const attempts: { key: string; ns?: string }[] = [
      { key, ns: slug },
      { key: `${prefix}.${key}`, ns: slug },
      { key: `${prefix}.${key}` },
    ];
    for (const attempt of attempts) {
      // Spread `vars` first: the lookup options are ours to control, and
      // a plugin passing an interpolation variable called `ns` must not
      // redirect the lookup.
      const out = i18n.t(attempt.key, {
        ...vars,
        ...(attempt.ns ? { ns: attempt.ns } : {}),
        defaultValue: I18N_MISS,
      });
      if (typeof out === 'string' && out !== I18N_MISS) return out;
    }
    // Last-resort interpolation for {{var}} placeholders so the
    // plugin UI still shows something readable while translations
    // land.
    if (!vars) return key;
    return key.replace(/\{\{(\w+)\}\}/g, (_, k: string) => {
      const value = vars[k];
      return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean' ? String(value) : '';
    });
  };
}
