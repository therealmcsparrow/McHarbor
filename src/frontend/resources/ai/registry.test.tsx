// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AIContextProvider, useAIContext, useAIContexts, type AIContext } from './registry';

const wrapper = ({ children }: { children: ReactNode }) => <AIContextProvider>{children}</AIContextProvider>;

function mount(initial: AIContext) {
  return renderHook(
    ({ ctx }: { ctx: AIContext }) => {
      useAIContext(ctx);
      return useAIContexts();
    },
    { wrapper, initialProps: { ctx: initial } },
  );
}

const base: AIContext = { source: 'test-container', type: 'container', resourceId: 'c1', resourceName: 'nginx' };

describe('AI context registry', () => {
  it('republishes when metadata changes but the identity fields do not', () => {
    const view = mount({ ...base, metadata: { state: 'running' }, summary: 'running' });
    expect(view.result.current.contexts[0]?.metadata).toEqual({ state: 'running' });

    view.rerender({ ctx: { ...base, metadata: { state: 'exited' }, summary: 'exited' } });

    expect(view.result.current.contexts).toHaveLength(1);
    expect(view.result.current.contexts[0]?.metadata).toEqual({ state: 'exited' });
    expect(view.result.current.focused.summary).toBe('exited');
  });

  it('does not churn the store when a new object carries identical content', () => {
    const view = mount({ ...base, metadata: { state: 'running' } });
    const before = view.result.current.contexts;
    view.rerender({ ctx: { ...base, metadata: { state: 'running' } } });
    expect(view.result.current.contexts).toBe(before);
  });

  it('runs the latest capability callback without re-registering', () => {
    const first = vi.fn();
    const second = vi.fn();
    const cap = (run: () => void) => [{ id: 'restart', label: 'Restart', run }];
    const view = mount({ ...base, capabilities: cap(first) });
    view.rerender({ ctx: { ...base, capabilities: cap(second) } });

    act(() => {
      void view.result.current.contexts[0]?.capabilities?.[0]?.run();
    });
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledOnce();
  });
});
