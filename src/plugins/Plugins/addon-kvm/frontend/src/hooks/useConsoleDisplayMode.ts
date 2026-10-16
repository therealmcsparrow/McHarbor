// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { readDisplayMode, writeDisplayMode, type ConsoleDisplayMode } from '../console-resize';

/** The viewer's own choice between resizing the guest and scaling the picture, kept per browser. */
export function useConsoleDisplayMode() {
  const [mode, setMode] = useState<ConsoleDisplayMode>(readDisplayMode);
  const choose = useCallback((next: ConsoleDisplayMode) => {
    writeDisplayMode(next);
    setMode(next);
  }, []);
  return { mode, setMode: choose };
}
