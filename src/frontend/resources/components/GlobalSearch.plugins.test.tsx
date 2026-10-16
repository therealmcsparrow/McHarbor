// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import i18n from 'i18next';
import { http, HttpResponse } from 'msw';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PluginCapability } from '@core/plugins/loader';
import type { PluginSearchFanOut, PluginSearchSource } from '@core/plugins/pickerTypes';
import { useEnvironmentStore } from '@resources/stores/environment';
import { clearPluginCapabilities, givePluginCapabilities } from '../../test/plugin-capabilities';
import { grantPermissions } from '../../test/permissions';
import { renderWithProviders } from '../../test/render';
import { server } from '../../test/server';
import { GlobalSearch } from './GlobalSearch';
import { MAX_FAN_OUT_TARGETS, SEARCH_ICONS, fanOutTargets, pluginSearchItems } from './globalSearchPlugins';

const thingSource: PluginSearchSource = {
  key: 'thing',
  labelKey: 'search.group.things',
  path: '/api/things',
  envScoped: true,
  id: 'id',
  name: ['name'],
  detail: ['state'],
  link: '/plugins/thing-plugin/{id}',
  icon: 'device-desktop',
  permission: 'thing.view',
  i18nNs: 'thing',
};

const brokenSource: PluginSearchSource = {
  ...thingSource,
  key: 'broken',
  labelKey: 'search.group.broken',
  path: '/api/broken',
  envScoped: false,
};

const secretSource: PluginSearchSource = {
  ...thingSource,
  key: 'secret',
  labelKey: 'search.group.secret',
  path: '/api/secrets',
  envScoped: false,
  permission: 'thing.secret',
};

const clusterFanOut: PluginSearchFanOut = {
  path: '/api/clusters',
  id: 'id',
  param: 'cluster',
  label: 'name',
  permission: 'thing.clusters',
};

const podSource: PluginSearchSource = {
  ...thingSource,
  key: 'pod',
  labelKey: 'search.group.pods',
  path: '/api/pods',
  envScoped: false,
  id: 'name',
  detail: ['namespace'],
  link: '/plugins/thing-plugin/pods/{namespace}/{name}?cluster={cluster}',
  fanOut: clusterFanOut,
};

const fanned: PluginCapability = { plugin: 'thing-plugin', kind: 'search-source', sources: [podSource] };

const sources: PluginCapability = {
  plugin: 'thing-plugin',
  kind: 'search-source',
  sources: [thingSource, secretSource],
};

// Only the failure test declares the broken source: a failure is reported once per page load.
const withBroken: PluginCapability = { ...sources, sources: [thingSource, brokenSource, secretSource] };

const actions: PluginCapability = {
  plugin: 'thing-plugin',
  kind: 'search-action',
  actions: [
    {
      key: 'thing.create',
      labelKey: 'search.action.createThing',
      keywordsKey: 'search.action.createThingKeywords',
      icon: 'plus',
      link: '/plugins/thing-plugin?action=create',
      permission: 'thing.manage',
      i18nNs: 'thing',
    },
  ],
};

function environment(id: string, name: string) {
  return {
    id,
    name,
    connectionType: 'socket',
    isDefault: false,
    dockerVersion: null,
    scheduledUpdateCheckEnabled: false,
    automaticImagePruningEnabled: false,
    trackContainerEventsEnabled: false,
    collectContainerMetricsEnabled: false,
    highlightContainerChangesEnabled: false,
    dockerDiskUsageNotificationsEnabled: false,
    dockerDiskUsageThresholdPercent: 80,
  };
}

class NoopResizeObserver implements ResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

function Where() {
  const location = useLocation();
  return <output data-testid="where">{`${location.pathname}${location.search}`}</output>;
}

function renderPalette() {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="*" element={<Where />} />
      </Routes>
      <GlobalSearch open onOpenChange={() => undefined} />
    </MemoryRouter>,
  );
}

