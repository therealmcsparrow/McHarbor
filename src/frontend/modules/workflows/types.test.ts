// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { CanvasNode, NodeDefinition } from './types';
import { JUNCTION_SIZE, getEffectiveInputPorts, getEffectiveOutputPorts, getNodeHeightForPorts } from './types';

const baseNode: CanvasNode = {
  id: 'node-1',
  type: 'action',
  action: 'debug',
  label: 'Debug',
  config: {},
  position: { x: 0, y: 0 },
};

const definition: NodeDefinition = {
  key: 'debug',
  label: 'Debug',
  category: 'utility',
  description: 'Debug output',
  icon: 'IconBug',
  configSchema: [],
  inputPorts: ['input'],
  outputPorts: ['done'],
};

describe('workflow type helpers', () => {
  it('uses definition ports and fallback ports', () => {
    expect(getEffectiveInputPorts(baseNode, definition)).toEqual(['input']);
    expect(getEffectiveInputPorts(baseNode)).toEqual(['input']);
    expect(getEffectiveOutputPorts(baseNode, definition)).toEqual(['done']);
    expect(getEffectiveOutputPorts(baseNode)).toEqual(['output']);
  });

  it('expands join input ports with lower and upper bounds', () => {
    expect(getEffectiveInputPorts({ ...baseNode, action: 'join', config: { input_count: 1 } })).toEqual([
      'input_0',
      'input_1',
    ]);
    expect(getEffectiveInputPorts({ ...baseNode, action: 'join', config: { input_count: 20 } })).toHaveLength(16);
    expect(getEffectiveInputPorts({ ...baseNode, action: 'join', config: { input_count: 'bad' } })).toEqual([
      'input_0',
      'input_1',
    ]);
  });

  it('expands condition and switch output ports from config', () => {
    expect(
      getEffectiveOutputPorts({
        ...baseNode,
        action: 'condition',
        config: { extra_conditions: [{ field: 'payload.ok', operator: 'eq', value: 'yes', label: 'OK' }] },
      }),
    ).toEqual(['true', 'condition_0', 'else']);

    expect(getEffectiveOutputPorts({ ...baseNode, action: 'switch', config: { switch_cases: [] } })).toEqual([
      'case_0',
      'default',
    ]);
    expect(
      getEffectiveOutputPorts({
        ...baseNode,
        action: 'switch',
        config: {
          switch_cases: [
            { value: 'one', label: 'One' },
            { value: 'two', label: 'Two' },
          ],
        },
      }),
    ).toEqual(['case_0', 'case_1', 'default']);
  });

  it('calculates node height from port count', () => {
    expect(getNodeHeightForPorts(0)).toBe(76);
    expect(getNodeHeightForPorts(4)).toBe(110);
    expect(JUNCTION_SIZE).toBe(20);
  });
});
