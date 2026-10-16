// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import { act } from 'react';
import { afterEach, describe, expect, it } from 'vitest';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { useCustomNodeRegistry } from '../stores/custom-node-registry';
import { useNodeDefinition } from '../hooks/useNodeDefinition';
import type { NodeDefinition } from '../types';
import { resolveNodeDefinition } from './resolve';

const pluginNode: NodeDefinition = {
  key: 'plugin:thing-plugin:thing-trigger',
  label: 'Thing trigger',
  category: 'trigger',
  description: '',
  icon: 'puzzle',
  configSchema: [],
  inputPorts: [],
  outputPorts: ['output'],
};

const claim = {
  plugin: 'thing-plugin',
  kind: 'workflow-node-alias',
  alias: 'legacy-thing-trigger',
  node: 'thing-trigger',
};

function registerPluginNode() {
  act(() => useCustomNodeRegistry.getState().setCustomNodes([pluginNode]));
}

describe('stored bare actions a plugin now owns', () => {
  afterEach(() => {
    clearPluginCapabilities();
    act(() => useCustomNodeRegistry.getState().setCustomNodes([]));
  });

  it('resolve to the plugin node a loaded plugin claims them for', () => {
    registerPluginNode();
    givePluginCapabilities([claim]);
    expect(resolveNodeDefinition('legacy-thing-trigger')?.key).toBe(pluginNode.key);
    const { result } = renderHook(() => useNodeDefinition('legacy-thing-trigger'));
    expect(result.current?.key).toBe(pluginNode.key);
  });

  it('stay unresolved, rather than failing, when the claiming plugin is not loaded', () => {
    registerPluginNode();
    givePluginCapabilities([claim], ['thing-plugin']);
    expect(resolveNodeDefinition('legacy-thing-trigger')).toBeUndefined();
    const { result } = renderHook(() => useNodeDefinition('legacy-thing-trigger'));
    expect(result.current).toBeUndefined();
  });

  it('show a stored per-provider send as the generic send node while its comm plugin is loaded', () => {
    const channelClaim = {
      plugin: 'pigeon-plugin',
      kind: 'workflow-node-alias',
      alias: 'send-pigeon',
      channelType: 'pigeon',
    };
    givePluginCapabilities([channelClaim]);
    expect(resolveNodeDefinition('send-pigeon')?.key).toBe('send-communication');
    const { result } = renderHook(() => useNodeDefinition('send-pigeon'));
    expect(result.current?.key).toBe('send-communication');

    givePluginCapabilities([channelClaim], ['pigeon-plugin']);
    expect(resolveNodeDefinition('send-pigeon')).toBeUndefined();
  });

  it('never let a claim shadow a built-in node', () => {
    givePluginCapabilities([{ ...claim, alias: 'manual-trigger' }]);
    registerPluginNode();
    expect(resolveNodeDefinition('manual-trigger')?.key).toBe('manual-trigger');
  });
});