describe('GlobalSearch plugin contract', () => {
  const listed = vi.fn();
  const secretListed = vi.fn();
  const clustersListed = vi.fn();
  const podsListed = vi.fn();

  beforeAll(() => {
    i18n.addResourceBundle(
      'en',
      'thing',
      {
        search: {
          group: { things: 'Things', broken: 'Broken things', secret: 'Secret things', pods: 'Pods' },
          action: { createThing: 'Create thing', createThingKeywords: 'new gizmo' },
        },
      },
      true,
      true,
    );
    // cmdk measures and scrolls its list; jsdom implements neither.
    globalThis.ResizeObserver ??= NoopResizeObserver;
    if (!('scrollIntoView' in Element.prototype)) {
      Object.defineProperty(Element.prototype, 'scrollIntoView', { value: () => undefined, configurable: true });
    }
  });

  beforeEach(() => {
    listed.mockReset();
    secretListed.mockReset();
    clustersListed.mockReset();
    podsListed.mockReset();
    useEnvironmentStore.setState({
      currentId: 'e1',
      environments: [environment('e1', 'Alpha host'), environment('e2', 'Beta host')],
    });
    server.use(
      http.get('*/api/:kind', ({ params }) => {
        if (['containers', 'images', 'volumes', 'networks', 'stacks'].includes(String(params.kind))) {
          return HttpResponse.json({ success: true, data: [] });
        }
        return undefined;
      }),
      http.get('*/api/things', ({ request }) => {
        const env = new URL(request.url).searchParams.get('env');
        listed(env);
        const rows =
          env === 'e1'
            ? [{ id: 'vm-1', name: 'web-01', state: 'running' }]
            : [{ id: 'vm 2/x', name: 'db-01', state: 'stopped' }];
        return HttpResponse.json({ success: true, data: rows });
      }),
      http.get('*/api/broken', () => HttpResponse.json({ success: false, error: 'boom' }, { status: 500 })),
      http.get('*/api/clusters', () => {
        clustersListed();
        return HttpResponse.json({
          success: true,
          data: [{ id: 'c1', name: 'Lab' }, { id: 'c2', name: 'Prod' }, { name: 'no id' }],
        });
      }),
      http.get('*/api/pods', ({ request }) => {
        const cluster = new URL(request.url).searchParams.get('cluster');
        podsListed(cluster);
        const rows = cluster === 'c1' ? [{ name: 'web', namespace: 'default' }] : [{ name: 'db', namespace: 'data' }];
        return HttpResponse.json({ success: true, data: rows });
      }),
      http.get('*/api/secrets', () => {
        secretListed();
        return HttpResponse.json({ success: true, data: [{ id: 's1', name: 'hidden-01' }] });
      }),
    );
  });

  afterEach(() => {
    clearPluginCapabilities();
    vi.restoreAllMocks();
  });

  it('lists a declared source per environment under its heading and opens the item in its environment', async () => {
    grantPermissions(['thing.view']);
    givePluginCapabilities([sources]);
    renderPalette();

    const group = await screen.findByText('Things (2)');
    const options = within(group.closest('[cmdk-group]') as HTMLElement);
    expect(await options.findByText('web-01')).toBeInTheDocument();
    expect(options.getByText('db-01')).toBeInTheDocument();
    expect(listed.mock.calls.map(([env]) => env).sort((a, b) => a.localeCompare(b))).toEqual(['e1', 'e2']);

    await userEvent.click(options.getByText('db-01'));
    expect(screen.getByTestId('where')).toHaveTextContent('/plugins/thing-plugin/vm%202%2Fx');
    expect(useEnvironmentStore.getState().currentId).toBe('e2');
  });

  it('fills link placeholders from the item and drops an item it cannot fill', () => {
    const source = { ...thingSource, link: '/plugins/thing-plugin/{ns}/{id}', detail: ['state', 'ns'] };
    const items = pluginSearchItems(
      source,
      [
        { id: 'a b', name: 'one', state: 'up', ns: 'lab/x' },
        { id: 'c', name: 'two', state: 'up' },
      ],
      { id: 'e1', name: 'Alpha host' },
    );
    expect(items.map((i) => i.to)).toEqual(['/plugins/thing-plugin/lab%2Fx/a%20b']);
    expect(items[0]).toMatchObject({ label: 'one', detail: 'up · lab/x', envId: 'e1', envName: 'Alpha host' });
  });

  it('leaves out a failing source, reports it once, and keeps the others', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    grantPermissions(['thing.view']);
    givePluginCapabilities([withBroken]);
    renderPalette();

    expect(await screen.findByText('web-01')).toBeInTheDocument();
    await waitFor(() => expect(warn).toHaveBeenCalledTimes(1));
    expect(String(warn.mock.calls[0]?.[1])).toBe('/api/broken');
    expect(screen.queryByText(/Broken things/)).not.toBeInTheDocument();
  });

  it('tells the operator a plugin search source is unavailable instead of failing silently', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    grantPermissions(['thing.view']);
    givePluginCapabilities([withBroken]);
    renderPalette();

    expect(await screen.findByText('web-01')).toBeInTheDocument();
    expect(
      await screen.findByText('1 search source is unavailable, so its results are missing.', undefined, {
        timeout: 4000,
      }),
    ).toBeInTheDocument();
  });

  it('shows no unavailable-source line while every source answers', async () => {
    grantPermissions(['thing.view']);
    givePluginCapabilities([sources]);
    renderPalette();

    expect(await screen.findByText('web-01')).toBeInTheDocument();
    expect(screen.queryByText(/search sources? (is|are) unavailable/)).not.toBeInTheDocument();
  });

  it('never lists a source the operator lacks the key for, and drops an unloaded plugin', async () => {
    grantPermissions(['thing.view']);
    givePluginCapabilities([sources]);
    const { unmount } = renderPalette();
    expect(await screen.findByText('web-01')).toBeInTheDocument();
    expect(secretListed).not.toHaveBeenCalled();
    expect(screen.queryByText(/Secret things/)).not.toBeInTheDocument();
    unmount();

    listed.mockReset();
    givePluginCapabilities([sources], ['thing-plugin']);
    renderPalette();
    expect(await screen.findByRole('combobox')).toBeInTheDocument();
    expect(screen.queryByText(/Things \(/)).not.toBeInTheDocument();
    expect(listed).not.toHaveBeenCalled();
  });

  it('adds a declared action to Actions only for an operator holding its key, and it navigates', async () => {
    grantPermissions(['thing.view']);
    givePluginCapabilities([sources, actions]);
    const { unmount } = renderPalette();
    expect(await screen.findByText('web-01')).toBeInTheDocument();
    expect(screen.queryByText('Create thing')).not.toBeInTheDocument();
    unmount();

    grantPermissions(['thing.view', 'thing.manage']);
    renderPalette();
    await userEvent.type(screen.getByRole('combobox'), 'gizmo');
    const action = await screen.findByText('Create thing');
    await userEvent.click(action);
    expect(screen.getByTestId('where')).toHaveTextContent('/plugins/thing-plugin?action=create');
  });

  it('repeats a fan-out source once per target and links each item with its target', async () => {
    grantPermissions(['thing.view', 'thing.clusters']);
    givePluginCapabilities([fanned]);
    renderPalette();

    const group = await screen.findByText('Pods (2)');
    const options = within(group.closest('[cmdk-group]') as HTMLElement);
    expect(await options.findByText('web')).toBeInTheDocument();
    expect(options.getByText('default · Lab')).toBeInTheDocument();
    expect(podsListed.mock.calls.map(([cluster]) => cluster).sort((a, b) => a.localeCompare(b))).toEqual(['c1', 'c2']);

    await userEvent.click(options.getByText('db'));
    expect(screen.getByTestId('where')).toHaveTextContent('/plugins/thing-plugin/pods/data/db?cluster=c2');
  });

  it('never fans out without the key that gates the target list', async () => {
    grantPermissions(['thing.view']);
    givePluginCapabilities([fanned, sources]);
    renderPalette();
    expect(await screen.findByText('web-01')).toBeInTheDocument();
    expect(clustersListed).not.toHaveBeenCalled();
    expect(podsListed).not.toHaveBeenCalled();
    expect(screen.queryByText(/Pods \(/)).not.toBeInTheDocument();
  });

  it('caps fan-out targets and skips rows without an id', () => {
    const rows = Array.from({ length: 40 }, (_, i) => ({ id: `c${i}`, name: `Cluster ${i}` }));
    const targets = fanOutTargets(clusterFanOut, [{ name: 'no id' }, ...rows]);
    expect(targets).toHaveLength(MAX_FAN_OUT_TARGETS);
    expect(targets[0]).toEqual({ param: 'cluster', id: 'c0', label: 'Cluster 0' });
  });

  it('maps exactly the icons the shared contract allows', () => {
    const contract = readFileSync(resolve(__dirname, '../../../contracts/plugincaps/search_icons.yaml'), 'utf8');
    const icons = [...contract.matchAll(/^ {2}- (\S+)$/gm)].map((m) => m[1] ?? '');
    expect(Object.keys(SEARCH_ICONS).sort((a, b) => a.localeCompare(b))).toEqual(
      [...icons].sort((a, b) => a.localeCompare(b)),
    );
  });
});
