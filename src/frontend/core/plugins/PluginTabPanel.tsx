// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState, type ComponentType, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { loadPluginExport } from './pluginComponent';

export type PluginTabContext = {
  pageId: string;
  tabKey: string;
  entityId?: string;
};

export type PluginTabEntry = {
  mount: (el: HTMLElement, ctx: PluginTabContext) => void | (() => void);
  unmount?: (el: HTMLElement) => void;
  update?: (el: HTMLElement, ctx: PluginTabContext) => void;
};

function isPluginTabEntry(value: unknown): value is PluginTabEntry {
  return typeof value === 'object' && value !== null && typeof (value as PluginTabEntry).mount === 'function';
}

type PluginTabPanelProps = {
  slug: string;
  tabKey: string;
  context: PluginTabContext;
};

type LoadedTab =
  | { status: 'loading' }
  | { status: 'missing' }
  | { status: 'entry'; entry: PluginTabEntry }
  | { status: 'component'; Component: ComponentType<PluginTabContext> };

export function PluginTabPanel({ slug, tabKey, context }: PluginTabPanelProps): ReactElement {
  const { t } = useTranslation('common');
  const tabId = `${slug}|${tabKey}`;
  const [loaded, setLoaded] = useState<{ id: string; tab: LoadedTab } | null>(null);
  const state: LoadedTab = loaded?.id === tabId ? loaded.tab : { status: 'loading' };

  useEffect(() => {
    let cancelled = false;
    const setState = (tab: LoadedTab) => setLoaded({ id: `${slug}|${tabKey}`, tab });

    loadPluginExport(slug, 'tabs', tabKey)
      .then((value) => {
        if (cancelled) return;
        if (isPluginTabEntry(value)) {
          setState({ status: 'entry', entry: value });
          return;
        }
        if (typeof value === 'function') {
          setState({
            status: 'component',
            Component: value as ComponentType<PluginTabContext>,
          });
          return;
        }
        setState({ status: 'missing' });
      })
      .catch(() => {
        if (!cancelled) setState({ status: 'missing' });
      });

    return () => {
      cancelled = true;
    };
  }, [slug, tabKey]);

  if (state.status === 'loading') {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner />
      </div>
    );
  }

  if (state.status === 'missing') {
    return (
      <EmptyState
        title={t('plugins.tabMissing.title')}
        description={t('plugins.tabMissing.body', { slug, tab: tabKey })}
      />
    );
  }

  if (state.status === 'component') {
    const { Component } = state;
    return <Component {...context} />;
  }

  return <PluginTabMount slug={slug} tabKey={tabKey} entry={state.entry} context={context} />;
}

function PluginTabMount({
  slug,
  tabKey,
  entry,
  context,
}: PluginTabPanelProps & { entry: PluginTabEntry }): ReactElement {
  const { t } = useTranslation('common');
  const ref = useRef<HTMLDivElement>(null);
  const [failure, setFailure] = useState<Error | null>(null);
  const latest = useRef(context);
  latest.current = context;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    let cleanup: void | (() => void);
    try {
      cleanup = entry.mount(el, latest.current);
    } catch (err) {
      console.error(`plugin tab ${slug}:${tabKey} mount threw`, err);
      setFailure(err instanceof Error ? err : new Error(String(err)));
    }
    return () => {
      try {
        if (typeof cleanup === 'function') cleanup();
        entry.unmount?.(el);
      } catch (err) {
        console.error(`plugin tab ${slug}:${tabKey} unmount threw`, err);
      }
    };
    // oxlint-disable-next-line react/exhaustive-deps -- the tab mounts once per registry entry; slug, tabKey and ctx are read at mount and later changes go through entry.update
  }, [entry]);

  useEffect(() => {
    const el = ref.current;
    if (!el || !entry.update) return;
    try {
      entry.update(el, latest.current);
    } catch (err) {
      console.error(`plugin tab ${slug}:${tabKey} update threw`, err);
      setFailure(err instanceof Error ? err : new Error(String(err)));
    }
  }, [entry, slug, tabKey, context.entityId]);

  return (
    <>
      {failure ? <ErrorState variant="inline" title={t('plugins.mountFailed', { slug })} error={failure} /> : null}
      <div ref={ref} data-plugin-tab={`${slug}:${tabKey}`} className="min-h-0 w-full flex-1" />
    </>
  );
}
