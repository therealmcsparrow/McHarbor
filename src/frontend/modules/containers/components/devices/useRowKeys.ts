// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';

type RowKeyState = { keys: string[]; counter: number };

export function useRowKeys(count: number, prefix: string) {
  const [state, setState] = useState<RowKeyState>({ keys: [], counter: 0 });

  let keys = state.keys;
  if (keys.length !== count) {
    let counter = state.counter;
    keys = keys.slice(0, count);
    while (keys.length < count) {
      counter += 1;
      keys.push(`${prefix}-${counter}`);
    }
    setState({ keys, counter });
  }

  const forget = useCallback((index: number) => {
    setState((current) => ({ ...current, keys: current.keys.filter((_, i) => i !== index) }));
  }, []);
  const keyAt = (index: number) => keys[index] ?? prefix;
  return { keyAt, forget };
}
