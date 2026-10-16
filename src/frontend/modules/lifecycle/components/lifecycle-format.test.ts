// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import type { LifecycleEvent } from '../hooks/useLifecycle';
import { useLifecycleSubjectPages } from '../hooks/useLifecycleSubjectPages';
import { buildSubjectPath } from './lifecycle-format';

function event(subjectType: LifecycleEvent['subjectType'], subjectId = 'x1'): LifecycleEvent {
  return {
    id: 'e1',
    subjectType,
    subjectId,
    eventType: 'state',
    action: 'start',
    severity: 'info',
    source: 'test',
    timestamp: '2026-09-29T00:00:00Z',
  };
}

describe('lifecycle subject links', () => {
  afterEach(() => clearPluginCapabilities());

  it('links a plugin-owned subject to the page its plugin declares', () => {
    givePluginCapabilities([
      { plugin: 'vm-plugin', kind: 'lifecycle-subject', subjectType: 'vm', page: '/plugins/vm-plugin' },
      { plugin: 'pool-plugin', kind: 'lifecycle-subject', subjectType: 'pool', page: '/plugins/pool-plugin/pools' },
    ]);
    const { result } = renderHook(() => useLifecycleSubjectPages());
    expect(buildSubjectPath(event('vm'), result.current)).toBe('/plugins/vm-plugin');
    expect(buildSubjectPath(event('pool'), result.current)).toBe('/plugins/pool-plugin/pools');
    expect(buildSubjectPath(event('container', 'c9'), result.current)).toBe('/containers/c9');
  });

  it('links nowhere once the owning plugin is gone', () => {
    givePluginCapabilities(
      [{ plugin: 'vm-plugin', kind: 'lifecycle-subject', subjectType: 'vm', page: '/plugins/vm-plugin' }],
      ['vm-plugin'],
    );
    const { result } = renderHook(() => useLifecycleSubjectPages());
    expect(result.current).toEqual({});
    expect(buildSubjectPath(event('vm'), result.current)).toBe('#');
  });
});
