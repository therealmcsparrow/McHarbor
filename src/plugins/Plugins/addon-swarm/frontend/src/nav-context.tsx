// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Navigation, without react-router.
//
// This is the one thing that could not be ported unchanged from the core
// module. The host mounts this bundle as a UMD global inside its own React
// tree, but the bundle carries its *own* copy of every library it imports —
// and a second react-router copy has no Router context, so `<Link>` and
// `useNavigate()` do not merely misbehave, they throw and blank the plugin
// root. (The Kubernetes and Reporting add-ons each learned this the same way.)
//
// What the host does provide is a `PluginNavContext` on mount and on every
// navigation below `/plugins/addon-swarm`: where we are, and a `navigate`
// bound to the host's own router. Everything here is a thin shell over that.

import { createContext, useCallback, useContext, useMemo, type AnchorHTMLAttributes, type ReactNode } from 'react';
import type { PluginNavContext } from './index';

const NavContext = createContext<PluginNavContext | null>(null);

export function PluginNavProvider({ value, children }: { value: PluginNavContext; children: ReactNode }) {
  return <NavContext.Provider value={value}>{children}</NavContext.Provider>;
}

export function usePluginNav(): PluginNavContext {
  const ctx = useContext(NavContext);
  if (!ctx) throw new Error('usePluginNav must be used inside PluginNavProvider');
  return ctx;
}

/**
 * The plugin's route segment below `/plugins/addon-swarm`, e.g. `services` or
 * `services/abc123`. Empty on the overview.
 */
export function usePluginSplat(): string {
  return usePluginNav().splat;
}

/** Absolute path for a plugin-relative route. */
export function pluginHref(to: string): string {
  if (to.startsWith('/')) return to;
  const trimmed = to.replace(/^\/+/, '');
  return trimmed ? `/plugins/addon-swarm/${trimmed}` : '/plugins/addon-swarm';
}

type PluginLinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> & {
  /** A path relative to the plugin root — `''`, `'services'`, `'nodes/abc'`. */
  to: string;
  children: ReactNode;
};

/**
 * A real `<a>` that navigates through the host router.
 *
 * The `href` is genuine, so middle-click, ctrl-click and "copy link address"
 * all behave the way a reader expects; the click handler intercepts the plain
 * left-click and hands it to the host instead of letting the browser do a full
 * page load. That is exactly what react-router's `Link` does — this just does
 * it against the host's router rather than a second, contextless one.
 */
export function PluginLink({ to, children, onClick, ...rest }: PluginLinkProps) {
  const { navigate } = usePluginNav();
  const href = pluginHref(to);

  const handleClick = useCallback(
    (evt: React.MouseEvent<HTMLAnchorElement>) => {
      onClick?.(evt);
      if (evt.defaultPrevented) return;
      // Leave the modified clicks to the browser: a new tab is a new document
      // and the host router has no say in it.
      if (evt.button !== 0 || evt.metaKey || evt.ctrlKey || evt.shiftKey || evt.altKey) {
        return;
      }
      evt.preventDefault();
      navigate(href);
    },
    [href, navigate, onClick],
  );

  return (
    <a href={href} onClick={handleClick} {...rest}>
      {children}
    </a>
  );
}

/**
 * `navigate`, bound to plugin-relative paths.
 *
 * The drop-in for `useNavigate()` in the six ported pages. It takes the same
 * `(to, { replace })` shape, so the call sites read unchanged; what differs is
 * that `to` is relative to the plugin root rather than to the site root.
 */
type NavOptions = { replace?: boolean; preventScrollReset?: boolean };

export function usePluginNavigate(): (to: string, opts?: NavOptions) => void {
  const { navigate } = usePluginNav();
  return useMemo(() => (to: string, opts?: NavOptions) => navigate(pluginHref(to), opts), [navigate]);
}

type SearchParamsInit = URLSearchParams | ((current: URLSearchParams) => URLSearchParams);

/**
 * The query string, in react-router's `[params, setParams]` shape.
 *
 * The three pages that read a `?tab=` or `?stack=` out of the URL kept their
 * call sites because of this. The setter accepts a value or an updater, like
 * react-router's, and writes by navigating the host router — so the tab a user
 * lands on survives a reload and a copied link, which is the only reason these
 * pages put the tab in the URL in the first place.
 *
 * `replace` is the default. A tab switch inside a detail page is not a
 * destination; pushing it would make the browser Back button walk through
 * every tab the user looked at before it left the page.
 */
export function usePluginSearchParams(): [URLSearchParams, (next: SearchParamsInit, opts?: NavOptions) => void] {
  const { path, search, navigate } = usePluginNav();
  const params = useMemo(() => new URLSearchParams(search), [search]);

  const setParams = useCallback(
    (next: SearchParamsInit, opts?: NavOptions) => {
      const resolved = typeof next === 'function' ? next(new URLSearchParams(search)) : next;
      const query = resolved.toString();
      navigate(query ? `${path}?${query}` : path, {
        replace: opts?.replace ?? true,
        preventScrollReset: opts?.preventScrollReset,
      });
    },
    [navigate, path, search],
  );

  return [params, setParams];
}

/**
 * The id segment of a detail route — `services/<id>`, `nodes/<id>`.
 *
 * Replaces `useParams<{ id: string }>()`. The host hands the plugin a splat
 * rather than named params, because the route it created is a single
 * `/plugins/addon-swarm/*` and it has no idea this plugin has detail pages
 * under it. Empty on a list route, which is exactly what the ported pages
 * already handled: they render the "not found" state for an empty id.
 */
export function usePluginDetailId(): string {
  const splat = usePluginSplat();
  return useMemo(() => splat.split('/')[1] ?? '', [splat]);
}
