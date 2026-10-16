// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { checkDemoConfig, demoSizesOf, draftOf } from '@resources/constants/demoConfig';
import { DEMO_DEFAULTS, DEMO_LIMITS } from './test-fixtures';

describe('checkDemoConfig', () => {
  it('accepts the defaults and round-trips them through the draft', () => {
    const result = checkDemoConfig(draftOf(DEMO_DEFAULTS), DEMO_LIMITS);
    expect(result.problems).toEqual({});
    expect(result.config).toEqual(DEMO_DEFAULTS);
  });

  it('rejects values outside the limits, blanks and fractions, naming the range', () => {
    const draft = {
      ...draftOf(DEMO_DEFAULTS),
      environments: '49',
      historyDays: '6',
      containers: '',
      swarmServices: '1.5',
    };
    const result = checkDemoConfig(draft, DEMO_LIMITS);
    expect(result.config).toBeUndefined();
    expect(result.problems).toEqual({
      environments: { kind: 'range', min: 1, max: 48 },
      historyDays: { kind: 'range', min: 7, max: 365 },
      containers: { kind: 'range', min: 0, max: 6000 },
      swarmServices: { kind: 'range', min: 0, max: 400 },
    });
  });

  it('accepts both ends of every range', () => {
    const low = checkDemoConfig(
      { ...draftOf(DEMO_DEFAULTS), environments: '1', historyDays: '7', swarmClusters: '0', swarmNodes: '0' },
      DEMO_LIMITS,
    );
    const high = checkDemoConfig(
      { ...draftOf(DEMO_DEFAULTS), environments: '48', historyDays: '365', swarmClusters: '5', swarmNodes: '60' },
      DEMO_LIMITS,
    );
    expect(low.problems).toEqual({});
    expect(high.problems).toEqual({});
  });

  it('needs at least one Swarm node per Swarm cluster', () => {
    const result = checkDemoConfig({ ...draftOf(DEMO_DEFAULTS), swarmClusters: '4', swarmNodes: '3' }, DEMO_LIMITS);
    expect(result.config).toBeUndefined();
    expect(result.problems).toEqual({ swarmNodes: { kind: 'swarmNodes', min: 4, max: 60 } });
  });

  it('prefills from a parked config and falls back to the defaults', () => {
    const parked = { ...DEMO_DEFAULTS, environments: 4 };
    const base = {
      available: true,
      active: false,
      databaseExists: true,
      clustered: false,
      canEnter: true,
      defaults: DEMO_DEFAULTS,
      limits: DEMO_LIMITS,
    };
    expect(demoSizesOf({ ...base, config: parked }).initial).toEqual(parked);
    expect(demoSizesOf(base).initial).toEqual(DEMO_DEFAULTS);
  });
});
