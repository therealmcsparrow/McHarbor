// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { describe, expect, it } from 'vitest';
import { useReactiveQueryData } from './useReactiveQueryData';

describe('useReactiveQueryData', () => {
  it('reacts to query cache updates', () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(['items'], ['one']);

    function Wrapper({ children }: PropsWithChildren) {
      return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    }

    const { result } = renderHook(() => useReactiveQueryData<string[]>(['items']), { wrapper: Wrapper });

    expect(result.current).toEqual(['one']);

    act(() => {
      queryClient.setQueryData(['items'], ['one', 'two']);
    });

    expect(result.current).toEqual(['one', 'two']);
  });
});
