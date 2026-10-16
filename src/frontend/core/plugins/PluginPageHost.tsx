// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import type { ReactElement } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { usePluginInstalled } from './loader';

declare global {
  interface Window {
    [key: string]: unknown;
  }
}

/**
 * PluginNavContext is what the host hands a plugin bundle on mount and
 * on every subsequent in-plugin navigation.
 *
 * A plugin is mounted once for the whole `/plugins/:slug/*` subtree, so
 * without this it cannot tell which of its own sidebar entries the user
 * clicked — the mount effect does not re-run when only the splat
 * changes. `onNavigate` closes that gap, and `navigate` lets the plugin
 * drive the host router (keeping sidebar active-state correct) instead
 * of reaching for `history.pushState`.
 */
export type PluginNavContext = {
  slug: string;
  /** Full pathname, e.g. `/plugins/addon-reporting/library`. */
  path: string;
  /** Path below `/plugins/<slug>/`, e.g. `library`. Empty at the root. */
  splat: string;
  /** Query string without the leading `?`. */
  search: string;
  /**
   * Navigate the host router. Relative paths resolve under the plugin.
   *
   * `preventScrollReset` is forwarded to react-router unchanged. A plugin that
   * puts its active tab in the query string — as the Swarm add-on's two detail
   * pages do — needs it: without it, switching tabs scrolls the reader back to
   * the top of a page they had not finished reading.
   */
  navigate: (to: string, opts?: { replace?: boolean; preventScrollReset?: boolean }) => void;
};

type PluginMount = {
  mount: (el: HTMLElement, ctx?: PluginNavContext) => void | (() => void);
  unmount?: (el?: HTMLElement) => void;
  /** Optional: called when the location changes under the plugin route. */
  onNavigate?: (ctx: PluginNavContext) => void;
};

const SCRIPT_ID_PREFIX = 'mcharbor-plugin-umd-';

/**
 * PluginPageHost renders the page area for the static
 * `/plugins/:slug/*` wildcard route.
 *
 * Phase 2+: fetch the plugin's UMD bundle from
 * `/api/plugins/loader/bundle?slug=<slug>` (served from the host's
 * data directory at `$DATA_DIR/plugins/<slug>@<version>/frontend/`)
 * and invoke its `mount(el, ctx)` lifecycle. The bundle registers
 * itself on `window.McharborPlugin_<slug_underscored>`.
 */
export function PluginPageHost(): ReactElement {
  const { slug = '' } = useParams<{ slug: string }>();
  const installed = usePluginInstalled();
  const { t } = useTranslation('plugins');
  const loc = useLocation();
  const navigate = useNavigate();
  const hostRef = useRef<HTMLDivElement | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);
  const [bundleError, setBundleError] = useState<{ key: string; message: string } | null>(null);

  const entry = installed.find((p: { slug: string }) => p.slug === slug);
  const globalName = `McharborPlugin_${slug.replace(/-/g, '_')}`;

  const base = `/plugins/${slug}`;
  const splat = loc.pathname.startsWith(`${base}/`) ? loc.pathname.slice(base.length + 1) : '';

  const buildCtx = useCallback(
    (): PluginNavContext => ({
      slug,
      path: loc.pathname,
      splat,
      search: loc.search.replace(/^\?/, ''),
      navigate: (to, opts) =>
        navigate(to.startsWith('/') ? to : `${base}/${to}`.replace(/\/+$/, ''), {
          replace: opts?.replace ?? false,
          preventScrollReset: opts?.preventScrollReset,
        }),
    }),
    [slug, loc.pathname, loc.search, splat, navigate, base],
  );

  // Keep the latest context reachable from the mount effect without
  // making it a dependency — remounting the whole bundle on every
  // in-plugin navigation would throw away its React state.
  const ctxRef = useRef(buildCtx);
  useEffect(() => {
    ctxRef.current = buildCtx;
  }, [buildCtx]);

  const errorKey = `${slug}|${Boolean(entry?.loaded)}`;
  let error: string | null = null;
  if (slug && !entry?.loaded) error = t('errors.not_loaded', { slug });
  else if (bundleError?.key === errorKey) error = bundleError.message;

  useEffect(() => {
    if (!slug) return undefined;
    if (!entry?.loaded) return undefined;

    const hostEl = hostRef.current;
    if (!hostEl) return undefined;
    const setError = (message: string) => setBundleError({ key: `${slug}|true`, message });

    const scriptId = `${SCRIPT_ID_PREFIX}${slug}`;
    let script = document.getElementById(scriptId) as HTMLScriptElement | null;
    let cancelled = false;

    // Both mount paths share one teardown. Returning `undefined` from the
    // already-loaded path — which is every visit after the first — left the
    // plugin's React root alive after the route moved on, and a plugin that
    // renders a `PageHeader` portals into `#header-slot`: the stale header
    // stayed in the topbar and the next visit stacked another one beside it.
    const teardown = () => {
      cancelled = true;
      const cleanup = cleanupRef.current;
      cleanupRef.current = null;
      if (cleanup) cleanup();
    };

    const existing = window[globalName] as PluginMount | undefined;
    if (existing) {
      cleanupRef.current = invokeMount(existing, hostEl, ctxRef.current());
      return teardown;
    }

    const onLoad = () => {
      if (cancelled) return;
      const bundle = window[globalName] as PluginMount | undefined;
      if (!bundle || typeof bundle.mount !== 'function') {
        setError(t('errors.no_mount', { slug }));
        return;
      }
      if (hostRef.current) {
        cleanupRef.current = invokeMount(bundle, hostRef.current, ctxRef.current());
      }
    };
    const onError = () => {
      if (cancelled) return;
      setError(t('errors.bundle_failed', { slug }));
    };

    if (!script) {
      script = document.createElement('script');
      script.id = scriptId;
      script.src = `/api/plugins/loader/bundle?slug=${encodeURIComponent(slug)}`;
      script.async = true;
      script.onload = onLoad;
      script.onerror = onError;
      document.head.appendChild(script);
    } else {
      // Re-attached after a navigation; if the global is already
      // populated we can mount immediately.
      onLoad();
    }

    return teardown;
  }, [entry?.loaded, slug, globalName, t]);

  // Location changes below `/plugins/<slug>` do not remount the bundle;
  // tell it instead, so one plugin can back several sidebar entries.
  useEffect(() => {
    const bundle = window[globalName] as PluginMount | undefined;
    if (typeof bundle?.onNavigate !== 'function') return;
    try {
      bundle.onNavigate(buildCtx());
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(`plugin ${slug} onNavigate threw`, err);
    }
  }, [globalName, buildCtx, slug]);

  if (!entry) {
    return <div className="p-6 text-sm text-muted-foreground">{t('errors.unknown_slug', { slug })}</div>;
  }

  if (error) {
    return <div className="p-6 text-sm text-muted-foreground">{error}</div>;
  }

  return <div ref={hostRef} className="flex h-full min-h-0 flex-1 flex-col overflow-y-auto" data-plugin-slug={slug} />;
}

function invokeMount(bundle: PluginMount, el: HTMLElement, ctx: PluginNavContext): () => void {
  let result: void | (() => void);
  try {
    result = bundle.mount(el, ctx);
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error(`plugin ${String(el.dataset.pluginSlug ?? '?')} mount threw`, err);
    return () => undefined;
  }
  return () => {
    try {
      if (typeof bundle.unmount === 'function') {
        bundle.unmount(el);
      } else if (typeof result === 'function') {
        result();
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('plugin unmount failed', err);
    }
  };
}
