// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PluginCapability } from '@core/plugins/loader';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { PluginSelect } from './PluginSelect';
import { pickerItems, pickerQuery } from '../utils/plugin-picker';

const source = { plugin: 'thing-plugin', target: 'thing' };

const picker: PluginCapability = {
  plugin: 'thing-plugin',
  kind: 'picker-source',
  target: 'thing',
  picker: {
    path: '/api/things',
    query: { env: '$env' },
    id: 'id',
    name: ['name'],
    detail: ['state'],
    labelKey: 'runbook.targets.thing',
    i18nNs: 'thing',
    permission: 'thing.view',
  },
};

describe('PluginSelect', () => {
  const listed = vi.fn();

  beforeEach(() => {
    listed.mockReset();
    server.use(
      http.get('*/api/things', ({ request }) => {
        listed(new URL(request.url).searchParams.get('env'));
        return HttpResponse.json({
          success: true,
          data: [
            { id: 't1', name: 'Alpha', state: 'running' },
            { id: 't2', name: 'Beta', state: 'stopped' },
          ],
        });
      }),
    );
  });
  afterEach(() => {
    clearPluginCapabilities();
    resetPermissions();
  });

  it('lists the route the plugin declares, scoped to the node environment', async () => {
    givePluginCapabilities([picker]);
    grantPermissions(['thing.view']);
    const onChange = vi.fn();
    renderWithProviders(<PluginSelect value="" onChange={onChange} source={source} envId="env-1" />);

    await userEvent.click(screen.getByRole('button', { name: 'Select...' }));
    await userEvent.click(await screen.findByText('Beta'));

    expect(onChange).toHaveBeenCalledWith('t2');
    expect(listed).toHaveBeenCalledWith('env-1');
  });

  it('stays visible but disabled, saying why, when the declaring plugin is not loaded', async () => {
    givePluginCapabilities([picker], ['thing-plugin']);
    grantPermissions(['thing.view']);
    renderWithProviders(<PluginSelect value="t1" onChange={vi.fn()} source={source} allowCustom />);

    expect(screen.getByRole('button', { name: 'Select...' })).toBeDisabled();
    expect(screen.getByText('The plugin that provides this list is not installed or not enabled.')).toBeInTheDocument();
    await waitFor(() => expect(listed).not.toHaveBeenCalled());
  });

  it('is disabled, naming the key, and asks nothing when the list permission is missing', async () => {
    givePluginCapabilities([picker]);
    grantPermissions([]);
    renderWithProviders(<PluginSelect value="" onChange={vi.fn()} source={source} />);

    const trigger = screen.getByRole('button', { name: 'Select...' });
    expect(trigger).toHaveAttribute('aria-disabled', 'true');
    await userEvent.tab();
    expect(await screen.findAllByText('Requires the thing.view permission')).not.toHaveLength(0);
    expect(listed).not.toHaveBeenCalled();
  });
});

describe('plugin picker mapping', () => {
  it('reads ids, titles and details the way the runbook selector does', () => {
    const list = picker.picker!;
    expect(pickerQuery(list, 'e1')).toEqual({ env: 'e1' });
    expect(pickerQuery(list)).toBeUndefined();
    expect(pickerQuery({ ...list, query: { kind: '=vm', cluster: 'cluster' } })).toEqual({ kind: 'vm' });

    expect(pickerItems(list, [{ id: 't1', name: '', state: 'running' }, { id: '' }])).toEqual([
      { value: 't1', title: 't1', subtitle: 'running' },
    ]);
    const joined = { ...list, id: undefined, idJoin: ['ns', 'name'], items: 'rows' };
    expect(pickerItems(joined, { rows: [{ ns: 'a', name: 'b' }] })).toEqual([
      { value: 'a/b', title: 'b', subtitle: undefined },
    ]);
    expect(pickerItems(joined, [{ ns: 'a', name: 'b' }])).toEqual([]);
  });
});
