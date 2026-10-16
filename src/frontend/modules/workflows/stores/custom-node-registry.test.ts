// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import type { NodeDefinition } from '../types';
import { useCustomNodeRegistry } from './custom-node-registry';

const customNode: NodeDefinition = {
  key: 'custom',
  label: 'Custom',
  category: 'utility',
  description: 'Custom node',
  icon: 'IconCode',
  configSchema: [],
  inputPorts: ['input'],
  outputPorts: ['output'],
};

describe('useCustomNodeRegistry', () => {
  beforeEach(() => {
    useCustomNodeRegistry.setState({ customNodes: [] });
  });

  it('sets custom node definitions', () => {
    useCustomNodeRegistry.getState().setCustomNodes([customNode]);

    expect(useCustomNodeRegistry.getState().customNodes).toEqual([customNode]);
  });
});
