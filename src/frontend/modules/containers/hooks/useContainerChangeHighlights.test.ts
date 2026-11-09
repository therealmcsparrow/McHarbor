// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import type { ContainerInfo } from '@core/types/docker';
import { useContainerChangeHighlights } from './useContainerChangeHighlights';

function container(id: string, state: ContainerInfo['State']): ContainerInfo {
  return {
    Id: id,
    Names: [`/${id}`],
    Image: 'nginx:latest',
    ImageID: 'sha256:abc',
    Command: 'nginx',
    Created: 0,
    Ports: [],
    Labels: {},
    State: state,
    Status: state,
    NetworkSettings: { Networks: {} },
    Mounts: [],
  };
}

describe('useContainerChangeHighlights', () => {
  // ContainersPage passes `data: containers = []` while the query is empty (a cold cache after re-login),
  // so every render hands over a new array; that must settle instead of looping into React #301.
  it('settles when every render passes a fresh empty array', () => {
    const { result, rerender } = renderHook(() =>
      useContainerChangeHighlights({ containers: [], updateResults: undefined, enabled: true }),
    );
    expect(() => rerender()).not.toThrow();
    expect(result.current.size).toBe(0);
  });

  it('does not highlight a container whose content is unchanged but whose array is new', () => {
    const { result, rerender } = renderHook(() =>
      useContainerChangeHighlights({
        containers: [container('a', 'running')],
        updateResults: undefined,
        enabled: true,
      }),
    );
    expect(() => rerender()).not.toThrow();
    expect(result.current.size).toBe(0);
  });

  it('highlights a container whose state changed', () => {
    const { result, rerender } = renderHook(
      ({ state }: { state: ContainerInfo['State'] }) =>
        useContainerChangeHighlights({
          containers: [container('a', state)],
          updateResults: undefined,
          enabled: true,
        }),
      { initialProps: { state: 'running' as ContainerInfo['State'] } },
    );
    rerender({ state: 'exited' });
    expect([...result.current]).toEqual(['a']);
  });
});
