// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// PluginWidgetMount — the host-side shell a plugin dashboard widget lives in.
//
// A plugin UMD bundle compiles its own React, so a component it exports
// cannot be rendered inside the host tree: every hook it calls reaches the
// bundle's dispatcher, which is null while the host is rendering, and the
// widget dies with "Invalid hook call". Pages solved this with an imperative
// `mount(el, ctx)` contract (see PluginPageHost); widgets use the same shape.
//
// The plugin exports `widgets[key] = { mount, unmount?, update? }`. The host
// renders one div, calls `mount` once with a context that carries the
// dashboard's `navigate` (so a click-through stays an in-app navigation
// rather than a full reload), and calls `update` when the tile is resized in
// edit mode.

import { useEffect, useRef, useState, type ComponentType } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';

/** The props the dashboard grid passes every widget; `typeId` stays a plain string so the host never names a built-in widget id. */
export type WidgetProps = { colSpan: number; typeId: string };

/** What the host hands a plugin widget on mount and on every update. */
export type PluginWidgetContext = {
  slug: string;
  key: string;
  /** The dashboard type id, `plugin:<slug>:<key>`. */
  typeId: string;
  /** Grid columns the tile currently spans. */
  colSpan: number;
  /** In-app navigation, e.g. `navigate('/plugins/addon-swarm/services')`. */
  navigate: (to: string, opts?: { replace?: boolean }) => void;
};

/** The shape a plugin's `widgets[key]` export must satisfy. */
export type PluginWidgetEntry = {
  mount: (el: HTMLElement, ctx: PluginWidgetContext) => void | (() => void);
  unmount?: (el: HTMLElement) => void;
  update?: (el: HTMLElement, ctx: PluginWidgetContext) => void;
};

export function isPluginWidgetEntry(value: unknown): value is PluginWidgetEntry {
  return typeof value === 'object' && value !== null && typeof (value as PluginWidgetEntry).mount === 'function';
}

/**
 * Wraps a plugin widget entry in a host component the dashboard grid can
 * render like any built-in widget.
 */
export function createPluginWidgetComponent(
  slug: string,
  key: string,
  entry: PluginWidgetEntry,
): ComponentType<WidgetProps> {
  function PluginWidgetMount({ colSpan, typeId }: WidgetProps) {
    const navigate = useNavigate();
    const { t } = useTranslation('common');
    const ref = useRef<HTMLDivElement>(null);
    const [failure, setFailure] = useState<Error | null>(null);
    const latest = useRef<PluginWidgetContext>({ slug, key, typeId, colSpan, navigate });
    latest.current = { slug, key, typeId, colSpan, navigate };

    useEffect(() => {
      const el = ref.current;
      if (!el) return;
      let cleanup: void | (() => void);
      try {
        cleanup = entry.mount(el, latest.current);
      } catch (err) {
        console.error(`plugin widget ${slug}:${key} mount threw`, err);
        setFailure(err instanceof Error ? err : new Error(String(err)));
      }
      return () => {
        try {
          if (typeof cleanup === 'function') cleanup();
          entry.unmount?.(el);
        } catch (err) {
          console.error(`plugin widget ${slug}:${key} unmount threw`, err);
        }
      };
      // Mount exactly once per tile instance; resizes go through `update`.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useEffect(() => {
      const el = ref.current;
      if (!el || !entry.update) return;
      try {
        entry.update(el, latest.current);
      } catch (err) {
        console.error(`plugin widget ${slug}:${key} update threw`, err);
        setFailure(err instanceof Error ? err : new Error(String(err)));
      }
    }, [colSpan]);

    return (
      <>
        {failure ? <ErrorState variant="inline" title={t('plugins.mountFailed', { slug })} error={failure} /> : null}
        <div ref={ref} data-plugin-widget={`${slug}:${key}`} className="h-full min-h-0 w-full overflow-hidden" />
      </>
    );
  }
  PluginWidgetMount.displayName = `PluginWidget(${slug}:${key})`;
  return PluginWidgetMount;
}
