// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Navigation, without react-router.
//
// This is the one thing that could not be ported unchanged from the core
// module. The host mounts this bundle as a UMD global inside its own React
// tree, but the bundle carries its *own* copy of every library it imports —
// and a second react-router copy has no Router context, so `<Link>` and
// `useNavigate()` do not merely misbehave, they throw and blank the plugin
// root. (The reporting plugin learned the same lesson through its breadcrumb.)
//
// What the host does provide is a `PluginNavContext` on mount and on every
// navigation below `/plugins/addon-kubernetes`: where we are, and a `navigate`
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
 * The plugin's route segment below `/plugins/addon-kubernetes`, e.g. `pods` or
 * `pods/default/api-7d9`. Empty on the overview.
 */
export function usePluginSplat(): string {
  return usePluginNav().splat;
}

type PluginLinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> & {
  /** A path relative to the plugin root — `''`, `'pods'`, `'pods/ns/name'`. */
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

/** Absolute path for a plugin-relative route. */
export function pluginHref(to: string): string {
  const trimmed = to.replace(/^\/+/, '');
  return trimmed ? `/plugins/addon-kubernetes/${trimmed}` : '/plugins/addon-kubernetes';
}

/** `navigate` bound to plugin-relative paths, for callers that are not links. */
export function usePluginNavigate(): (to: string, opts?: { replace?: boolean }) => void {
  const { navigate } = usePluginNav();
  return useMemo(() => (to: string, opts?: { replace?: boolean }) => navigate(pluginHref(to), opts), [navigate]);
}
